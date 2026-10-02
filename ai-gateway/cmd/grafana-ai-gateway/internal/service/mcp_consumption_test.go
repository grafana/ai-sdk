package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeMCP_ActualCandidateConsumption(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name, namespace, options              string
			failPrimary, denied, reachesAnthropic bool
		}{
			{name: "unconsumed malformed foreign options", namespace: "my-vllm.chat", options: `{"mcpServers":"foreign"}`},
			{name: "compatible namespace collision forwards extensions", namespace: "anthropic.chat", options: `{"mcpServers":"foreign"}`},
			{name: "next native consumer validates", namespace: "my-vllm.chat", options: `{"mcpServers":"foreign"}`, failPrimary: true, denied: true},
			{name: "next native consumer projects", namespace: "my-vllm.chat", options: `{"mcpServers":[{"name":"echo","url":"https://mcp.example.test","authorizationToken":"mcp-secret"}]}`, failPrimary: true, reachesAnthropic: true},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				var compatibleCalls, anthropicCalls atomic.Int32
				compatible := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					compatibleCalls.Add(1)
					assert.Equal(t, "Bearer compatible-key", request.Header.Get("Authorization"))
					var body map[string]any
					require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
					assert.Equal(t, "compatible-model", body["model"])
					if tc.namespace == "anthropic.chat" {
						assert.Equal(t, "foreign", body["mcpServers"])
					} else {
						assert.NotContains(t, body, "mcpServers")
						assert.NotContains(t, body, "mcp_servers")
					}
					if tc.failPrimary {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					writeNativeOptionsResponse(w, "openai-compatible", streaming)
				}))
				defer compatible.Close()
				anthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					anthropicCalls.Add(1)
					assert.Equal(t, "anthropic-key", request.Header.Get("X-Api-Key"))
					var body map[string]any
					require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
					assert.Equal(t, "anthropic-model", body["model"])
					servers := body["mcp_servers"].([]any)
					require.Len(t, servers, 1)
					assert.Equal(t, "mcp-secret", servers[0].(map[string]any)["authorization_token"])
					writeNativeOptionsResponse(w, "anthropic", streaming)
				}))
				defer anthropic.Close()
				catalog, err := BuildCatalog(config.File{Models: map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "compatible", Model: "compatible-model"}, Fallback: []config.Primary{{Provider: "anthropic", Model: "anthropic-model"}}}}}, map[string]config.ResolvedProvider{
					"compatible": {Type: "openai-compatible", APIKey: "compatible-key", BaseURL: compatible.URL, ProviderName: tc.namespace},
					"anthropic":  {Type: "anthropic", APIKey: "anthropic-key", BaseURL: anthropic.URL},
				}, http.DefaultClient, identityModelFactory)
				require.NoError(t, err)
				handler, err := providerv4.New(providerv4.Config{Resolver: catalog, Limits: serviceTestLimits()})
				require.NoError(t, err)
				request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[],"providerOptions":{"anthropic":`+tc.options+`}}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
				request.Header.Set(providerv4.HeaderModelID, "public")
				request.Header.Set(providerv4.HeaderStreaming, fmt.Sprint(streaming))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				status := http.StatusOK
				if tc.denied {
					status = http.StatusBadRequest
				}
				require.Equal(t, status, response.Code, response.Body.String())
				assert.EqualValues(t, 1, compatibleCalls.Load())
				wantAnthropic := 0
				if tc.reachesAnthropic {
					wantAnthropic = 1
				}
				assert.EqualValues(t, wantAnthropic, anthropicCalls.Load())
				assert.NotContains(t, response.Body.String(), "mcp-secret")
			})
		}
	}
}
