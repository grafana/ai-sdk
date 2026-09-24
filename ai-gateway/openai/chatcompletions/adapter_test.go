package chatcompletions

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
}

func (*fakeModel) SpecificationVersion() string               { return "v4" }
func (*fakeModel) Provider() string                           { return "private-provider" }
func (*fakeModel) ModelID() string                            { return "private-model" }
func (*fakeModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (f *fakeModel) DoGenerate(ctx context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
	return f.generate(ctx, o)
}
func (f *fakeModel) DoStream(ctx context.Context, o provider.CallOptions) (*provider.StreamResult, error) {
	return f.stream(ctx, o)
}
func testHandler(t *testing.T, f *fakeModel, change func(*Limits)) *handler {
	t.Helper()
	c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "canonical", Aliases: []string{"alias"}}, Model: f}})
	require.NoError(t, err)
	l := Limits{RequestBytes: 1 << 20, ResponseBytes: 1 << 20, FrameBytes: 1 << 16, StreamParts: 1024, ModelDuration: time.Second, IdleDuration: 200 * time.Millisecond, DrainDuration: 10 * time.Millisecond}
	if change != nil {
		change(&l)
	}
	h, err := New(Config{Resolver: c, Policies: map[string]Backend{"canonical": BackendResponses}, Limits: l})
	require.NoError(t, err)
	return h
}

const basic = `{"model":"alias","messages":[{"role":"user","content":"hello"}]}`

func requestWith(t *testing.T, extra string) mappedRequest {
	t.Helper()
	r, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + extra + "}"))
	require.NoError(t, err)
	return r
}
func textResult() *provider.GenerateResult {
	return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{InputTokens: provider.InputTokenUsage{Total: ptr(3), CacheRead: ptr(1)}, OutputTokens: provider.OutputTokenUsage{Total: ptr(2)}}}
}
func partsStream(parts ...provider.StreamPart) *provider.StreamResult {
	ch := make(chan provider.StreamPart, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	return &provider.StreamResult{Stream: ch}
}
func textParts() []provider.StreamPart {
	return []provider.StreamPart{{Type: provider.PartStreamStart}, {Type: provider.PartTextStart, ID: "text"}, {Type: provider.PartTextDelta, ID: "text", Delta: "hello"}, {Type: provider.PartTextEnd, ID: "text"}, {Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &textResult().Usage}}
}

func TestRequestDefaultsAndRejections(t *testing.T) {
	tool := `,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}]`
	for _, strict := range []string{"", `,"strict":null`, `,"strict":false`, `,"strict":true`} {
		t.Run("strict"+strict, func(t *testing.T) {
			raw := strings.Replace(tool, `"parameters"`, `"parameters"`, 1)
			raw = strings.Replace(raw, `"type":"object"}}}]`, `"type":"object"}`+strict+`}}]`, 1)
			r := requestWith(t, raw)
			require.NoError(t, r.applyPolicy(BackendResponses))
			require.NotNil(t, r.options.Tools[0].Strict)
			assert.Equal(t, strings.Contains(strict, "true"), *r.options.Tools[0].Strict)
			b, err := json.Marshal(r.options.ProviderOptions)
			require.NoError(t, err)
			assert.JSONEq(t, `{"openai":{"store":false,"strictJsonSchema":false}}`, string(b))
		})
	}
	for _, extra := range []string{`,"n":2`, `,"store":true`, `,"temperature":3`, `,"max_tokens":0`, `,"max_tokens":1,"max_completion_tokens":2`, `,"audio":{}`, `,"stream_options":{}`, `,"tool_choice":"required"`, `,"response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object","$ref":"https://private/"}}}}`, `,"Model":"other"`, `,"model":"other"`} {
		t.Run(extra, func(t *testing.T) {
			_, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + extra + "}"))
			require.Error(t, err)
		})
	}
	for _, body := range []string{`null`, basic + basic, `{"model":"x","messages":[{"role":"developer","content":"no"}]}`, `{"model":"x","messages":[{"role":"tool","tool_call_id":"orphan","content":"no"}]}`} {
		_, err := mapRequest([]byte(body))
		require.Error(t, err)
	}
	for _, backend := range []Backend{BackendAnthropic, BackendCompatible, BackendFallback} {
		r := requestWith(t, `,"reasoning_effort":"high"`)
		require.Error(t, r.applyPolicy(backend))
	}
	r := requestWith(t, tool)
	require.Error(t, r.applyPolicy(BackendFallback))
	r = requestWith(t, `,"reasoning_effort":"high"`)
	require.NoError(t, r.applyPolicy(BackendReasoning))
}

