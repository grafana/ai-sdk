package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_MarchWebToolMarksImplicitCodeExecution(t *testing.T) {
	const codeExecution = "code_execution_20260120"
	const modelID = "claude-sonnet-4-6"
	for _, webType := range []string{"web_search_20260318", "web_fetch_20260318"} {
		for _, tc := range []struct {
			name        string
			fallback    bool
			explicit    bool
			choice      provider.ToolChoiceType
			wantDynamic bool
		}{
			{name: "ordinary", choice: provider.ToolChoiceAuto, wantDynamic: true},
			{name: "ordinary with explicit execution", explicit: true, choice: provider.ToolChoiceAuto},
			{name: "fallback excludes explicit execution", fallback: true, explicit: true, choice: provider.ToolChoiceTool, wantDynamic: true},
			{name: "fallback retains explicit execution", fallback: true, explicit: true, choice: provider.ToolChoiceAuto},
		} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", webType, tc.name, stream), func(t *testing.T) {
					var wire struct {
						Tools []struct {
							Type string `json:"type"`
							Name string `json:"name"`
						} `json:"tools"`
						Choice struct {
							Type string `json:"type"`
						} `json:"tool_choice"`
					}
					client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
						if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
							return nil, err
						}
						return dynamicWebToolResponse(r, stream), nil
					})}
					m := New("test-key", modelID, WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
					opts := provider.CallOptions{
						MaxOutputTokens: new(256),
						Prompt:          []provider.Message{provider.UserText("Find the answer.")},
						ToolChoice:      &provider.ToolChoice{Type: tc.choice, ToolName: "web"},
						Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic." + webType, Name: "web"}},
					}
					if tc.explicit {
						opts.Tools = append(opts.Tools, provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic." + codeExecution, Name: "code_execution"})
					}
					if tc.fallback {
						opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","additionalProperties":true}`)}
					}

					var wantDynamic *bool
					if tc.wantDynamic {
						wantDynamic = new(true)
					}
					if stream {
						res, err := m.DoStream(t.Context(), opts)
						require.NoError(t, err)
						var seen []provider.StreamPartType
						for part := range res.Stream {
							switch part.Type {
							case provider.PartToolInputStart, provider.PartToolCall:
								seen = append(seen, part.Type)
								assert.Equal(t, "code_execution", part.ToolName)
								assert.True(t, part.ProviderExecuted)
								assert.Equal(t, wantDynamic, part.Dynamic)
							case provider.PartError:
								assert.Fail(t, "unexpected provider stream error", "%+v", part)
							}
						}
						assert.Equal(t, []provider.StreamPartType{provider.PartToolInputStart, provider.PartToolCall}, seen)
					} else {
						res, err := m.DoGenerate(t.Context(), opts)
						require.NoError(t, err)
						require.NotEmpty(t, res.Content)
						call := res.Content[0]
						assert.Equal(t, provider.ContentToolCall, call.Type)
						assert.Equal(t, "code_execution", call.ToolName)
						assert.True(t, call.ProviderExecuted)
						assert.Equal(t, wantDynamic, call.Dynamic)
					}

					wantTools := []string{webType}
					if tc.explicit && tc.choice != provider.ToolChoiceTool {
						wantTools = append(wantTools, codeExecution)
					}
					wantChoice := "auto"
					if tc.fallback {
						wantTools = append(wantTools, "json")
						wantChoice = "any"
					}
					var gotTools []string
					for _, tool := range wire.Tools {
						if tool.Name == "json" {
							gotTools = append(gotTools, tool.Name)
						} else {
							gotTools = append(gotTools, tool.Type)
						}
					}
					assert.Equal(t, wantTools, gotTools)
					assert.Equal(t, wantChoice, wire.Choice.Type)
				})
			}
		}
	}
}

func dynamicWebToolResponse(r *http.Request, stream bool) *http.Response {
	const call = `{"type":"server_tool_use","id":"stu_1","name":"code_execution","input":{}}`
	const text = `{"type":"text","text":"{}"}`
	contentType := "application/json"
	payload := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[` + call + `,` + text + `],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	if stream {
		contentType = "text/event-stream"
		payload = transportSSE([]string{
			transportEvents[0],
			`{"type":"content_block_start","index":0,"content_block":` + call + `}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
			transportEvents[3],
			`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"{}"}}`,
			`{"type":"content_block_stop","index":1}`,
			transportEvents[4], transportEvents[5],
		})
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}
}
