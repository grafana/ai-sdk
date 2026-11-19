package responses

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "private-provider-secret"

type fakeModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	parts    []provider.StreamPart
}

func (*fakeModel) SpecificationVersion() string               { return "v4" }
func (*fakeModel) Provider() string                           { return "private-provider" }
func (*fakeModel) ModelID() string                            { return "private-model" }
func (*fakeModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (f *fakeModel) DoGenerate(ctx context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
	return f.generate(ctx, o)
}
func (f *fakeModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	ch := make(chan provider.StreamPart, len(f.parts))
	for _, p := range f.parts {
		ch <- p
	}
	close(ch)
	return &provider.StreamResult{Stream: ch}, nil
}

func serve(t *testing.T, f *fakeModel, raw string) *httptest.ResponseRecorder {
	t.Helper()
	c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "canonical", Aliases: []string{"alias"}}, Model: execution.Translate(f, execution.Target{Type: execution.TargetOpenAI})}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", Path, strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	return w
}

type sseEvent struct {
	name string
	data map[string]any
}

func events(t *testing.T, w *httptest.ResponseRecorder) []sseEvent {
	t.Helper()
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	var out []sseEvent
	for _, frame := range strings.Split(strings.TrimSpace(w.Body.String()), "\n\n") {
		name, data, ok := strings.Cut(frame, "\ndata: ")
		require.True(t, ok, frame)
		var ev sseEvent
		ev.name = strings.TrimPrefix(name, "event: ")
		require.NoError(t, json.Unmarshal([]byte(data), &ev.data))
		require.Equal(t, ev.name, ev.data["type"])
		require.EqualValues(t, len(out), ev.data["sequence_number"])
		out = append(out, ev)
	}
	return out
}

func types(evs []sseEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.name
	}
	return out
}

func finishPart(reason provider.UnifiedFinishReason) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: reason}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: ptr(10), CacheRead: ptr(4)}, OutputTokens: provider.OutputTokenUsage{Total: ptr(7), Reasoning: ptr(3)}}}
}

