package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardedStream(t *testing.T) {
	text := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "output-canary"},
		{Type: provider.PartTextEnd, ID: "text"},
	}
	tool := []provider.StreamPart{
		{Type: provider.PartToolInputStart, ID: "call", ToolName: "lookup"},
		{Type: provider.PartToolInputDelta, ID: "call", Delta: `{"secret":"output-canary"}`},
		{Type: provider.PartToolInputEnd, ID: "call"},
		{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: `{"secret":"output-canary"}`},
	}
	textOutput := []provider.ContentPart{provider.TextPart("output-canary")}
	toolOutput := []provider.ContentPart{provider.ToolCallPart("call", "lookup", json.RawMessage(`{"secret":"output-canary"}`))}
	for _, tc := range []struct {
		expectedOutput []provider.ContentPart
		name           string
		parts          []provider.StreamPart
		postErr        error
		status         int
		postCalls      int
		bytes          int64
		partLimit      int
		noClose        bool
	}{
		{name: "text allow", parts: append(append([]provider.StreamPart{}, text...), finishPart()), status: 200, postCalls: 1, expectedOutput: textOutput},
		{name: "text deny", parts: append(append([]provider.StreamPart{}, text...), finishPart()), postErr: ErrGuardDenied, status: 403, postCalls: 1, expectedOutput: textOutput},
		{name: "tool deny", parts: append(append([]provider.StreamPart{}, tool...), finishPart()), postErr: ErrGuardDenied, status: 403, postCalls: 1, expectedOutput: toolOutput},
		{name: "tool allow", parts: append(append([]provider.StreamPart{}, tool...), finishPart()), status: 200, postCalls: 1, expectedOutput: toolOutput},
		{name: "missing finish", parts: text, status: 424},
		{name: "finish without EOF", parts: append(append([]provider.StreamPart{}, text...), finishPart()), status: 200, postCalls: 1, noClose: true, expectedOutput: textOutput},
		{name: "output transformation", parts: append(append([]provider.StreamPart{}, text...), finishPart()), postErr: ErrGuardFailed, status: 424, postCalls: 1, expectedOutput: textOutput},
		{name: "buffer limit", parts: append(append([]provider.StreamPart{}, text...), finishPart()), status: 424, bytes: 1},
		{name: "part limit", parts: append(append([]provider.StreamPart{}, text...), finishPart()), status: 424, partLimit: 2},
		{name: "unfinished text", parts: append(append([]provider.StreamPart{}, text[:2]...), finishPart()), status: 424},
		{name: "closed tool without call", parts: append(append([]provider.StreamPart{}, tool[:3]...), finishPart()), status: 424},
		{name: "standalone call", parts: []provider.StreamPart{tool[3], finishPart()}, status: 200, postCalls: 1, expectedOutput: toolOutput},
		{name: "no argument deltas", parts: []provider.StreamPart{tool[0], tool[2], tool[3], finishPart()}, status: 200, postCalls: 1, expectedOutput: toolOutput},
		{name: "argument mismatch", parts: []provider.StreamPart{tool[0], tool[1], tool[2], {Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: `{"secret":"other"}`}, finishPart()}, status: 424},
		{name: "provider error before finish", parts: []provider.StreamPart{{Type: provider.PartError, APICallError: &provider.APICallError{Message: "private", StatusCode: 500}}, finishPart()}, status: 200, postCalls: 1, expectedOutput: []provider.ContentPart{}},
		{name: "reasoning allow", parts: []provider.StreamPart{{Type: provider.PartReasoningStart, ID: "thinking"}, {Type: provider.PartReasoningDelta, ID: "thinking", Delta: "reasoning"}, {Type: provider.PartReasoningEnd, ID: "thinking"}, finishPart()}, status: 200, postCalls: 1, expectedOutput: []provider.ContentPart{provider.ReasoningPart("reasoning")}},
		{name: "source unsupported", parts: []provider.StreamPart{{Type: provider.PartSource, Source: &provider.SourceInfo{ID: "source", SourceType: provider.SourceTypeURL, URL: "https://example.com"}}, finishPart()}, status: 424},
		{name: "handler failure after provider error", parts: []provider.StreamPart{{Type: provider.PartError, APICallError: &provider.APICallError{Message: "private", StatusCode: 500}}, {Type: provider.PartTextDelta, ID: "unknown", Delta: "output-canary"}, finishPart()}, status: 424},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.StreamDrainDuration = 10 * time.Millisecond
			if tc.partLimit > 0 {
				limits.StreamParts = tc.partLimit
			}
			h := newRuntimeHarness(t, limits)
			var modelCtx context.Context
			h.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				modelCtx = ctx
				if !tc.noClose {
					return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
				}
				ch := make(chan provider.StreamPart, len(tc.parts))
				for _, p := range tc.parts {
					ch <- p
				}
				return &provider.StreamResult{Stream: ch}, nil
			}
			g := &testGuard{post: func(ctx context.Context, id string, _ provider.CallOptions, output []provider.ContentPart) error {
				assert.NoError(t, ctx.Err())
				assert.ErrorIs(t, modelCtx.Err(), context.Canceled)
				assert.Equal(t, "canonical/model", id)
				assert.Equal(t, tc.expectedOutput, output)
				return tc.postErr
			}}
			h.handler.guard = g
			h.handler.guardRetainedBytes = 1 << 20
			if tc.bytes > 0 {
				h.handler.guardRetainedBytes = tc.bytes
			}
			r := h.serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, r.Code, r.Body.String())
			assert.Equal(t, tc.postCalls, g.postCalls)
			assert.Equal(t, 1, h.model.callCount())
			assert.Equal(t, 1, g.releases)
			if tc.status == 200 {
				assert.Equal(t, "text/event-stream", r.Header().Get("Content-Type"))
				requireStreamBodyMatchesSchema(t, r.Body.String())
			} else {
				assert.NotContains(t, r.Body.String(), "output-canary")
				assert.NotContains(t, r.Body.String(), "data: ")
			}
			assert.ErrorIs(t, modelCtx.Err(), context.Canceled)
		})
	}
}

