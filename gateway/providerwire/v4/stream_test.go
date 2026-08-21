package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_StreamNormalizesLifecycle(t *testing.T) {
	for _, includeProviderStart := range []bool{true, false} {
		name := "without provider start"
		if includeProviderStart {
			name = "with provider start"
		}
		t.Run(name, func(t *testing.T) {
			parts := []provider.StreamPart{}
			if includeProviderStart {
				parts = append(parts, provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{}})
			}
			parts = append(parts,
				provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "response-1", ModelID: "backend-model", Provider: "private-provider", ResponseHeaders: map[string]string{"Authorization": "secret"}},
				provider.StreamPart{Type: provider.PartTextStart, ID: "text-1", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":true}`)}},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: ""},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"},
				validFinishPart(),
			)
			model := streamModel(parts, nil)
			handler := newTestHandler(t, model)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, MIMESSE, response.Header().Get("Content-Type"))
			assert.Equal(t, "no-cache, no-transform", response.Header().Get("Cache-Control"))
			assert.Empty(t, response.Header().Get("Connection"))
			assert.True(t, response.Flushed)
			assert.NotContains(t, response.Body.String(), "[DONE]")
			decoded := decodeFrames(t, response.Body.String())
			require.Len(t, decoded, 6)
			assert.Equal(t, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}, frameTypes(decoded))
			assert.Equal(t, []any{}, decoded[0]["warnings"])
			assert.Equal(t, "public/canonical", decoded[1]["modelId"])
			assert.Equal(t, "response-1", decoded[1]["id"])
			assert.Equal(t, "", decoded[3]["delta"])
			for _, private := range []string{"backend-model", "private-provider", "Authorization", "secret", "providerMetadata"} {
				assert.NotContains(t, response.Body.String(), private)
			}
		})
	}
}

func TestHandler_StreamAllowsSequentialTextBlocks(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "one"},
		{Type: provider.PartTextDelta, ID: "one", Delta: "a"},
		{Type: provider.PartTextEnd, ID: "one"},
		{Type: provider.PartTextStart, ID: "two"},
		{Type: provider.PartTextDelta, ID: "two", Delta: "b"},
		{Type: provider.PartTextEnd, ID: "two"},
		validFinishPart(),
	}
	response := httptest.NewRecorder()
	newTestHandler(t, streamModel(parts, nil)).ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
	assert.Equal(t, []string{"stream-start", "text-start", "text-delta", "text-end", "text-start", "text-delta", "text-end", "finish"}, frameTypes(decodeFrames(t, response.Body.String())))
}

func TestHandler_StreamLifecycleFailuresAreTerminal(t *testing.T) {
	warning := provider.Warning{Type: provider.WarnOther, Message: "private warning"}
	mismatchFinish := validFinishPart()
	mismatchFinish.Warnings = []provider.Warning{warning}
	tests := []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"start warnings", []provider.StreamPart{{Type: provider.PartStreamStart, Warnings: []provider.Warning{warning}}}},
		{"finish warnings", []provider.StreamPart{mismatchFinish}},
		{"mismatched delta", []provider.StreamPart{{Type: provider.PartTextStart, ID: "one"}, {Type: provider.PartTextDelta, ID: "two", Delta: "private"}}},
		{"warnings on text", []provider.StreamPart{{Type: provider.PartTextStart, ID: "one", Warnings: []provider.Warning{warning}}}},
		{"late provider start", []provider.StreamPart{{Type: provider.PartTextStart, ID: "one"}, {Type: provider.PartStreamStart}}},
		{"late response metadata", []provider.StreamPart{{Type: provider.PartTextStart, ID: "one"}, {Type: provider.PartTextEnd, ID: "one"}, {Type: provider.PartResponseMeta}}},
		{"empty text ID", []provider.StreamPart{{Type: provider.PartTextStart}}},
		{"finish during text", []provider.StreamPart{{Type: provider.PartTextStart, ID: "one"}, validFinishPart()}},
		{"part after finish", []provider.StreamPart{validFinishPart(), {Type: provider.PartTextStart, ID: "late"}}},
		{"unsupported", []provider.StreamPart{{Type: provider.PartReasoningStart, ID: "private"}}},
		{"premature close", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			newTestHandler(t, streamModel(tc.parts, nil)).ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
			assert.Equal(t, http.StatusOK, response.Code)
			decoded := decodeFrames(t, response.Body.String())
			require.NotEmpty(t, decoded)
			assert.Equal(t, "stream-start", decoded[0]["type"])
			assert.Equal(t, "error", decoded[len(decoded)-1]["type"])
			assert.Equal(t, 1, countType(decoded, "error"))
			assert.NotContains(t, response.Body.String(), "private")
		})
	}
}