func TestContinuationAndStructuredValidation(t *testing.T) {
	raw := `{"model":"alias","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call","type":"function","function":{"name":"weather","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call","content":"sunny"},{"role":"user","content":[{"type":"text","text":"thanks"}]}]}`
	r, err := mapRequest([]byte(raw))
	require.NoError(t, err)
	require.NoError(t, r.applyPolicy(BackendResponses))
	assert.True(t, r.history)
	require.Len(t, r.options.Prompt, 3)
	assert.Equal(t, "weather", r.options.Prompt[1].Content[0].ToolName)
	_, err = mapRequest([]byte(strings.Replace(raw, `"tool_call_id":"call"`, `"tool_call_id":"bad"`, 1)))
	require.Error(t, err)
	r = requestWith(t, `,"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`)
	require.NoError(t, r.validateText(`{"answer":"yes"}`, "stop"))
	require.Error(t, r.validateText(`{"answer":1}`, "stop"))
	require.NoError(t, r.validateText(`{"answer":`, "length"))
	require.Error(t, r.validateText(`{"answer":`, "stop"))
}

func TestOfficialSDKUnaryStreamAndSafeErrors(t *testing.T) {
	f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return textResult(), nil
	}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(textParts()...), nil
	}}
	server := httptest.NewServer(testHandler(t, f, nil))
	defer server.Close()
	client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
	params := openaisdk.ChatCompletionNewParams{Model: "alias", Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("hello")}}
	result, err := client.Chat.Completions.New(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, "canonical", result.Model)
	assert.Equal(t, "hello", result.Choices[0].Message.Content)
	assert.Equal(t, int64(5), result.Usage.TotalTokens)
	assert.True(t, strings.HasPrefix(result.ID, "chatcmpl-"))
	params.StreamOptions = openaisdk.ChatCompletionStreamOptionsParam{IncludeUsage: openaisdk.Bool(true)}
	stream := client.Chat.Completions.NewStreaming(context.Background(), params)
	defer func() { _ = stream.Close() }()
	text := ""
	finishes := 0
	usages := 0
	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			text += choice.Delta.Content
			if choice.FinishReason != "" {
				finishes++
			}
		}
		if len(chunk.Choices) == 0 {
			usages++
			assert.Equal(t, int64(5), chunk.Usage.TotalTokens)
		}
	}
	require.NoError(t, stream.Err())
	assert.Equal(t, "hello", text)
	assert.Equal(t, 1, finishes)
	assert.Equal(t, 1, usages)
	params.StreamOptions = openaisdk.ChatCompletionStreamOptionsParam{}
	params.Model = "unknown"
	_, err = client.Chat.Completions.New(context.Background(), params)
	var api *openaisdk.Error
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 404, api.StatusCode)
	assert.NotContains(t, err.Error(), "private-provider")
}

func TestStreamFunctionState(t *testing.T) {
	r := requestWith(t, `,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}]`)
	s := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	parts := []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "call", ToolName: "weather"}, {Type: provider.PartToolInputDelta, ID: "call", Delta: `{"x":`}, {Type: provider.PartToolInputDelta, ID: "call", Delta: `1}`}, {Type: provider.PartToolInputEnd, ID: "call"}, {Type: provider.PartToolCall, ToolCallID: "call", ToolName: "weather", Input: `{"x":1}`}}
	var deltas []*delta
	for _, p := range parts {
		d, err := s.consume(p, r, 1<<20)
		require.NoError(t, err)
		if d != nil {
			deltas = append(deltas, d)
		}
	}
	require.Len(t, deltas, 3)
	assert.Equal(t, "call", deltas[0].ToolCalls[0].ID)
	assert.Empty(t, deltas[1].ToolCalls[0].ID)
	_, _, err := s.finish(provider.StreamPart{FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, r)
	require.NoError(t, err)
	_, err = s.consume(parts[4], r, 1<<20)
	require.Error(t, err)
	bad := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	for _, p := range parts[:4] {
		_, err := bad.consume(p, r, 1<<20)
		require.NoError(t, err)
	}
	mismatch := parts[4]
	mismatch.Input = `{"x":2}`
	_, err = bad.consume(mismatch, r, 1<<20)
	require.Error(t, err)
}

