package v4

import (
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeMCPOptions_AreOpaque(t *testing.T) {
	for _, body := range []string{
		`{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":"not-interpreted","extension":{}}}}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"mcp","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"Type":"mcp-tool-use","serverName":"unconfigured","caller":null,"extension":[]}}}]}],"providerOptions":{"anthropic":{"mcpServers":[null]}}}`,
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(body, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				request := validRequest(body)
				if streaming {
					request.Header.Set(HeaderStreaming, "true")
				}
				response := harness.serve(request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Equal(t, 1, harness.model.callCount())
				options := harness.model.receivedOptions()
				value, ok := options.ProviderOptions["anthropic"].(provider.RawProviderOption)
				require.True(t, ok)
				assert.NotEmpty(t, value.Raw)
			})
		}
	}
}