func TestStream_EventOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
		parts []provider.StreamPart
		want  []string
		check func(t *testing.T, terminal map[string]any)
	}{
		{"text", "", []provider.StreamPart{
			{Type: provider.PartStreamStart},
			{Type: provider.PartTextStart, ID: "t"},
			{Type: provider.PartTextDelta, ID: "t", Delta: "Hel"},
			{Type: provider.PartTextDelta, ID: "t", Delta: ""},
			{Type: provider.PartTextDelta, ID: "t", Delta: "lo"},
			{Type: provider.PartTextEnd, ID: "t"},
			finishPart(provider.FinishReasonStop),
		}, []string{"response.created", "response.in_progress", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"}, func(t *testing.T, terminal map[string]any) {
			output := terminal["output"].([]any)
			require.Len(t, output, 1)
			assert.Equal(t, "Hello", output[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
			assert.JSONEq(t, `{"input_tokens":10,"input_tokens_details":{"cached_tokens":4},"output_tokens":7,"output_tokens_details":{"reasoning_tokens":3},"total_tokens":17}`, string(must(json.Marshal(terminal["usage"]))))
		}},
		{"function call", `,"tools":[` + weatherTool + `]`, []provider.StreamPart{
			{Type: provider.PartToolInputStart, ID: "call_1", ToolName: "weather"},
			{Type: provider.PartToolInputDelta, ID: "call_1", Delta: `{"city":`},
			{Type: provider.PartToolInputDelta, ID: "call_1", Delta: `"Rio"}`},
			{Type: provider.PartToolInputEnd, ID: "call_1"},
			{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "weather", Input: `{"city":"Rio"}`},
			finishPart(provider.FinishReasonToolCalls),
		}, []string{"response.created", "response.in_progress", "response.output_item.added", "response.function_call_arguments.delta", "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.output_item.done", "response.completed"}, func(t *testing.T, terminal map[string]any) {
			item := terminal["output"].([]any)[0].(map[string]any)
			assert.Equal(t, "call_1", item["call_id"])
			assert.Equal(t, `{"city":"Rio"}`, item["arguments"])
		}},
		{"reasoning then text", "", []provider.StreamPart{
			{Type: provider.PartReasoningStart, ID: "r:0"},
			{Type: provider.PartReasoningDelta, ID: "r:0", Delta: "think"},
			{Type: provider.PartReasoningEnd, ID: "r:0"},
			{Type: provider.PartReasoningStart, ID: "r:1"},
			{Type: provider.PartReasoningEnd, ID: "r:1"},
			{Type: provider.PartTextStart, ID: "t"},
			{Type: provider.PartTextDelta, ID: "t", Delta: "done"},
			{Type: provider.PartTextEnd, ID: "t"},
			finishPart(provider.FinishReasonStop),
		}, []string{"response.created", "response.in_progress", "response.output_item.added", "response.reasoning_summary_part.added", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done", "response.reasoning_summary_part.done", "response.output_item.done", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"}, func(t *testing.T, terminal map[string]any) {
			reasoning := terminal["output"].([]any)[0].(map[string]any)
			assert.Equal(t, []any{map[string]any{"type": "summary_text", "text": "think"}}, reasoning["summary"])
			assert.NotContains(t, reasoning, "encrypted_content")
		}},
		{"length finish", "", []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "t"},
			{Type: provider.PartTextDelta, ID: "t", Delta: "cut"},
			{Type: provider.PartTextEnd, ID: "t"},
			finishPart(provider.FinishReasonLength),
		}, []string{"response.created", "response.in_progress", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.incomplete"}, func(t *testing.T, terminal map[string]any) {
			assert.Equal(t, "incomplete", terminal["status"])
			assert.Equal(t, map[string]any{"reason": "max_output_tokens"}, terminal["incomplete_details"])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serve(t, &fakeModel{parts: tc.parts}, body(`,"stream":true`+tc.extra))
			require.Equal(t, 200, w.Code)
			evs := events(t, w)
			assert.Equal(t, tc.want, types(evs))
			tc.check(t, evs[len(evs)-1].data["response"].(map[string]any))
		})
	}
}

func TestStream_Failures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
		code  string
	}{
		{"provider error", []provider.StreamPart{{Type: provider.PartTextStart, ID: "t"}, {Type: provider.PartError, APICallError: &provider.APICallError{StatusCode: 429, Message: secret}}}, "rate_limit_exceeded"},
		{"in-band error", []provider.StreamPart{{Type: provider.PartError, APICallError: &provider.APICallError{Message: secret}}}, "upstream_error"},
		{"undeclared tool", []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "c", ToolName: "other", Input: "{}"}, finishPart(provider.FinishReasonToolCalls)}, "upstream_error"},
		{"no finish", []provider.StreamPart{{Type: provider.PartTextStart, ID: "t"}}, "upstream_error"},
		{"runtime lifecycle violation", []provider.StreamPart{{Type: provider.PartTextStart, ID: "t"}, finishPart(provider.FinishReasonStop)}, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serve(t, &fakeModel{parts: tc.parts}, body(`,"stream":true`))
			assert.NotContains(t, w.Body.String(), secret)
			evs := events(t, w)
			last := evs[len(evs)-1]
			require.Equal(t, "response.failed", last.name)
			response := last.data["response"].(map[string]any)
			assert.Equal(t, "failed", response["status"])
			assert.Equal(t, tc.code, response["error"].(map[string]any)["code"])
		})
	}
}