func TestHandler_StreamSetupIsBoundedWhenModelIgnoresContext(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	model := &testModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		close(started)
		<-release
		parts := make(chan provider.StreamPart)
		return &provider.StreamResult{Stream: parts}, nil
	}}
	handler := newTestHandler(t, model, WithTotalTimeout(20*time.Millisecond))
	response := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, http.StatusGatewayTimeout, response.Code)
	assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
	assert.NotContains(t, response.Body.String(), "stream-start")
	<-started
	close(release)
}

func TestHandler_StreamCancellationWinsReturnedSetup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	model := &testModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		close(started)
		<-release
		parts := make(chan provider.StreamPart)
		return &provider.StreamResult{Stream: parts}, nil
	}}
	handler := newTestHandler(t, model)
	ctx, cancel := context.WithCancel(context.Background())
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true).WithContext(ctx))
		close(done)
	}()
	<-started
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after cancellation")
	}
	assert.Equal(t, 499, response.Code)
	assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
	assert.NotContains(t, response.Body.String(), "stream-start")
}

func TestMapReceivedStreamPart_ContextWinsReadyValueErrorAndClose(t *testing.T) {
	apiError := provider.NewAPICallError(provider.APICallErrorOptions{Message: "private", StatusCode: http.StatusTooManyRequests})
	cases := []struct {
		name string
		part provider.StreamPart
		open bool
	}{
		{"part", provider.StreamPart{Type: provider.PartTextStart, ID: "ready"}, true},
		{"error", provider.StreamPart{Type: provider.PartError, APICallError: apiError}, true},
		{"close", provider.StreamPart{}, false},
	}
	for _, deadline := range []bool{false, true} {
		for _, tc := range cases {
			name := "cancellation/" + tc.name
			var ctx context.Context
			var cancel context.CancelFunc
			wantCategory := failure.CategoryCancellation
			if deadline {
				name = "deadline/" + tc.name
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				wantCategory = failure.CategoryTimeout
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			t.Run(name, func(t *testing.T) {
				defer cancel()
				_, _, terminal, closed := mapReceivedStreamPart(ctx, &streamState{finishSeen: true}, tc.part, tc.open, "public/canonical")
				require.NotNil(t, terminal)
				assert.Equal(t, wantCategory, terminal.Category())
				assert.False(t, closed)
			})
		}
	}
}

func TestMapStreamPart_RejectsInvalidUTF8(t *testing.T) {
	invalid := string([]byte{0xff})
	finish := validFinishPart()
	finish.FinishReason.Raw = invalid
	tests := []struct {
		name    string
		state   streamState
		part    provider.StreamPart
		modelID string
	}{
		{"canonical model ID", streamState{}, provider.StreamPart{Type: provider.PartTextStart, ID: "id"}, invalid},
		{"response ID", streamState{}, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: invalid}, "public/canonical"},
		{"response model ID", streamState{}, provider.StreamPart{Type: provider.PartResponseMeta, ModelID: invalid}, "public/canonical"},
		{"response provider", streamState{}, provider.StreamPart{Type: provider.PartResponseMeta, Provider: invalid}, "public/canonical"},
		{"text start ID", streamState{}, provider.StreamPart{Type: provider.PartTextStart, ID: invalid}, "public/canonical"},
		{"text delta ID", streamState{activeTextID: invalid}, provider.StreamPart{Type: provider.PartTextDelta, ID: invalid, Delta: "x"}, "public/canonical"},
		{"text delta", streamState{activeTextID: "id"}, provider.StreamPart{Type: provider.PartTextDelta, ID: "id", Delta: invalid}, "public/canonical"},
		{"text end ID", streamState{activeTextID: invalid}, provider.StreamPart{Type: provider.PartTextEnd, ID: invalid}, "public/canonical"},
		{"raw finish reason", streamState{}, finish, "public/canonical"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, terminal := mapStreamPart(context.Background(), &tc.state, tc.part, tc.modelID)
			require.NotNil(t, terminal)
			assert.Equal(t, failure.CategoryInternalFailure, terminal.Category())
		})
	}
}

