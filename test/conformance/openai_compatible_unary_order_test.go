package conformance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompatibleUnaryOrder_SimulatedStreaming(t *testing.T) {
	t.Parallel()
	for _, withTool := range []bool{false, true} {
		name := "reasoning and text"
		if withTool {
			name += " and tool call"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			message := `"content":"answer","reasoning_content":"thinking"`
			finishReason := "stop"
			wantContent := []aisdk.ContentPart{
				aisdk.ReasoningContent{Text: "thinking"},
				aisdk.TextContent{Text: "answer"},
			}
			wantMessage := provider.Message{
				Role: provider.RoleAssistant,
				Content: []provider.ContentPart{
					{Type: provider.ContentPartTypeReasoning, Text: "thinking"},
					{Type: provider.ContentPartTypeText, Text: "answer"},
				},
			}
			if withTool {
				message += `,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"}}]`
				finishReason = "tool_calls"
				wantContent = append(wantContent, aisdk.ToolCallContent{ToolCallID: "call-1", ToolName: "lookup", Input: json.RawMessage(`{}`)})
				wantMessage.Content = append(wantMessage.Content, provider.ContentPart{Type: provider.ContentPartTypeToolCall, ToolCallID: "call-1", ToolName: "lookup", Input: json.RawMessage(`{}`)})
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				assert.NotEqual(t, true, request["stream"], "the regression must exercise DoGenerate")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant",` + message + `},"finish_reason":"` + finishReason + `"}]}`))
			}))
			defer server.Close()

			toolSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{}}`))
			require.NoError(t, err)
			model := middleware.WrapLanguageModel(openaicompatible.New("test-model", openaicompatible.WithBaseURL(server.URL)), middleware.SimulateStreaming())
			result, err := aisdk.GenerateText(t.Context(), model,
				aisdk.WithModelMessages(provider.UserText("hi")),
				aisdk.WithTools(aisdk.ToolSet{"lookup": {InputSchema: toolSchema}}),
			)
			require.NoError(t, err)
			assert.Equal(t, wantContent, result.Content)
			require.Len(t, result.Steps, 1)
			assert.Equal(t, wantContent, result.Steps[0].Content)
			assert.Equal(t, []provider.Message{wantMessage}, result.Response.Messages)
			assert.Equal(t, []provider.Message{wantMessage}, result.ResponseMessages)
		})
	}
}