func TestUnary(t *testing.T) {
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentReasoning, Text: "think"},
			{Type: provider.ContentText, Text: "Let me "},
			{Type: provider.ContentText, Text: "check."},
			{Type: provider.ContentToolCall, ToolCallID: "call_1", ToolName: "weather", Input: json.RawMessage(`{"city":"Rio"}`)},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls},
		Usage:        provider.Usage{InputTokens: provider.InputTokenUsage{Total: ptr(3)}, OutputTokens: provider.OutputTokenUsage{Total: ptr(2)}},
	}
	w := serve(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}, body(`,"tools":[`+weatherTool+`]`))
	require.Equal(t, 200, w.Code, w.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "completed", got["status"])
	output := got["output"].([]any)
	require.Len(t, output, 3)
	assert.Equal(t, "reasoning", output[0].(map[string]any)["type"])
	assert.Equal(t, "Let me check.", output[1].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
	call := output[2].(map[string]any)
	assert.Equal(t, "function_call", call["type"])
	assert.Equal(t, "call_1", call["call_id"])
	assert.Equal(t, "weather", call["name"])
	assert.Equal(t, `{"city":"Rio"}`, call["arguments"])
	assert.JSONEq(t, `{"input_tokens":3,"output_tokens":2,"total_tokens":5}`, string(must(json.Marshal(got["usage"]))))

	result.FinishReason.Unified = provider.FinishReasonLength
	result.Content = result.Content[1:3]
	w = serve(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}, body(""))
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "incomplete", got["status"])
	assert.Equal(t, map[string]any{"reason": "max_output_tokens"}, got["incomplete_details"])

	w = serve(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, &provider.APICallError{StatusCode: 429, Message: secret}
	}}, body(""))
	assert.Equal(t, 429, w.Code)
	assert.NotContains(t, w.Body.String(), secret)
	assert.JSONEq(t, `{"error":{"message":"rate limit exceeded","type":"rate_limit_error","param":null,"code":"rate_limit_exceeded"}}`, w.Body.String())
}

func TestHandler_RouteBoundary(t *testing.T) {
	c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "canonical"}, Model: &fakeModel{}}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	_, err = New(nil)
	require.Error(t, err)
	for _, tc := range []struct {
		method, target, contentType string
		status                      int
	}{
		{"POST", "/v1/responses/x", "application/json", 404},
		{"POST", "/v1/responses?x=1", "application/json", 404},
		{"GET", "/v1/responses", "application/json", 405},
		{"POST", "/v1/responses", "text/plain", 415},
		{"POST", "/v1/responses", "application/json", 400},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", tc.contentType)
		h.ServeHTTP(w, req)
		assert.Equal(t, tc.status, w.Code, tc.method+" "+tc.target)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	}
}

func TestCategoryMapping(t *testing.T) {
	for _, tc := range []struct {
		category execution.Category
		status   int
		code     string
	}{
		{execution.CategoryInvalidRequest, 400, "invalid_request"},
		{execution.CategoryBYOKCredentials, 400, "invalid_request"},
		{execution.CategoryBYOKSelector, 400, "invalid_request"},
		{execution.CategoryGatewayControl, 400, "invalid_request"},
		{execution.CategoryPermission, 403, "permission_denied"},
		{execution.CategoryModelNotFound, 404, "model_not_found"},
		{execution.CategoryFailedDependency, 424, "failed_dependency"},
		{execution.CategoryCatalogConfiguration, 424, "failed_dependency"},
		{execution.CategoryRateLimit, 429, "rate_limit_exceeded"},
		{execution.CategoryCancellation, 499, "canceled"},
		{execution.CategoryUpstream, 502, "upstream_error"},
		{execution.CategoryProviderStream, 502, "upstream_error"},
		{execution.CategoryOverload, 503, "unavailable"},
		{execution.CategoryUnavailable, 503, "unavailable"},
		{execution.CategoryTimeout, 504, "timeout"},
		{execution.CategoryInternal, 500, "internal_error"},
		{0, 500, "internal_error"},
	} {
		status := categoryStatus(tc.category)
		assert.Equal(t, tc.status, status, "category %d", tc.category)
		assert.Equal(t, tc.code, errorFor(status).Code, "category %d", tc.category)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