func TestHandler_StreamCommitmentAndProviderFailure(t *testing.T) {
	t.Run("setup error remains JSON", func(t *testing.T) {
		model := streamModel(nil, errors.New("private setup failure"))
		response := httptest.NewRecorder()
		newTestHandler(t, model).ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		assert.Equal(t, http.StatusBadGateway, response.Code)
		assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
		assert.NotContains(t, response.Body.String(), "private")
	})

	t.Run("nil stream remains JSON", func(t *testing.T) {
		model := &testModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{}, nil
		}}
		response := httptest.NewRecorder()
		newTestHandler(t, model).ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
	})

	t.Run("first invalid part is post commit", func(t *testing.T) {
		response := httptest.NewRecorder()
		newTestHandler(t, streamModel([]provider.StreamPart{{Type: provider.PartRaw, RawValue: json.RawMessage(`{"private":true}`)}}, nil)).ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, []string{"stream-start", "error"}, frameTypes(decodeFrames(t, response.Body.String())))
	})

	t.Run("provider error is safely reduced", func(t *testing.T) {
		retryable := false
		apiError := provider.NewAPICallError(provider.APICallErrorOptions{
			Message:           "private-message",
			URL:               "https://private-url",
			RequestBodyValues: json.RawMessage(`{"privateRequest":true}`),
			ResponseHeaders:   map[string][]string{"X-Private": {"private-header"}},
			ResponseBody:      "private-body",
			StatusCode:        401,
			IsRetryable:       &retryable,
			Data:              json.RawMessage(`{"privateData":true}`),
			Cause:             errors.New("private-cause"),
		})
		response := httptest.NewRecorder()
		newTestHandler(t, streamModel([]provider.StreamPart{{Type: provider.PartError, APICallError: apiError}}, nil)).ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		decoded := decodeFrames(t, response.Body.String())
		require.Len(t, decoded, 2)
		errorObject := decoded[1]["error"].(map[string]any)
		assert.Equal(t, float64(http.StatusFailedDependency), errorObject["statusCode"])
		assert.Equal(t, false, errorObject["retryable"])
		assert.Equal(t, "failed_dependency", errorObject["type"])
		assert.Equal(t, "provider request failed", errorObject["message"])
		assert.NotContains(t, response.Body.String(), "private")
	})
}

func TestIdleExpired_RejectsPartsAtOrAfterDeadline(t *testing.T) {
	deadline := time.Unix(10, 0)
	assert.False(t, idleExpired(deadline.Add(-time.Nanosecond), deadline))
	assert.True(t, idleExpired(deadline, deadline))
	assert.True(t, idleExpired(deadline.Add(time.Nanosecond), deadline))
}

func TestHandler_StreamLimitsTimeoutsAndCancellation(t *testing.T) {
	t.Run("complete frame alone crosses limit", func(t *testing.T) {
		part := textStartDTO{Type: streamPartTextStart, ID: strings.Repeat("x", len(canonicalErrorFrame))}
		body, err := json.Marshal(part)
		require.NoError(t, err)
		limit := int64(len(body) + len("data: ") + len("\n"))
		require.GreaterOrEqual(t, limit, int64(len(canonicalErrorFrame)))
		require.Less(t, int64(len(body)), limit)
		require.Greater(t, int64(len(appendFrame(body))), limit)
		parts := make(chan provider.StreamPart, 1)
		parts <- provider.StreamPart{Type: provider.PartTextStart, ID: part.ID}
		canceled := make(chan struct{})
		model := &testModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			go func() { <-ctx.Done(); close(canceled) }()
			return &provider.StreamResult{Stream: parts}, nil
		}}
		handler := newTestHandler(t, model, WithMaxEventBytes(limit))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		assert.Equal(t, []string{"stream-start", "error"}, frameTypes(decodeFrames(t, response.Body.String())))
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("event overflow did not cancel model context")
		}
	})

	t.Run("idle timeout covers first part", func(t *testing.T) {
		parts := make(chan provider.StreamPart)
		canceled := make(chan struct{})
		model := &testModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			go func() { <-ctx.Done(); close(canceled) }()
			return &provider.StreamResult{Stream: parts}, nil
		}}
		handler := newTestHandler(t, model, WithIdleTimeout(10*time.Millisecond))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		decoded := decodeFrames(t, response.Body.String())
		assert.Equal(t, []string{"stream-start", "error"}, frameTypes(decoded))
		assert.Equal(t, float64(http.StatusGatewayTimeout), decoded[1]["error"].(map[string]any)["statusCode"])
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("idle timeout did not cancel model context")
		}
	})

	t.Run("provider start does not reset near-deadline idle timeout", func(t *testing.T) {
		parts := make(chan provider.StreamPart)
		model := &testModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			go func() {
				time.Sleep(80 * time.Millisecond)
				parts <- provider.StreamPart{Type: provider.PartStreamStart}
				time.Sleep(150 * time.Millisecond)
				close(parts)
			}()
			return &provider.StreamResult{Stream: parts}, nil
		}}
		handler := newTestHandler(t, model, WithIdleTimeout(200*time.Millisecond), WithTotalTimeout(time.Second))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		decoded := decodeFrames(t, response.Body.String())
		assert.Equal(t, []string{"stream-start", "error"}, frameTypes(decoded))
		assert.Equal(t, float64(http.StatusGatewayTimeout), decoded[1]["error"].(map[string]any)["statusCode"])
	})

	t.Run("activity resets idle timeout", func(t *testing.T) {
		parts := make(chan provider.StreamPart)
		model := &testModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			go func() {
				defer close(parts)
				for _, part := range []provider.StreamPart{{Type: provider.PartTextStart, ID: "one"}, {Type: provider.PartTextDelta, ID: "one", Delta: "x"}, {Type: provider.PartTextEnd, ID: "one"}, validFinishPart()} {
					time.Sleep(40 * time.Millisecond)
					parts <- part
				}
			}()
			return &provider.StreamResult{Stream: parts}, nil
		}}
		handler := newTestHandler(t, model, WithIdleTimeout(100*time.Millisecond), WithTotalTimeout(time.Second))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		assert.Equal(t, "finish", frameTypes(decodeFrames(t, response.Body.String()))[4])
	})

	t.Run("total timeout", func(t *testing.T) {
		parts := make(chan provider.StreamPart)
		canceled := make(chan struct{})
		model := &testModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			go func() { <-ctx.Done(); close(canceled) }()
			return &provider.StreamResult{Stream: parts}, nil
		}}
		handler := newTestHandler(t, model, WithTotalTimeout(10*time.Millisecond), WithIdleTimeout(time.Second))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true))
		decoded := decodeFrames(t, response.Body.String())
		assert.Equal(t, float64(http.StatusGatewayTimeout), decoded[1]["error"].(map[string]any)["statusCode"])
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("total timeout did not cancel model context")
		}
	})

	t.Run("request cancellation after commitment", func(t *testing.T) {
		parts := make(chan provider.StreamPart)
		model := &testModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: parts}, nil
		}}
		handler := newTestHandler(t, model, WithIdleTimeout(time.Second))
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, true).WithContext(ctx))
		decoded := decodeFrames(t, response.Body.String())
		assert.Equal(t, float64(499), decoded[1]["error"].(map[string]any)["statusCode"])
		assert.Equal(t, false, decoded[1]["error"].(map[string]any)["retryable"])
	})
}

