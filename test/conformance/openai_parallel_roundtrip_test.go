package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/openai"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParallelToolCall_StreamTextRoundTrip(t *testing.T) {
	for _, failure := range []string{"none", "malformed before calls", "malformed after completion"} {
		t.Run(failure, func(t *testing.T) {
			const parallelInput = `{"tool_uses":[{"recipient_name":"functions.weather","parameters":{"location":"SF"}},{"recipient_name":"functions.cityAttractions","parameters":{"city":"Rome"}}]}`
			var requests atomic.Int32
			bodies := make(chan map[string]any, 2)
			arguments, err := json.Marshal(parallelInput)
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); !assert.NoError(t, err) {
					return
				}
				bodies <- body
				w.Header().Set("Content-Type", "text/event-stream")
				var events []string
				if requests.Add(1) == 1 {
					events = []string{
						`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_parallel","call_id":"call_parallel","name":"parallel","arguments":""}}`,
						fmt.Sprintf(`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_parallel","delta":%s}`, arguments),
						fmt.Sprintf(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_parallel","call_id":"call_parallel","name":"parallel","arguments":%s}}`, arguments),
						`{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
					}
				} else {
					events = []string{
						`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
						`{"type":"response.output_text.delta","output_index":0,"item_id":"msg_1","delta":"done"}`,
						`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"done","annotations":[]}]}}`,
						`{"type":"response.completed","response":{"id":"resp_2","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
					}
				}
				if requests.Load() == 1 {
					if failure == "malformed before calls" {
						events = append([]string{`not JSON`}, events...)
					}
					if failure == "malformed after completion" {
						events = append(events, `not JSON`)
					}
				}
				for _, event := range append(events, "[DONE]") {
					_, err := fmt.Fprintf(w, "data: %s\n\n", event)
					assert.NoError(t, err)
				}
			}))
			defer server.Close()
			var mu sync.Mutex
			executions := map[string]int{}
			tools := aisdk.ToolSet{}
			for _, name := range []string{"weather", "cityAttractions"} {
				tools[name] = aisdk.Tool{
					Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
						mu.Lock()
						executions[name]++
						mu.Unlock()
						return json.Marshal(name)
					},
					ToModelOutput: func(aisdk.ToolOutputContext) (*provider.ToolResultOutput, error) {
						return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: name}, nil
					},
				}
			}
			model := openai.NewResponses("test", "gpt-5.4", openai.WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
			result := aisdk.StreamText(t.Context(), model, aisdk.WithModelMessages(provider.UserText("weather")), aisdk.WithTools(tools), aisdk.WithStopWhen(aisdk.StepCountIs(2)), aisdk.WithProviderOptions(openai.OpenAIResponsesOptions{Conversation: "conv_1"}))
			var calls []string
			var finishReason provider.UnifiedFinishReason
			for part := range result.FullStream() {
				switch part := part.(type) {
				case aisdk.StreamToolCall:
					calls = append(calls, part.ToolCallID)
				case aisdk.StreamFinishStep:
					finishReason = part.FinishReason.Unified
				}
			}
			if failure != "none" {
				require.Error(t, result.Err())
				assert.Equal(t, []string{"call_parallel_0", "call_parallel_1"}, calls)
				assert.Equal(t, provider.FinishReasonError, finishReason)
				assert.Empty(t, result.Text())
				assert.Equal(t, int32(1), requests.Load())
				mu.Lock()
				assert.Empty(t, executions)
				mu.Unlock()
				require.Len(t, bodies, 1)
				return
			}
			require.NoError(t, result.Err())
			assert.Equal(t, []string{"call_parallel_0", "call_parallel_1"}, calls)
			assert.Equal(t, "done", result.Text())
			assert.Equal(t, int32(2), requests.Load())
			mu.Lock()
			assert.Equal(t, map[string]int{"weather": 1, "cityAttractions": 1}, executions)
			mu.Unlock()
			require.Len(t, bodies, 2)
			<-bodies
			second := <-bodies
			assert.Equal(t, "conv_1", second["conversation"])
			assert.Equal(t, []any{
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "weather"}}},
				map[string]any{"type": "function_call_output", "call_id": "call_parallel", "output": "weather\ncityAttractions"},
			}, second["input"])
		})
	}
}
