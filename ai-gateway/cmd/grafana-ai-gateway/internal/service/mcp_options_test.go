package service

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeMCP_ResolvedConfiguration(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name, options  string
				denied, server bool
			}{
				{name: "missing", options: "{}"},
				{name: "empty", options: `{"mcpServers":[]}`},
				{name: "null", options: `{"mcpServers":null}`},
				{name: "canonical", options: `{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test","authorizationToken":"","toolConfiguration":{"enabled":false,"allowedTools":[]},"extension":{"ignored":true}}]}`, server: true},
				{name: "case alias", options: `{"MCPServers":[{"NAME":"echo","URL":"https://mcp.example.test"}]}`, server: true},
				{name: "last case-colliding field wins", options: `{"mcpServers":[{"name":"bad","url":"http://mcp.example.test"}],"MCPServers":[{"name":"echo","url":"https://mcp.example.test"}]}`, server: true},
				{name: "last duplicate field wins", options: `{"mcpServers":[{"name":"bad","url":"http://mcp.example.test"}],"mcpServers":[{"name":"echo","url":"https://mcp.example.test"}]}`, server: true},
				{name: "unconsumed type", options: `{"mcpServers":[{"type":{"future":true},"name":"echo","url":"https://mcp.example.test"}]}`, server: true},
				{name: "wrong list type", options: `{"mcpServers":"bad"}`, denied: true},
				{name: "null entry", options: `{"mcpServers":[null]}`, denied: true},
				{name: "insecure destination", options: `{"mcpServers":[{"name":"echo","url":"http://mcp.example.test"}]}`, denied: true},
				{name: "destination credentials", options: `{"mcpServers":[{"name":"echo","url":"https://user:secret@mcp.example.test"}]}`, denied: true},
				{name: "destination fragment", options: `{"mcpServers":[{"name":"echo","url":"https://mcp.example.test/#"}]}`, denied: true},
				{name: "duplicate name", options: `{"mcpServers":[{"name":"echo","url":"https://mcp.example.test"},{"name":"echo","url":"https://other.example.test"}]}`, denied: true},
				{name: "invalid enabled", options: `{"mcpServers":[{"name":"echo","url":"https://mcp.example.test","toolConfiguration":{"enabled":"no"}}]}`, denied: true},
				{name: "invalid allowed tool", options: `{"mcpServers":[{"name":"echo","url":"https://mcp.example.test","toolConfiguration":{"allowedTools":[null]}}]}`, denied: true},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/fallback=%t", tc.name, streaming, fallback), func(t *testing.T) {
					body := `{"prompt":[],"providerOptions":{"anthropic":` + tc.options + `}}`
					response, requests := nativeOptionsRequest(t, "anthropic", body, streaming, fallback)
					if tc.denied {
						require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
						assert.Empty(t, requests)
						return
					}
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					count := 1
					if fallback {
						count = 2
					}
					require.Len(t, requests, count)
					for _, request := range requests {
						assert.Equal(t, "backend", request["model"])
						if !tc.server {
							assert.NotContains(t, request, "mcp_servers")
							continue
						}
						servers := request["mcp_servers"].([]any)
						require.Len(t, servers, 1)
						server := servers[0].(map[string]any)
						assert.Equal(t, "url", server["type"])
						assert.Equal(t, "echo", server["name"])
						assert.Equal(t, "https://mcp.example.test", server["url"])
						assert.NotContains(t, server, "extension")
						if tc.name == "canonical" {
							assert.Equal(t, "", server["authorization_token"])
							assert.Equal(t, map[string]any{"enabled": false, "allowed_tools": []any{}}, server["tool_configuration"])
						}
					}
				})
			}
		}
	}
}

func TestNativeMCP_ConsumedHistory(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name, marker  string
				owned, denied bool
			}{
				{name: "native continuation", marker: `{"type":"mcp-tool-use","serverName":"echo","caller":null,"extension":{}}`, owned: true},
				{name: "case-consumed type", marker: `{"Type":"mcp-tool-use","serverName":"echo"}`, owned: true},
				{name: "unconfigured consumed server", marker: `{"type":"mcp-tool-use","serverName":"other"}`, owned: true, denied: true},
				{name: "missing consumed server", marker: `{"type":"mcp-tool-use"}`, owned: true, denied: true},
				{name: "null consumed server", marker: `{"type":"mcp-tool-use","serverName":null}`, owned: true, denied: true},
				{name: "server alias not consumed", marker: `{"type":"mcp-tool-use","ServerName":"echo"}`, owned: true, denied: true},
				{name: "local marker is inert", marker: `{"type":"mcp-tool-use","serverName":"other"}`},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/fallback=%t", tc.name, streaming, fallback), func(t *testing.T) {
					body := fmt.Sprintf(`{"providerOptions":{"anthropic":{"mcpServers":[{"name":"echo","url":"https://mcp.example.test"}]}},"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerExecuted":%t,"providerOptions":{"anthropic":%s}},{"type":"tool-result","toolCallId":"call","toolName":"lookup","output":{"type":"json","value":[{"type":"text","text":"done"}]},"providerOptions":{"anthropic":{"serverName":"irrelevant","type":"future","caller":null}}}]}]}`, tc.owned, tc.marker)
					response, requests := nativeOptionsRequest(t, "anthropic", body, streaming, fallback)
					if tc.denied {
						require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
						assert.Empty(t, requests)
						return
					}
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					count := 1
					if fallback {
						count = 2
					}
					require.Len(t, requests, count)
					for _, request := range requests {
						message := request["messages"].([]any)[0].(map[string]any)
						call := message["content"].([]any)[0].(map[string]any)
						if tc.owned {
							assert.Equal(t, "mcp_tool_use", call["type"])
							assert.Equal(t, "echo", call["server_name"])
							result := message["content"].([]any)[1].(map[string]any)
							assert.Equal(t, "mcp_tool_result", result["type"])
							assert.Equal(t, "call", result["tool_use_id"])
						} else {
							assert.Equal(t, "tool_use", call["type"])
							assert.NotContains(t, call, "server_name")
						}
					}
				})
			}
		}
	}
}