func TestGuardedStream_ReasoningMetadata(t *testing.T) {
	for _, tc := range guardedReasoningMetadataCases() {
		for _, location := range []provider.StreamPartType{provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd} {
			t.Run(tc.name+"/"+string(location), func(t *testing.T) {
				h := newRuntimeHarness(t, testLimits())
				parts := []provider.StreamPart{
					{Type: provider.PartReasoningStart, ID: "thinking"},
					{Type: provider.PartReasoningDelta, ID: "thinking", Delta: "represented"},
					{Type: provider.PartReasoningEnd, ID: "thinking"},
					finishPart(),
				}
				for i := range parts {
					if parts[i].Type == location {
						parts[i].ProviderMetadata = provider.ProviderMetadata{tc.namespace: json.RawMessage(tc.fields)}
					}
				}
				h.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(parts...)}, nil
				}
				g := &testGuard{}
				h.handler.guard, h.handler.guardRetainedBytes = g, 1<<20
				r := h.serve(streamRequest(`{"prompt":[]}`))
				if tc.opaque {
					assert.Equal(t, 424, r.Code, r.Body.String())
					assert.Zero(t, g.postCalls)
					assert.NotContains(t, r.Body.String(), "data: ")
					assert.NotContains(t, r.Body.String(), "opaque")
				} else {
					assert.Equal(t, 200, r.Code, r.Body.String())
					assert.Equal(t, 1, g.postCalls)
					assert.Contains(t, r.Body.String(), tc.fields)
				}
			})
		}
	}
}

func TestGuardedStream_WithholdingAndCancellation(t *testing.T) {
	for _, phase := range []string{"preflight", "postflight"} {
		t.Run(phase, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			h.handler.guardRetainedBytes = 1 << 20
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan struct{})
			resume := make(chan struct{})
			block := func(c context.Context) error { close(entered); <-resume; return c.Err() }
			g := &testGuard{}
			if phase == "preflight" {
				g.pre = func(c context.Context, _ string, o provider.CallOptions) (provider.CallOptions, error) {
					return o, block(c)
				}
			} else {
				g.post = func(c context.Context, _ string, _ provider.CallOptions, _ []provider.ContentPart) error {
					return block(c)
				}
			}
			h.handler.guard = g
			r := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); h.handler.ServeHTTP(r, streamRequest(`{"prompt":[]}`).WithContext(ctx)) }()
			<-entered
			assert.Empty(t, r.Header())
			assert.Empty(t, r.Body.String())
			cancel()
			close(resume)
			<-done
			assert.Equal(t, 499, r.Code)
			assert.NotContains(t, r.Body.String(), "data: ")
			if phase == "preflight" {
				assert.Zero(t, h.model.callCount())
			} else {
				assert.Equal(t, 1, h.model.callCount())
			}
		})
	}
}

