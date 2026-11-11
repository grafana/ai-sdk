package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mcpApprovalRequestEvents = []string{
	`{"type":"response.created","response":{"id":"resp_approval","created_at":1700000000,"model":"gpt-4o","output":[]}}`,
	`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_approval_request","id":"item_1","approval_request_id":"appr_1","name":"do_thing","server_label":"srv","arguments":"{}"}}`,
	`{"type":"response.completed","response":{"id":"resp_approval","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
}

func TestMCPApprovalDenial_CoreContinuation(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			var bodies []map[string]any
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, json.Unmarshal(raw, &body))
				bodies = append(bodies, body)
				events := transportEvents
				if len(bodies) == 1 {
					events = mcpApprovalRequestEvents
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(transportSSE(events))), Request: r}, nil
			})}
			model := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			run := func(messages []provider.Message) []provider.Message {
				t.Helper()
				if mode == "generate" {
					result, err := aisdk.GenerateText(t.Context(), model, aisdk.WithModelMessages(messages...))
					require.NoError(t, err)
					return result.ResponseMessages
				}
				result := aisdk.StreamText(t.Context(), model, aisdk.WithModelMessages(messages...))
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				return result.ResponseMessages()
			}

			history := []provider.Message{provider.UserText("do the thing")}
			history = append(history, run(history)...)
			var toolCallID string
			for _, part := range history[1].Content {
				if part.Type == provider.ContentPartTypeToolApprovalRequest {
					toolCallID = part.ToolCallID
				}
			}
			require.NotEmpty(t, toolCallID)
			history = append(history, provider.NewToolMessage(provider.ProviderExecutedToolApprovalResponsePart("appr_1", false, "not allowed")))

			resumed := run(history)

			var denials []provider.ContentPart
			for _, msg := range resumed {
				for _, part := range msg.Content {
					if part.Type == provider.ContentPartTypeToolResult && part.Output != nil && part.Output.Type == provider.ToolOutputExecutionDenied {
						denials = append(denials, part)
					}
				}
			}
			if mode == "generate" {
				require.Len(t, denials, 1)
				assert.Equal(t, toolCallID, denials[0].ToolCallID)
				assert.Equal(t, "mcp.do_thing", denials[0].ToolName)
				assert.Equal(t, "appr_1", openAIPartOptionsFor(denials[0].Output.ProviderOptions, "openai").ApprovalID)
			} else {
				assert.Empty(t, denials)
			}

			history = append(history, resumed...)
			history = append(history, provider.UserText("next turn"))
			run(history)

			require.Len(t, bodies, 3)
			for _, body := range bodies[1:] {
				var approvalResponses []any
				for _, item := range body["input"].([]any) {
					entry := item.(map[string]any)
					assert.NotEqual(t, "function_call_output", entry["type"])
					if entry["type"] == "mcp_approval_response" {
						approvalResponses = append(approvalResponses, entry)
					}
				}
				assert.Equal(t, []any{map[string]any{"type": "mcp_approval_response", "approval_request_id": "appr_1", "approve": false}}, approvalResponses)
			}
		})
	}
}