func TestUnsupportedProfilesDoNotInvokeModel(t *testing.T) {
	for _, tc := range []struct {
		backend Backend
		extra   string
	}{
		{BackendAnthropic, `,"temperature":2`}, {BackendAnthropic, `,"max_tokens":4097`}, {BackendCompatible, `,"response_format":{"type":"json_object"}`}, {BackendReasoning, `,"temperature":0`}, {BackendResponses, `,"seed":1`}, {BackendResponses, `,"reasoning_effort":"high"`}, {BackendFallback, `,"tool_choice":"none"`},
	} {
		t.Run(string(tc.backend)+tc.extra, func(t *testing.T) {
			calls := 0
			f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				calls++
				return textResult(), nil
			}}
			h := testHandler(t, f, nil)
			h.policies["canonical"] = tc.backend
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", Path, strings.NewReader(strings.TrimSuffix(basic, "}")+tc.extra+"}"))
			req.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(w, req)
			assert.Equal(t, 400, w.Code)
			assert.Zero(t, calls)
		})
	}
}

func TestRuntimeFailureBoundsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		parts  []provider.StreamPart
		modify func(*Limits)
	}{
		{name: "premature", parts: textParts()[:3]},
		{name: "unrepresented stop", parts: []provider.StreamPart{{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}}},
		{name: "duplicate finish", parts: append(textParts(), textParts()[4])},
		{name: "late nonterminal", parts: append(textParts(), provider.StreamPart{Type: provider.PartTextStart, ID: "late"})},
		{name: "invalid UTF8", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "text"}, {Type: provider.PartTextDelta, ID: "text", Delta: string([]byte{0xff})}}},
		{name: "contradiction", parts: []provider.StreamPart{{Type: provider.PartTextDelta, ID: "missing", Delta: "secret"}}},
		{name: "part bound", parts: textParts(), modify: func(l *Limits) { l.StreamParts = 2 }},
		{name: "unsupported content", parts: []provider.StreamPart{{Type: provider.PartSource}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(tc.parts...), nil
			}}
			server := httptest.NewServer(testHandler(t, f, tc.modify))
			defer server.Close()
			res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
			require.NoError(t, err)
			defer func() { _ = res.Body.Close() }()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), `"error"`)
			assert.NotContains(t, string(body), "[DONE]")
			assert.NotContains(t, string(body), "secret")
		})
	}
	t.Run("discarded progress times out", func(t *testing.T) {
		stopped := make(chan struct{})
		f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart)
			go func() {
				defer close(stopped)
				defer close(ch)
				for {
					select {
					case <-ctx.Done():
						return
					case ch <- provider.StreamPart{Type: provider.PartReasoningDelta, Delta: "x"}:
					}
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}
		server := httptest.NewServer(testHandler(t, f, func(l *Limits) {
			l.IdleDuration = 20 * time.Millisecond
			l.StreamParts = 1000000
			l.ResponseBytes = 1 << 30
		}))
		defer server.Close()
		res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		require.NoError(t, err)
		assert.Contains(t, string(body), `"timeout"`)
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("producer not cancelled")
		}
	})
	t.Run("unary concurrent load", func(t *testing.T) {
		var calls atomic.Int64
		f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			calls.Add(1)
			return textResult(), nil
		}}
		h := testHandler(t, f, nil)
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				w := httptest.NewRecorder()
				req := httptest.NewRequest("POST", Path, strings.NewReader(basic))
				req.Header.Set("Content-Type", "application/json")
				h.ServeHTTP(w, req)
				assert.Equal(t, 200, w.Code)
			})
		}
		wg.Wait()
		assert.Equal(t, int64(32), calls.Load())
	})
}