func TestGuardedStream_TimeoutsAndLateResults(t *testing.T) {
	for _, tc := range []struct {
		name string
		late bool
		idle bool
	}{
		{name: "idle", idle: true}, {name: "total"}, {name: "late model result", late: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.ModelDuration = 60 * time.Millisecond
			limits.StreamIdleDuration = 10 * time.Millisecond
			limits.StreamDrainDuration = 10 * time.Millisecond
			if !tc.idle {
				limits.StreamIdleDuration = time.Second
			}
			h := newRuntimeHarness(t, limits)
			g := &testGuard{}
			h.handler.guard = g
			h.handler.guardRetainedBytes = 1 << 20
			returned := make(chan struct{})
			var ch chan provider.StreamPart
			h.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch = make(chan provider.StreamPart)
				if tc.late {
					<-ctx.Done()
				}
				close(returned)
				return &provider.StreamResult{Stream: ch}, nil
			}
			r := h.serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, 504, r.Code)
			assert.Zero(t, g.postCalls)
			assert.NotContains(t, r.Body.String(), "data: ")
			<-returned
			close(ch)
		})
	}
}

func BenchmarkGuardedFrames(b *testing.B) {
	for b.Loop() {
		sink := newGuardedFrames(8 << 20)
		h := &handler{limits: testLimits()}
		state := newStreamState(1000)
		_ = h.processStreamPart(sink, state, provider.StreamPart{Type: provider.PartTextStart, ID: "a"})
		for range 256 {
			_ = h.processStreamPart(sink, state, provider.StreamPart{Type: provider.PartTextDelta, ID: "a", Delta: strings.Repeat("x", 1024)})
		}
		_ = h.processStreamPart(sink, state, provider.StreamPart{Type: provider.PartTextEnd, ID: "a"})
		_ = h.processStreamPart(sink, state, finishPart())
		_ = sink.policyOutput()
	}
}

func TestGuardedStream_SelectedFallbackAndWriterFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		postErr error
		writer  *responseWriterProbe
		writes  int
		status  int
	}{
		{name: "postflight deny", postErr: ErrGuardDenied, writer: &responseWriterProbe{}, writes: 1, status: 403},
		{name: "short write", writer: &responseWriterProbe{shortWrite: true}, writes: 1, status: 200},
		{name: "frame write error", writer: &responseWriterProbe{writeErr: errors.New("private")}, writes: 1, status: 200},
		{name: "header flush error", writer: &responseWriterProbe{flushErrors: []error{errors.New("private")}}, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			selected, unused := &recordingModel{}, &recordingModel{}
			model, err := fallback.New(selected, unused)
			require.NoError(t, err)
			h.resolver.resolved.Model = model
			g := &testGuard{post: func(context.Context, string, provider.CallOptions, []provider.ContentPart) error { return tc.postErr }}
			h.handler.guard, h.handler.guardRetainedBytes = g, 1<<20
			h.handler.ServeHTTP(tc.writer, streamRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, tc.writer.status)
			assert.Equal(t, tc.writes, tc.writer.writes)
			assert.Equal(t, 1, selected.callCount())
			assert.Zero(t, unused.callCount())
			assert.Equal(t, 1, g.postCalls)
			assert.Equal(t, 1, g.releases)
		})
	}
}

func TestGuardedFrames_ByteBoundary(t *testing.T) {
	frame := []byte("data: {}\n\n")
	cost := int64(len(frame))*4 + 512
	for _, limit := range []int64{cost - 1, cost, cost + 1} {
		sink := newGuardedFrames(limit)
		assert.Equal(t, limit >= cost, sink.writeFrame(frame))
		assert.LessOrEqual(t, sink.used, limit)
	}
}

var _ http.ResponseWriter = (*guardedFrames)(nil)
