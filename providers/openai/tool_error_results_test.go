package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertToolResultRequest(t *testing.T, namespace string, streaming bool, opts provider.CallOptions, expected []any) {
	t.Helper()
	before, err := json.Marshal(opts)
	require.NoError(t, err)
	var captured []byte
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var err error
		captured, err = io.ReadAll(r.Body)
		assert.NoError(t, err)
		return transportResponse(r, streaming), nil
	})}
	m := NewResponses("test-key", "gpt-4o", WithProviderName(namespace), WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
	if streaming {
		result, err := m.DoStream(t.Context(), opts)
		require.NoError(t, err)
		for part := range result.Stream {
			assert.NotEqual(t, provider.PartError, part.Type)
		}
	} else {
		_, err := m.DoGenerate(t.Context(), opts)
		require.NoError(t, err)
	}
	var body map[string]any
	require.NoError(t, json.Unmarshal(captured, &body))
	assert.Equal(t, expected, body["input"])
	if streaming {
		assert.Equal(t, true, body["stream"])
	} else {
		assert.NotEqual(t, true, body["stream"])
	}
	after, err := json.Marshal(opts)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "caller options must remain unchanged")
}

func TestModel_ToolErrorResults(t *testing.T) {
	cases := []struct {
		name   string
		output provider.ToolResultOutput
		want   string
		quote  bool
	}{
		{"text", provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}, "ok", true},
		{"empty text", provider.ToolResultOutput{Type: provider.ToolOutputText}, "", true},
		{"json object", provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"code":"E42"}`)}, `{"code":"E42"}`, false},
		{"json null", provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`)}, `null`, false},
		{"json empty string", provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`""`)}, `""`, false},
		{"error text", provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "E42\n\"failed\""}, `{"error":"E42\n\"failed\""}`, false},
		{"error text HTML characters", provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "got <nil> & x>y"}, `{"error":"got <nil> & x>y"}`, false},
		{"error JSON HTML characters", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"message":"got <nil> & x>y"}`)}, `{"error":{"message":"got <nil> & x>y"}}`, false},
		{"empty error text", provider.ToolResultOutput{Type: provider.ToolOutputErrorText}, `{"error":""}`, false},
		{"error object", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"code":"E42","details":[1,false,null]}`)}, `{"error":{"code":"E42","details":[1,false,null]}}`, false},
		{"nested error", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":"E42"}`)}, `{"error":{"error":"E42"}}`, false},
		{"error null", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`null`)}, `{"error":null}`, false},
		{"error nil", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON}, `{"error":null}`, false},
		{"error empty bytes", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage{}}, `{"error":null}`, false},
		{"error array", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`["E42",0,false,null]`)}, `{"error":["E42",0,false,null]}`, false},
		{"error empty JSON string", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`""`)}, `{"error":""}`, false},
		{"error boolean", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`false`)}, `{"error":false}`, false},
		{"error number", provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`9007199254740993`)}, `{"error":9007199254740993}`, false},
		{"denial default", provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied}, "Tool call execution denied.", true},
		{"denial empty", provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: new("")}, "", true},
	}
	for _, namespace := range []string{"openai", "azure"} {
		for _, custom := range []bool{false, true} {
			for _, schema := range []bool{false, true} {
				if custom && schema {
					continue
				}
				for _, location := range []string{"none", "part", "output", "both", "inactive"} {
					for _, streaming := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/custom=%t/schema=%t/cache=%s/stream=%t", namespace, custom, schema, location, streaming), func(t *testing.T) {
							tool := provider.Tool{Type: provider.ToolTypeFunction, Name: "deploy", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}
							itemType := "function_call_output"
							if custom {
								tool.Type, tool.ID, itemType = provider.ToolTypeProvider, toolIDCustom, "custom_tool_call_output"
								tool.InputSchema = nil
							}
							if schema {
								tool.ProviderOptions = provider.BuildProviderOptions(OpenAIToolOptions{OutputSchema: json.RawMessage(`{"type":"string"}`)})
							}
							var parts []provider.ContentPart
							var expected []any
							for i, tc := range cases {
								output := tc.output
								part := provider.ToolResultPart(fmt.Sprintf("call_%d", i), "deploy", &output)
								if location == "part" || location == "both" {
									part.ProviderOptions = resultOptions(t, namespace, true)
								}
								if location == "output" || location == "both" {
									output.ProviderOptions = resultOptions(t, namespace, true)
								}
								if location == "inactive" {
									other := "azure"
									if namespace == "azure" {
										other = "openai"
									}
									part.ProviderOptions, output.ProviderOptions = resultOptions(t, other, true), resultOptions(t, other, true)
								}
								want := tc.want
								if schema && !custom && tc.quote {
									encoded, err := json.Marshal(want)
									require.NoError(t, err)
									want = string(encoded)
								}
								var wire any = want
								if location != "none" && location != "inactive" {
									wire = []any{map[string]any{"type": "input_text", "text": want, "prompt_cache_breakpoint": map[string]any{"mode": "explicit"}}}
								}
								expected = append(expected, map[string]any{"type": itemType, "call_id": part.ToolCallID, "output": wire})
								parts = append(parts, part)
							}
							assertToolResultRequest(t, namespace, streaming, provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(parts...)}, Tools: []provider.Tool{tool}}, expected)
						})
					}
				}
			}
		}
	}
}

