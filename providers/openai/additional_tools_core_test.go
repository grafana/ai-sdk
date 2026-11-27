package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdditionalTools_CoreContinuation(t *testing.T) {
	for _, generate := range []bool{false, true} {
		for _, store := range []bool{false, true} {
			t.Run(fmt.Sprintf("generate=%t/store=%t", generate, store), func(t *testing.T) {
				const definition = `[{"type":"function","name":"lookup","parameters":{}}]`
				history := []provider.Message{
					provider.UserText("Continue a previous search"),
					provider.NewAssistantMessage(provider.ToolCallPart("lookup_1", "search", json.RawMessage(`{}`))),
					additionalResultPrompt(additionalOptions(t, `{"openai":{"additionalTools":`+definition+`}}`))[0],
				}
				var bodies []additionalToolsRequest
				m := additionalToolsModel(func(r *http.Request) (*http.Response, error) {
					var body additionalToolsRequest
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						return nil, err
					}
					bodies = append(bodies, body)
					callID, toolName := "call_continue", "continue_work"
					if len(bodies) == 2 {
						callID, toolName = "call_lookup", "lookup"
					}
					responseID := fmt.Sprintf("resp_%d", len(bodies))
					call := fmt.Sprintf(`{"type":"function_call","id":"fc_%s","call_id":%q,"name":%q,"arguments":"{}","status":"completed"}`, callID, callID, toolName)
					envelope := fmt.Sprintf(`{"id":%q,"object":"response","created_at":1700000000,"model":"gpt-5.4","status":"completed","output":[%s],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, responseID, call)
					payload := transportSSE([]string{
						fmt.Sprintf(`{"type":"response.created","response":{"id":%q,"created_at":1700000000,"model":"gpt-5.4","output":[]}}`, responseID),
						`{"type":"response.output_item.added","output_index":0,"item":` + call + `}`,
						`{"type":"response.output_item.done","output_index":0,"item":` + call + `}`,
						`{"type":"response.completed","response":` + envelope + `}`,
					})
					return &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(payload)), Request: r,
					}, nil
				})
				var searchExecutions, continueExecutions atomic.Int32
				inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object"}`))
				require.NoError(t, err)
				tools := aisdk.ToolSet{
					"search": {InputSchema: inputSchema, Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
						searchExecutions.Add(1)
						return nil, fmt.Errorf("unexpected execution of a historical tool call")
					}},
					"continue_work": {InputSchema: inputSchema, Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
						continueExecutions.Add(1)
						return json.RawMessage(`{"continued":true}`), nil
					}},
				}
				historyOpt, toolOpt := aisdk.WithModelMessages(history...), aisdk.WithTools(tools)
				stopOpt, retryOpt := aisdk.WithStopWhen(aisdk.StepCountIs(2)), aisdk.WithMaxRetries(0)
				providerOpt := aisdk.WithProviderOptions(OpenAIResponsesOptions{Store: &store})
				var steps []aisdk.StepResult
				if generate {
					result, err := aisdk.GenerateText(t.Context(), m, historyOpt, toolOpt, stopOpt, retryOpt, providerOpt)
					require.NoError(t, err)
					steps = result.Steps
				} else {
					result := aisdk.StreamText(t.Context(), m, historyOpt, toolOpt, stopOpt, retryOpt, providerOpt)
					for range result.FullStream() {
					}
					require.NoError(t, result.Err())
					steps = result.Steps()
				}
				require.Len(t, steps, 2)
				require.Len(t, steps[1].ToolCalls, 1)
				call := steps[1].ToolCalls[0]
				assert.Equal(t, "lookup", call.ToolName)
				assert.True(t, call.Invalid)
				assert.ErrorContains(t, call.Error, "AI_NoSuchToolError")
				require.Len(t, bodies, 2)
				assert.Zero(t, searchExecutions.Load())
				assert.EqualValues(t, 1, continueExecutions.Load())
				const prefix = `{"role":"user","content":[{"type":"input_text","text":"Continue a previous search"}]},{"type":"function_call","call_id":"lookup_1","name":"search","arguments":"{}"},` + additionalResult
				firstInput := prefix + "," + additionalToolItem(definition)
				assert.JSONEq(t, "["+firstInput+"]", string(bodies[0].Input))
				assert.JSONEq(t, "["+firstInput+`,{"type":"function_call","call_id":"call_continue","name":"continue_work","arguments":"{}"},{"type":"function_call_output","call_id":"call_continue","output":"{\"continued\":true}"}]`, string(bodies[1].Input))
				for _, body := range bodies {
					assert.True(t, body.Stream)
					var declarations []struct{ Name string }
					require.NoError(t, json.Unmarshal(body.Tools, &declarations))
					require.Len(t, declarations, 2)
					assert.ElementsMatch(t, []string{"search", "continue_work"}, []string{declarations[0].Name, declarations[1].Name})
				}
			})
		}
	}
}
