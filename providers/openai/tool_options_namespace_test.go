package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponses_FunctionToolOptionsNamespace(t *testing.T) {
	allOptions := OpenAIToolOptions{
		Async: boolPointer(true), DeferLoading: boolPointer(true),
		AllowedCallers: []OpenAIAllowedCaller{OpenAIAllowedCallerProgrammatic},
		OutputSchema:   json.RawMessage(`{"type":"string"}`),
		Namespace:      &OpenAIToolNamespaceOptions{Name: "utilities", Description: "Helpers"},
	}
	for _, tc := range []struct {
		name, identity string
		options        provider.ProviderOptions
		want           OpenAIToolOptions
		azureCall      bool
	}{
		{name: "azure all fields", identity: "azure.responses", options: provider.ProviderOptions{"azure": allOptions}, want: allOptions},
		{name: "azure ignores invalid fallback", identity: "azure.responses", options: provider.ProviderOptions{"azure": allOptions, "openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"async":"invalid"}`)}}, want: allOptions},
		{name: "default ignores invalid azure", options: provider.ProviderOptions{"azure": provider.RawProviderOption{Key: "azure", Raw: json.RawMessage(`{"async":"invalid"}`)}, "openai": allOptions}, want: allOptions},
		{name: "azure whole object wins", identity: "azure.responses", options: provider.ProviderOptions{"azure": OpenAIToolOptions{DeferLoading: boolPointer(false)}, "openai": allOptions}, want: OpenAIToolOptions{DeferLoading: boolPointer(false)}},
		{name: "azure empty object wins", identity: "azure.responses", options: provider.ProviderOptions{"azure": OpenAIToolOptions{}, "openai": allOptions}},
		{name: "azure absent falls back", identity: "azure.responses", options: provider.BuildProviderOptions(allOptions), want: allOptions},
		{name: "azure null falls back", identity: "azure.responses", options: provider.ProviderOptions{"azure": nil, "openai": allOptions}, want: allOptions},
		{name: "default ignores azure", options: provider.ProviderOptions{"azure": allOptions}},
		{name: "default ignores azure despite call fallback", options: provider.ProviderOptions{"azure": allOptions}, azureCall: true},
		{name: "default prefers openai", options: provider.ProviderOptions{"azure": OpenAIToolOptions{}, "openai": allOptions}, want: allOptions},
		{name: "custom non azure ignores azure", identity: "example.responses", options: provider.ProviderOptions{"azure": allOptions}},
	} {
		for _, raw := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				name := tc.name + "/typed/generate"
				if raw {
					name = tc.name + "/json/generate"
				}
				if stream {
					name = strings.TrimSuffix(name, "generate") + "stream"
				}
				t.Run(name, func(t *testing.T) {
					options := tc.options
					before, err := json.Marshal(options)
					require.NoError(t, err)
					if raw {
						require.NoError(t, json.Unmarshal(before, &options))
					}
					var requestBody []byte
					client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						requestBody, err = io.ReadAll(req.Body)
						require.NoError(t, err)
						response := `{"id":"resp_1","created_at":1700000000,"model":"gpt-6-astra","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
						contentType := "application/json"
						if stream {
							response = "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":0,\"response\":" + response + "}\n\n"
							contentType = "text/event-stream"
						}
						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
					})}
					model := NewResponses("test-key", "gpt-6-astra", WithProviderName(tc.identity), WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
					opts := provider.CallOptions{
						Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call_1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}))},
						Tools: []provider.Tool{
							{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "toolSearch"},
							{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), ProviderOptions: options},
						},
					}
					if tc.azureCall {
						opts.ProviderOptions = withAzureOptions(t, OpenAIResponsesOptions{Instructions: "azure-only"})
					}
					var request *provider.RequestMetadata
					if stream {
						result, err := model.DoStream(t.Context(), opts)
						require.NoError(t, err)
						for part := range result.Stream {
							assert.NotEqual(t, provider.PartError, part.Type)
						}
						request = result.Request
					} else {
						result, err := model.DoGenerate(t.Context(), opts)
						require.NoError(t, err)
						request = result.Request
					}
					require.NotNil(t, request)
					assert.JSONEq(t, string(requestBody), string(request.Body))
					var body map[string]any
					require.NoError(t, json.Unmarshal(requestBody, &body))
					fn := map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, "strict": false}
					if tc.want.Async != nil {
						fn["async"] = *tc.want.Async
					}
					if tc.want.DeferLoading != nil {
						fn["defer_loading"] = *tc.want.DeferLoading
					}
					if tc.want.AllowedCallers != nil {
						fn["allowed_callers"] = []any{"programmatic"}
					}
					if tc.want.OutputSchema != nil {
						fn["output_schema"] = map[string]any{"type": "string"}
					}
					var declaration any = fn
					if ns := tc.want.Namespace; ns != nil {
						declaration = map[string]any{"type": "namespace", "name": ns.Name, "description": ns.Description, "tools": []any{fn}}
					}
					assert.Equal(t, []any{map[string]any{"type": "tool_search"}, declaration}, body["tools"])
					output := "ok"
					if tc.want.OutputSchema != nil {
						output = `"ok"`
					}
					assert.Equal(t, []any{map[string]any{"type": "function_call_output", "call_id": "call_1", "output": output}}, body["input"])
					after, err := json.Marshal(options)
					require.NoError(t, err)
					assert.JSONEq(t, string(before), string(after))
				})
			}
		}
	}
}

func TestResponses_FunctionToolOptionsNamespaceInvalid(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options provider.ProviderOptions
		wantKey string
	}{
		{name: "invalid selected object", options: provider.ProviderOptions{"azure": provider.RawProviderOption{Key: "azure", Raw: json.RawMessage(`{"async":"invalid"}`)}, "openai": OpenAIToolOptions{}}, wantKey: "azure"},
		{name: "wrong selected type", options: provider.ProviderOptions{"azure": OpenAIResponsesOptions{}, "openai": OpenAIToolOptions{}}, wantKey: "azure"},
		{name: "invalid fallback", options: provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"deferLoading":"invalid"}`)}}, wantKey: "openai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, nil
			})}
			model := NewResponses("test-key", "gpt-6-astra", WithProviderName("azure.responses"), WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			opts := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", ProviderOptions: tc.options}}}
			_, err := model.DoGenerate(t.Context(), opts)
			require.ErrorContains(t, err, `"`+tc.wantKey+`"`)
			_, err = model.DoStream(t.Context(), opts)
			require.ErrorContains(t, err, `"`+tc.wantKey+`"`)
			assert.Zero(t, requests)
		})
	}
}
