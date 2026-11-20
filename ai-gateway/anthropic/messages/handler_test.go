package messages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
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

func testHandler(t *testing.T, model *fakeModel) http.Handler {
	t.Helper()
	entries, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "canonical", Aliases: []string{"alias"}}, Model: execution.Translate(model, execution.Target{Type: execution.TargetAnthropic})}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(entries), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	return h
}

func ptr[T any](v T) *T { return &v }

func textResult() *provider.GenerateResult {
	return &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:        provider.Usage{InputTokens: provider.InputTokenUsage{Total: ptr(3), CacheRead: ptr(1)}, OutputTokens: provider.OutputTokenUsage{Total: ptr(2)}},
	}
}

func streamOf(parts ...provider.StreamPart) *provider.StreamResult {
	ch := make(chan provider.StreamPart, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	return &provider.StreamResult{Stream: ch}
}

func finish(reason provider.UnifiedFinishReason) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: reason}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: ptr(4)}, OutputTokens: provider.OutputTokenUsage{Total: ptr(6)}}}
}

func TestHandler_OfficialSDK(t *testing.T) {
	t.Run("unary text", func(t *testing.T) {
		server := httptest.NewServer(testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return textResult(), nil
		}}))
		defer server.Close()
		client := anthropicsdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		msg, err := client.Messages.New(context.Background(), anthropicsdk.MessageNewParams{
			Model: "alias", MaxTokens: 16,
			Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("hi"))},
		})
		require.NoError(t, err)
		assert.Equal(t, "canonical", string(msg.Model))
		assert.Equal(t, anthropicsdk.StopReasonEndTurn, msg.StopReason)
		require.Len(t, msg.Content, 1)
		assert.Equal(t, "hello", msg.Content[0].Text)
		assert.Equal(t, int64(2), msg.Usage.InputTokens)
		assert.Equal(t, int64(1), msg.Usage.CacheReadInputTokens)
		assert.Equal(t, int64(2), msg.Usage.OutputTokens)
	})

	t.Run("streaming text then tool use", func(t *testing.T) {
		server := httptest.NewServer(testHandler(t, &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return streamOf(
				provider.StreamPart{Type: provider.PartStreamStart},
				provider.StreamPart{Type: provider.PartTextStart, ID: "t1"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "Let me "},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "check."},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"},
				provider.StreamPart{Type: provider.PartToolInputStart, ID: "call_1", ToolName: "weather"},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call_1", Delta: `{"city":`},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call_1", Delta: `"Oslo"}`},
				provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call_1"},
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "weather", Input: `{"city":"Oslo"}`},
				finish(provider.FinishReasonToolCalls),
			), nil
		}}))
		defer server.Close()
		client := anthropicsdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		stream := client.Messages.NewStreaming(context.Background(), anthropicsdk.MessageNewParams{
			Model: "alias", MaxTokens: 16,
			Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("hi"))},
		})
		var acc anthropicsdk.Message
		for stream.Next() {
			require.NoError(t, acc.Accumulate(stream.Current()))
		}
		require.NoError(t, stream.Err())
		assert.Equal(t, anthropicsdk.StopReasonToolUse, acc.StopReason)
		require.Len(t, acc.Content, 2)
		assert.Equal(t, "Let me check.", acc.Content[0].Text)
		assert.Equal(t, "weather", acc.Content[1].Name)
		assert.JSONEq(t, `{"city":"Oslo"}`, string(acc.Content[1].Input))
		assert.Equal(t, int64(6), acc.Usage.OutputTokens)
	})

	t.Run("tool result continuation", func(t *testing.T) {
		var seen provider.CallOptions
		server := httptest.NewServer(testHandler(t, &fakeModel{generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
			seen = o
			return textResult(), nil
		}}))
		defer server.Close()
		client := anthropicsdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		_, err := client.Messages.New(context.Background(), anthropicsdk.MessageNewParams{
			Model: "alias", MaxTokens: 16,
			Tools: []anthropicsdk.ToolUnionParam{{OfTool: &anthropicsdk.ToolParam{Name: "weather", InputSchema: anthropicsdk.ToolInputSchemaParam{}}}},
			Messages: []anthropicsdk.MessageParam{
				anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("weather?")),
				anthropicsdk.NewAssistantMessage(anthropicsdk.NewToolUseBlock("call_1", map[string]any{"city": "Oslo"}, "weather")),
				anthropicsdk.NewUserMessage(anthropicsdk.NewToolResultBlock("call_1", "rainy", false)),
			},
		})
		require.NoError(t, err)
		require.Len(t, seen.Tools, 1)
		require.Len(t, seen.Prompt, 3)
		assert.Equal(t, provider.RoleTool, seen.Prompt[2].Role)
	})

	t.Run("unknown model", func(t *testing.T) {
		server := httptest.NewServer(testHandler(t, &fakeModel{}))
		defer server.Close()
		client := anthropicsdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		_, err := client.Messages.New(context.Background(), anthropicsdk.MessageNewParams{
			Model: "missing", MaxTokens: 16,
			Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("hi"))},
		})
		var api *anthropicsdk.Error
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 404, api.StatusCode)
		assert.NotContains(t, err.Error(), "private-provider")
	})
}

func TestHandler_RequestBoundary(t *testing.T) {
	const basic = `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`
	for _, tc := range []struct {
		name, method, path, media, body string
		status                          int
	}{
		{"wrong method", "GET", Path, "application/json", basic, 405},
		{"wrong path", "POST", "/v1/other", "application/json", basic, 404},
		{"media", "POST", Path, "text/plain", basic, 415},
		{"invalid utf8", "POST", Path, "application/json", string([]byte{255}), 400},
		{"no max_tokens", "POST", Path, "application/json", `{"model":"alias","messages":[{"role":"user","content":"hi"}]}`, 400},
		{"image block", "POST", Path, "application/json", `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":[{"type":"image"}]}]}`, 400},
		{"server tool", "POST", Path, "application/json", `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search_20250305","name":"web_search"}]}`, 400},
		{"tool choice without name", "POST", Path, "application/json", `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"tool"}}`, 400},
		{"unknown tool choice", "POST", Path, "application/json", `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"unknown"}}`, 400},
		{"case-sensitive block type", "POST", Path, "application/json", `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":[{"type":"Text","text":"hi"}]}]}`, 400},
		{"orphan tool result", "POST", Path, "application/json", `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"r"}]}]}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				t.Error("unexpected generation")
				return nil, nil
			}})
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			assert.Equal(t, tc.status, w.Code)
			var envelope errorEnvelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
			assert.Equal(t, envelopeError, envelope.Type)
		})
	}
}

func TestHandler_ProviderFailureIsFixedError(t *testing.T) {
	const secret = "sentinel-provider-secret"
	echo := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 500, Message: secret, ResponseBody: secret})
	h := testHandler(t, &fakeModel{
		generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, echo },
		stream:   func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, echo },
	})
	for _, extra := range []string{``, `,"stream":true`} {
		req := httptest.NewRequest("POST", Path, strings.NewReader(`{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}]`+extra+`}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.GreaterOrEqual(t, w.Code, 500)
		assert.NotContains(t, w.Body.String(), secret)
	}
}
