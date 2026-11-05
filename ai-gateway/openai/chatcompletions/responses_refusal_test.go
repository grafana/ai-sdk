package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	openai "github.com/grafana/ai-sdk/providers/openai"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const nativeRefusalResponse = `{"id":"resp_refusal","created_at":1,"model":"gpt-4o","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"Cannot help."}]}],"usage":{"input_tokens":1,"output_tokens":3,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`

func TestResponsesRefusal_OfficialClientStructuredOutput(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, finalOnly := range []bool{false, true} {
			for _, requiredTool := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%v/finalOnly=%v/requiredTool=%v", streaming, finalOnly, requiredTool), func(t *testing.T) {
					calls := 0
					nativeClient := &http.Client{Transport: mediaTransport(func(req *http.Request) (*http.Response, error) {
						calls++
						body, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						assert.NotContains(t, string(body), "preserveRefusal")
						assert.Contains(t, string(body), `"json_schema"`)
						payload, contentType := nativeRefusalResponse, "application/json"
						if streaming {
							events := []string{`{"type":"response.created","sequence_number":0,"response":{"id":"resp_refusal","created_at":1,"model":"gpt-4o","object":"response","status":"in_progress","output":[]}}`}
							if !finalOnly {
								events = append(events,
									`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
									`{"type":"response.refusal.delta","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Cannot "}`,
									`{"type":"response.refusal.delta","sequence_number":3,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"help."}`,
									`{"type":"response.refusal.done","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"refusal":"Cannot help."}`,
									`{"type":"response.output_item.done","sequence_number":5,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"Cannot help."}]}}`)
							}
							events = append(events, `{"type":"response.completed","sequence_number":6,"response":`+nativeRefusalResponse+`}`)
							payload = ""
							for _, event := range events {
								payload += "data: " + event + "\n\n"
							}
							contentType = "text/event-stream"
						}
						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
					})}
					model := openai.NewResponses("synthetic", "gpt-4o", openai.WithRequestOptions(option.WithHTTPClient(nativeClient), option.WithMaxRetries(0)))
					c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "alias"}, Model: execution.Translate(model, execution.Target{Type: execution.TargetOpenAI})}})
					require.NoError(t, err)
					runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
					require.NoError(t, err)
					h, err := New(runtime)
					require.NoError(t, err)
					client := sdk.NewClient(option.WithBaseURL("https://gateway.test/v1/"), option.WithAPIKey("synthetic"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: mediaTransport(func(req *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						h.ServeHTTP(w, req)
						return w.Result(), nil
					})}))
					raw := `{"model":"alias","messages":[{"role":"user","content":"test"}],"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`
					if requiredTool {
						raw += `,"tools":[{"type":"function","function":{"name":"f","parameters":{"type":"object"}}}],"tool_choice":"required"`
					}
					raw += "}"
					var params sdk.ChatCompletionNewParams
					require.NoError(t, json.Unmarshal([]byte(raw), &params))
					if streaming {
						stream := client.Chat.Completions.NewStreaming(context.Background(), params)
						var refusal strings.Builder
						finishes := 0
						for stream.Next() {
							chunk := stream.Current()
							for _, choice := range chunk.Choices {
								refusal.WriteString(choice.Delta.Refusal)
								assert.Empty(t, choice.Delta.Content)
								if choice.FinishReason != "" {
									finishes++
									assert.Equal(t, "stop", choice.FinishReason)
								}
							}
						}
						require.NoError(t, stream.Err())
						require.NoError(t, stream.Close())
						assert.Equal(t, "Cannot help.", refusal.String())
						assert.Equal(t, 1, finishes)
					} else {
						result, err := client.Chat.Completions.New(context.Background(), params)
						require.NoError(t, err)
						require.Len(t, result.Choices, 1)
						assert.Equal(t, "Cannot help.", result.Choices[0].Message.Refusal)
						assert.Contains(t, result.RawJSON(), `"content":null`)
					}
					assert.Equal(t, 1, calls)
				})
			}
		}
	}
}
