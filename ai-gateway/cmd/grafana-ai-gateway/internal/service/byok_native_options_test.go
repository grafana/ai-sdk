package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBYOKSelection_NativeOptionGuards(t *testing.T) {
	const mcpOptions = `{"anthropic":{"mcpServers":[{"name":"external","url":"https://example.invalid"}]}}`
	const mcpHistory = `[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"external"}}}]}]`
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name, options, prompt, nativeField string
			allowed                            bool
		}{
			{"MCP servers", mcpOptions, `[]`, "mcp_servers", true},
			{"invalid MCP destination", `{"anthropic":{"mcpServers":[{"name":"external","url":"http://example.invalid"}]}}`, `[]`, "", false},
			{"container skills", `{"anthropic":{"container":{"skills":[{"type":"custom","skillId":"external"}]}}}`, `[]`, "", false},
			{"native routing", `{"anthropic":{"fallbacks":"default"}}`, `[]`, "", false},
			{"unconfigured MCP history", `{}`, mcpHistory, "", false},
			{"configured MCP history", mcpOptions, mcpHistory, "mcp_tool_use", true},
			{"local MCP marker", `{}`, `[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":{"anthropic":{"type":"mcp-tool-use"}}}]}]`, "tool_use", true},
			{"ordinary fields and unrelated namespaces", `{"anthropic":{"model":"ignored","container":{"id":"supplied-container"},"mcp_servers":[]},"other":{"mcpServers":[{"name":"ordinary"}]}}`, `[]`, "supplied-container", true},
		} {
			t.Run(tc.name+"/stream="+strconv.FormatBool(streaming), func(t *testing.T) {
				var calls atomic.Int32
				client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
					calls.Add(1)
					data, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.NotContains(t, string(data), "dummy-request-key")
					assert.NotContains(t, string(data), "byok")
					assert.NotContains(t, string(data), "ignored")
					assert.Contains(t, string(data), tc.nativeField)
					return &http.Response{StatusCode: 401, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"authentication_error","message":"rejected"}}`)), Request: r}, nil
				})}
				factory := func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) { return model, nil }
				language, err := providerv4.New(providerv4.Config{Selector: NewBYOKSelector(client, factory), Limits: serviceTestLimits()})
				require.NoError(t, err)
				writer := providerv4.NewHostErrorWriter()
				authenticator, headers := serviceModeAuthentication(gatewayauth.SourceCloudGateway)
				handler := gatewayauth.Middleware(authenticator, func(w http.ResponseWriter) { writer.Write(w, providerv4.HostErrorAuthentication) }, func(context.Context, gatewayauth.Observation) {}, language)
				var options map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(tc.options), &options))
				options["gateway"] = json.RawMessage(`{"byok":{"anthropic":[{"apiKey":"dummy-request-key"},{"apiKey":"unused-key"}]}}`)
				body, err := json.Marshal(map[string]any{"prompt": json.RawMessage(tc.prompt), "providerOptions": options, "maxOutputTokens": 64})
				require.NoError(t, err)
				r := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(string(body)))
				r.Header = headers
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set(providerv4.HeaderModelID, "anthropic/native-model")
				r.Header.Set(providerv4.HeaderSpecificationVersion, "4")
				r.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if tc.allowed {
					assert.Equal(t, http.StatusFailedDependency, w.Code, w.Body.String())
					assert.Equal(t, int32(1), calls.Load())
				} else {
					assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
					assert.Zero(t, calls.Load())
				}
				assert.NotContains(t, w.Body.String(), "dummy-request-key")
			})
		}
	}
}
