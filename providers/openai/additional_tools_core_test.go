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
					response := transportResponse(r, true)
					if len(bodies) == 1 {
						const call = `{"type":"function_call","id":"fc_continue","call_id":"call_continue","name":"continue_work","arguments":"{}","status":"completed"}`
						const envelope = `{"id":"resp_first","object":"response","created_at":1700000000,"model":"gpt-5.4","status":"completed","output":[` + call + `],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
						payload := transportSSE([]string{
							`{"type":"response.created","response":{"id":"resp_first","created_at":1700000000,"model":"gpt-5.4","output":[]}}`,
							`{"type":"response.output_item.added","output_index":0,"item":` + call + `}`,
							`{"type":"response.output_item.done","output_index":0,"item":` + call + `}`,
							`{"type":"response.completed","response":` + envelope + `}`,
						})
						_ = response.Body.Close()
						response.Body = io.NopCloser(strings.NewReader(payload))
					}
					return response, nil
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
				if generate {
					result, err := aisdk.GenerateText(t.Context(), m, historyOpt, toolOpt, stopOpt, retryOpt, providerOpt)
					require.NoError(t, err)
					require.Len(t, result.Steps, 2)
				} else {
					result := aisdk.StreamText(t.Context(), m, historyOpt, toolOpt, stopOpt, retryOpt, providerOpt)
					for range result.FullStream() {
					}
					require.NoError(t, result.Err())
					require.Len(t, result.Steps(), 2)
				}
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