func TestLateSetupAndSlowClientAreBounded(t *testing.T) {
	t.Run("late setup ownership", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		drained := make(chan struct{})
		f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			close(started)
			<-release
			ch := make(chan provider.StreamPart)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "late"}
				close(drained)
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}
		h := testHandler(t, f, nil)
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("POST", Path, strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		done := make(chan struct{})
		go func() { defer close(done); h.ServeHTTP(httptest.NewRecorder(), req) }()
		<-started
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("handler waited for setup")
		}
		close(release)
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Fatal("late stream was not drained")
		}
	})
	t.Run("slow socket client", func(t *testing.T) {
		stopped := make(chan struct{})
		f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart)
			go func() {
				defer close(stopped)
				defer close(ch)
				select {
				case ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}:
				case <-ctx.Done():
					return
				}
				for {
					select {
					case <-ctx.Done():
						return
					case ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: strings.Repeat("x", 32768)}:
					}
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}
		server := httptest.NewServer(testHandler(t, f, func(l *Limits) {
			l.ModelDuration = 150 * time.Millisecond
			l.IdleDuration = 100 * time.Millisecond
			l.ResponseBytes = 1 << 30
			l.StreamParts = 1000000
		}))
		defer server.Close()
		conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		body := strings.TrimSuffix(basic, "}") + `,"stream":true}`
		_, err = io.WriteString(conn, "POST "+Path+" HTTP/1.1\r\nHost: local\r\nContent-Type: application/json\r\nContent-Length: "+strconv.Itoa(len(body))+"\r\n\r\n"+body)
		require.NoError(t, err)
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("slow client retained provider")
		}
	})
}

func TestUsageAndNativeByteBounds(t *testing.T) {
	u, err := mapUsage(provider.Usage{})
	require.NoError(t, err)
	assert.Nil(t, u)
	for _, value := range []provider.Usage{{InputTokens: provider.InputTokenUsage{Total: ptr(-1)}}, {InputTokens: provider.InputTokenUsage{Total: ptr(1), CacheRead: ptr(2)}, OutputTokens: provider.OutputTokenUsage{Total: ptr(1)}}} {
		_, err = mapUsage(value)
		require.Error(t, err)
	}
	for _, tc := range []struct {
		name   string
		body   string
		limit  func(*Limits)
		status int
	}{
		{"request", basic, func(l *Limits) { l.RequestBytes = 20 }, 413},
		{"unary", basic, func(l *Limits) { l.ResponseBytes = 512 }, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				v := textResult()
				v.Content[0].Text = strings.Repeat("x", 1000)
				return v, nil
			}}
			h := testHandler(t, f, tc.limit)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", Path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(w, req)
			assert.Equal(t, tc.status, w.Code)
			assert.Less(t, w.Body.Len(), 512)
		})
	}
	t.Run("oversized frame emits safe error", func(t *testing.T) {
		parts := textParts()
		parts[2].Delta = strings.Repeat("x", 2000)
		f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return partsStream(parts...), nil
		}}
		server := httptest.NewServer(testHandler(t, f, func(l *Limits) { l.FrameBytes = 512 }))
		defer server.Close()
		res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
		require.NoError(t, err)
		defer func() { _ = res.Body.Close() }()
		b, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.Contains(t, string(b), `"error"`)
		assert.NotContains(t, string(b), "[DONE]")
		assert.NotContains(t, string(b), strings.Repeat("x", 50))
	})
}

func TestStreamingCancellationStorm(t *testing.T) {
	var started, stopped atomic.Int64
	f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		started.Add(1)
		ch := make(chan provider.StreamPart)
		go func() { <-ctx.Done(); close(ch); stopped.Add(1) }()
		return &provider.StreamResult{Stream: ch}, nil
	}}
	server := httptest.NewServer(testHandler(t, f, nil))
	defer server.Close()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
			if assert.NoError(t, err) {
				_ = res.Body.Close()
			}
		})
	}
	wg.Wait()
	require.Eventually(t, func() bool { return stopped.Load() == 32 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int64(32), started.Load())
}
