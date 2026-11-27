package openaicompatible

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mixedArrayContent = `[{"type":"text","text":"First."},` +
	`{"type":"thinking","thinking":[null,4,[],{},{"type":"text","text":"Check "},{"type":"text","text":12},{"type":"image","text":"skip"},{"type":"text","text":"the result."}]},` +
	`{"type":"future-part","text":{"nested":true},"thinking":{"nested":true}},{"type":"text","text":[]},{"type":"text","text":""},` +
	`{"type":"thinking","thinking":"not an array"},{"type":"thinking","thinking":[{"type":"text","text":""}]},{"type":"Text","text":"skip"},` +
	`{"type":"text","text":"Done."}]`

func TestDoGenerate_ArrayContent(t *testing.T) {
	t.Parallel()

	text := func(value string) provider.GenerateContentPart {
		return provider.GenerateContentPart{Type: provider.ContentText, Text: value}
	}
	reasoning := func(value string) provider.GenerateContentPart {
		return provider.GenerateContentPart{Type: provider.ContentReasoning, Text: value}
	}
	for _, tc := range []struct {
		name, fields string
		want         []provider.GenerateContentPart
		finish       provider.UnifiedFinishReason
	}{
		{
			name:   "ordered blocks, scalar reasoning, refusal and tool",
			fields: `"content":` + mixedArrayContent + `,"reasoning_content":"Primary.","reasoning":"Ignore.","refusal":"Cannot continue.","tool_calls":[{"id":"call-1","function":{"name":"lookup","arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig"}}}]`,
			want: []provider.GenerateContentPart{
				reasoning("Primary."), text("First."), reasoning("Check the result."), text("Done."), text("Cannot continue."),
				{Type: provider.ContentToolCall, ToolCallID: "call-1", ToolName: "lookup", Input: json.RawMessage(`{}`), ProviderMetadata: provider.ProviderMetadata{
					"openai-compatible": json.RawMessage(`{"thoughtSignature":"sig"}`),
				}},
			},
			finish: provider.FinishReasonContentFilter,
		},
		{
			name:   "empty array selects empty primary over fallback",
			fields: `"content":[],"reasoning_content":"","reasoning":"Ignore."`,
			want:   []provider.GenerateContentPart{},
			finish: provider.FinishReasonStop,
		},
		{
			name:   "null primary selects fallback",
			fields: `"content":[{"type":"text","text":"Done."}],"reasoning_content":null,"reasoning":"Fallback."`,
			want:   []provider.GenerateContentPart{reasoning("Fallback."), text("Done.")},
			finish: provider.FinishReasonStop,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := `{"id":"chat-1","model":"synthetic","created":1,"choices":[{"message":{` + tc.fields + `},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":3}}`
			model := newArrayContentTestModel(t, "application/json", body)
			result, err := model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.Content)
			assert.Equal(t, provider.FinishReason{Unified: tc.finish, Raw: "stop"}, result.FinishReason)
			require.NotNil(t, result.Response)
			assert.Equal(t, "chat-1", result.Response.ID)
			assert.JSONEq(t, body, string(result.Response.Body))
			assert.JSONEq(t, `{"prompt_tokens":4,"completion_tokens":3}`, string(result.Usage.Raw))
		})
	}
}

func TestDoStream_ArrayContent(t *testing.T) {
	t.Parallel()

	want := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "txt-0"},
		{Type: provider.PartTextDelta, ID: "txt-0", Delta: "First."},
		{Type: provider.PartTextEnd, ID: "txt-0"},
		{Type: provider.PartReasoningStart, ID: "reasoning-0"},
		{Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: "Check the result."},
		{Type: provider.PartReasoningEnd, ID: "reasoning-0"},
		{Type: provider.PartTextStart, ID: "txt-0"},
		{Type: provider.PartTextDelta, ID: "txt-0", Delta: "Done."},
		{Type: provider.PartTextDelta, ID: "txt-0", Delta: " Cannot continue."},
		{Type: provider.PartTextEnd, ID: "txt-0"},
	}
	for _, tc := range []struct{ name, stream string }{
		{
			name:   "alternating array blocks",
			stream: arrayContentSSE(`{"content":`+mixedArrayContent+`,"reasoning_content":"","reasoning":"Ignore.","refusal":" Cannot continue."}`, "stop"),
		},
		{
			name: "array text followed by scalar reasoning",
			stream: arrayContentSSE(`{"content":[{"type":"text","text":"First."}]}`, "") +
				arrayContentSSE(`{"reasoning_content":"Check the result."}`, "") +
				arrayContentSSE(`{"content":[{"type":"text","text":"Done."}],"refusal":" Cannot continue."}`, "stop"),
		},
		{
			name: "scalar-only text followed by reasoning",
			stream: arrayContentSSE(`{"content":"First."}`, "") +
				arrayContentSSE(`{"reasoning_content":"","reasoning":"Check the result."}`, "") +
				arrayContentSSE(`{"content":"Done.","refusal":" Cannot continue."}`, "stop"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			model := newArrayContentTestModel(t, "text/event-stream", tc.stream+"data: [DONE]\n\n")
			result, err := model.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			require.NoError(t, err)
			parts := collectStreamParts(result)
			require.Len(t, parts, len(want)+2)
			assert.Equal(t, provider.PartStreamStart, parts[0].Type)
			assert.Equal(t, want, parts[1:len(parts)-1])
			assert.Equal(t, provider.PartFinish, parts[len(parts)-1].Type)
			assert.Equal(t, &provider.FinishReason{Unified: provider.FinishReasonContentFilter, Raw: "stop"}, parts[len(parts)-1].FinishReason)
		})
	}
}

func TestContentArray_InvalidResponse(t *testing.T) {
	t.Parallel()

	for _, tc := range []string{`{}`, `false`, `[null]`, `[3]`, `[[]]`, `[{}]`, `[{"type":null}]`, `[{"type":0}]`} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}
			fields := `{"content":` + tc + `}`
			unary := newArrayContentTestModel(t, "application/json", `{"choices":[{"message":`+fields+`,"finish_reason":"stop"}]}`)
			_, err := unary.DoGenerate(t.Context(), opts)
			require.ErrorContains(t, err, "decoding response")

			body := arrayContentSSE(fields, "") + arrayContentSSE(`{"content":[{"type":"text","text":"Recovered."}]}`, "stop") + "data: [DONE]\n\n"
			stream := newArrayContentTestModel(t, "text/event-stream", body)
			result, err := stream.DoStream(t.Context(), opts)
			require.NoError(t, err)
			parts := collectStreamParts(result)
			require.Len(t, parts, 6)
			assert.Equal(t, provider.PartError, parts[1].Type)
			require.NotNil(t, parts[1].APICallError)
			assert.Contains(t, parts[1].APICallError.Message, "stream decode failure")
			assert.Equal(t, provider.StreamPart{Type: provider.PartTextDelta, ID: "txt-0", Delta: "Recovered."}, parts[3])
			assert.Equal(t, provider.PartFinish, parts[5].Type)
		})
	}
}

func newArrayContentTestModel(t *testing.T, contentType, body string) provider.LanguageModel {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return New("test", WithBaseURL(server.URL))
}

func arrayContentSSE(delta, finish string) string {
	reason := "null"
	if finish != "" {
		reason = fmt.Sprintf("%q", finish)
	}
	return fmt.Sprintf("data: {\"choices\":[{\"delta\":%s,\"finish_reason\":%s}]}\n\n", delta, reason)
}
