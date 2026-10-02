package v4

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderOptions_AllSupportedScopes(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(streamingName(streaming), func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			opaque := func(scope string) string {
				return fmt.Sprintf(`{"scope":%q,"future":{"null":null,"false":false,"zero":0,"empty":"","array":[],"object":{}},"model":"ordinary","type":"ordinary"}`, scope)
			}
			options := func(scope string) string {
				return `{"futureNamespace": ` + opaque(scope) + ` ,"empty":{},"FutureNamespace":{"case":true}}`
			}
			body := `{"providerOptions":` + options("call") + `,"tools":[{"type":"function","name":"lookup","inputSchema":{},"providerOptions":` + options("function-tool") + `},{"type":"function","name":"optionless","inputSchema":{}}],"prompt":[` +
				`{"role":"system","content":"system","providerOptions":` + options("system") + `},` +
				`{"role":"user","providerOptions":` + options("user") + `,"content":[{"type":"text","text":"hi","providerOptions":` + options("text") + `},{"type":"file","mediaType":"application/pdf","data":{"type":"data","data":""},"providerOptions":` + options("file") + `},{"type":"text","text":"optionless"}]},` +
				`{"role":"assistant","content":[{"type":"reasoning","text":"thought","providerOptions":` + options("reasoning") + `},{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":` + options("tool-call") + `}]},` +
				`{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"lookup","providerOptions":` + options("tool-result") + `,"output":{"type":"content","value":[{"type":"file","mediaType":"application/pdf","data":{"type":"data","data":""},"providerOptions":` + options("result-file") + `},{"type":"file","mediaType":"application/pdf","data":{"type":"data","data":""}}]}}]}]}`
			scopes := func(received provider.CallOptions) []provider.ProviderOptions {
				return []provider.ProviderOptions{
					received.ProviderOptions, received.Tools[0].ProviderOptions,
					received.Prompt[0].ProviderOptions, received.Prompt[1].ProviderOptions,
					received.Prompt[1].Content[0].ProviderOptions, received.Prompt[1].Content[1].ProviderOptions,
					received.Prompt[2].Content[0].ProviderOptions, received.Prompt[2].Content[1].ProviderOptions,
					received.Prompt[3].Content[0].ProviderOptions, received.Prompt[3].Content[0].Output.Content[0].ProviderOptions,
				}
			}
			names := []string{"call", "function-tool", "system", "user", "text", "file", "reasoning", "tool-call", "tool-result", "result-file"}
			check := func(scoped provider.ProviderOptions, name string) {
				require.Len(t, scoped, 3, name)
				assert.Equal(t, json.RawMessage(opaque(name)), scoped["futureNamespace"].(provider.RawProviderOption).Raw, name)
				assert.Equal(t, json.RawMessage(`{}`), scoped["empty"].(provider.RawProviderOption).Raw, name)
				assert.Equal(t, json.RawMessage(`{"case":true}`), scoped["FutureNamespace"].(provider.RawProviderOption).Raw, name)
			}
			for invocation := range 2 {
				request := validRequest(body)
				if streaming {
					request.Header.Set(HeaderStreaming, "true")
				}
				response := harness.serve(request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				received := harness.model.receivedOptions()
				mapped := scopes(received)
				for index, scoped := range mapped {
					check(scoped, names[index])
				}
				assert.Empty(t, received.Tools[1].ProviderOptions)
				assert.Empty(t, received.Prompt[1].Content[2].ProviderOptions)
				assert.Empty(t, received.Prompt[3].Content[0].Output.Content[1].ProviderOptions)
				if invocation == 0 {
					mapped[0]["futureNamespace"].(provider.RawProviderOption).Raw[0] = '['
					delete(mapped[0], "empty")
					for index := 1; index < len(mapped); index++ {
						check(mapped[index], names[index])
					}
				}
			}
		})
	}
}

func streamingName(streaming bool) string {
	if streaming {
		return "stream"
	}
	return "generate"
}
