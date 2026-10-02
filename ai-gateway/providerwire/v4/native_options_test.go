package v4

import (
	"encoding/json"
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
			opaque := ` {"future":{"null":null,"false":false,"zero":0,"empty":"","array":[],"object":{}},"model":"ordinary","type":"ordinary"} `
			options := `{"futureNamespace":` + opaque + `,"empty":{},"FutureNamespace":{"case":true}}`
			body := `{"providerOptions":` + options + `,"tools":[{"type":"function","name":"lookup","inputSchema":{},"providerOptions":` + options + `}],"prompt":[` +
				`{"role":"system","content":"system","providerOptions":` + options + `},` +
				`{"role":"user","providerOptions":` + options + `,"content":[{"type":"text","text":"hi","providerOptions":` + options + `},{"type":"file","mediaType":"application/pdf","data":{"type":"data","data":""},"providerOptions":` + options + `}]},` +
				`{"role":"assistant","content":[{"type":"reasoning","text":"thought","providerOptions":` + options + `},{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":` + options + `}]},` +
				`{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"lookup","providerOptions":` + options + `,"output":{"type":"content","value":[{"type":"file","mediaType":"application/pdf","data":{"type":"data","data":""},"providerOptions":` + options + `}]}}]}]}`
			request := validRequest(body)
			if streaming {
				request.Header.Set(HeaderStreaming, "true")
			}
			response := harness.serve(request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			received := harness.model.receivedOptions()
			for _, scoped := range []provider.ProviderOptions{
				received.ProviderOptions, received.Tools[0].ProviderOptions,
				received.Prompt[0].ProviderOptions, received.Prompt[1].ProviderOptions,
				received.Prompt[1].Content[0].ProviderOptions, received.Prompt[1].Content[1].ProviderOptions,
				received.Prompt[2].Content[0].ProviderOptions, received.Prompt[2].Content[1].ProviderOptions,
				received.Prompt[3].Content[0].ProviderOptions, received.Prompt[3].Content[0].Output.Content[0].ProviderOptions,
			} {
				require.Len(t, scoped, 3)
				assert.Equal(t, json.RawMessage(opaque[1:len(opaque)-1]), scoped["futureNamespace"].(provider.RawProviderOption).Raw)
				assert.Equal(t, json.RawMessage(`{}`), scoped["empty"].(provider.RawProviderOption).Raw)
				assert.Contains(t, scoped, "FutureNamespace")
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