func TestModel_ParallelToolErrorResults(t *testing.T) {
	for _, namespace := range []string{"openai", "azure"} {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name       string
				breakpoint bool
				schema     bool
				options    string
			}{
				{name: "conversation", options: `{"conversation":"conv_1"}`},
				{name: "conversation cached", breakpoint: true, options: `{"conversation":"conv_1"}`},
				{name: "previous response schema", schema: true, options: `{"previousResponseId":"resp_1"}`},
				{name: "previous response schema cached", schema: true, breakpoint: true, options: `{"previousResponseId":"resp_1"}`},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", namespace, streaming, tc.name), func(t *testing.T) {
					br := buildResult{functionTools: map[string]struct{}{"weather": {}, "cityAttractions": {}}}
					calls := br.expandParallelToolCall(responses.ResponseFunctionToolCall{ID: "fc_parallel", CallID: "call_parallel", Name: parallelToolName, Arguments: parallelInput}, namespace)
					require.Len(t, calls, 2)
					var parts []provider.ContentPart
					for i, call := range calls {
						output := &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "failed"}
						if i == 1 {
							output = &provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"code":"E42"}`)}
						}
						if tc.breakpoint && i == 0 {
							output.ProviderOptions = resultOptions(t, namespace, true)
						}
						part := provider.ToolResultPart(call.ToolCallID, call.ToolName, output)
						part.ProviderOptions = provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: call.ProviderMetadata[namespace]}}
						parts = append(parts, part)
					}
					options := provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(tc.options)}}
					var want any = "{\"error\":\"failed\"}\n{\"error\":{\"code\":\"E42\"}}"
					if tc.breakpoint {
						want = []any{map[string]any{"type": "input_text", "text": `{"error":"failed"}`, "prompt_cache_breakpoint": map[string]any{"mode": "explicit"}}, map[string]any{"type": "input_text", "text": "\n{\"error\":{\"code\":\"E42\"}}"}}
					}
					var tools []provider.Tool
					if tc.schema {
						for _, name := range []string{"weather", "cityAttractions"} {
							tools = append(tools, provider.Tool{Type: provider.ToolTypeFunction, Name: name, InputSchema: json.RawMessage(`{"type":"object","properties":{}}`), ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{OutputSchema: json.RawMessage(`{"type":"string"}`)})})
						}
					}
					assertToolResultRequest(t, namespace, streaming, provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(parts[1], parts[0])}, ProviderOptions: options, Tools: tools}, []any{map[string]any{"type": "function_call_output", "call_id": "call_parallel", "output": want}})
				})
			}
		}
	}
}

func TestToolResultOutputString_InvalidErrorJSON(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"invalid token", "not json", `{"error":"not json"}`},
		{"truncated object", `{"message":"<nil> & x>y"`, `{"error":"{\"message\":\"<nil> & x>y\""}`},
		{"whitespace", " \n", `{"error":" \n"}`},
	} {
		for _, schema := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/schema=%t", tc.name, schema), func(t *testing.T) {
				out := provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(tc.input)}
				assert.Equal(t, tc.want, toolResultOutputString(&out, schema))
				assert.Equal(t, tc.input, string(out.JSON))
			})
		}
	}
}