func TestHandler_StreamFlushesEveryFrame(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "one"},
		{Type: provider.PartTextEnd, ID: "one"},
		validFinishPart(),
	}
	writer := &failingResponseWriter{}
	newTestHandler(t, streamModel(parts, nil)).ServeHTTP(writer, validRequest(`{"prompt":[]}`, true))
	assert.Equal(t, 4, writer.writes)
	assert.Equal(t, writer.writes, writer.flushes)
}

func TestHandler_StreamWriterFailureCancelsAndDoesNotWriteAgain(t *testing.T) {
	tests := []struct {
		name    string
		failAt  int
		shortAt int
		writes  int
	}{
		{"start write failure", 1, 0, 1},
		{"event write failure", 2, 0, 2},
		{"event short write", 0, 2, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "one"}
			canceled := make(chan struct{})
			model := &testModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				go func() { <-ctx.Done(); close(canceled) }()
				return &provider.StreamResult{Stream: parts}, nil
			}}
			handler := newTestHandler(t, model)
			writer := &failingResponseWriter{failAt: tc.failAt, shortAt: tc.shortAt}
			handler.ServeHTTP(writer, validRequest(`{"prompt":[]}`, true))
			assert.Equal(t, tc.writes, writer.writes)
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("model context was not canceled")
			}
		})
	}
}

func streamModel(parts []provider.StreamPart, callError error) *testModel {
	return &testModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		if callError != nil {
			return nil, callError
		}
		stream := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			stream <- part
		}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
}

func decodeFrames(t *testing.T, body string) []map[string]any {
	t.Helper()
	trimmed := strings.TrimSuffix(body, "\n\n")
	if trimmed == "" {
		return nil
	}
	frames := strings.Split(trimmed, "\n\n")
	decoded := make([]map[string]any, 0, len(frames))
	for _, frame := range frames {
		require.True(t, strings.HasPrefix(frame, "data: "), frame)
		var value map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &value))
		decoded = append(decoded, value)
	}
	return decoded
}

func frameTypes(frames []map[string]any) []string {
	result := make([]string, 0, len(frames))
	for _, frame := range frames {
		result = append(result, frame["type"].(string))
	}
	return result
}

func countType(frames []map[string]any, partType string) int {
	count := 0
	for _, frame := range frames {
		if frame["type"] == partType {
			count++
		}
	}
	return count
}

type failingResponseWriter struct {
	header  http.Header
	body    bytes.Buffer
	writes  int
	flushes int
	failAt  int
	shortAt int
	mu      sync.Mutex
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *failingResponseWriter) WriteHeader(int) {}
func (w *failingResponseWriter) Write(body []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	if w.writes == w.failAt {
		return 0, errors.New("writer failed")
	}
	if w.writes == w.shortAt {
		return len(body) - 1, nil
	}
	return w.body.Write(body)
}
func (w *failingResponseWriter) Flush() { w.flushes++ }
