package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
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

func TestNativeOptions_ConsumedRequests(t *testing.T) {
	for _, configuredFallback := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name    string
				backend string
				body    string
				check   func(*testing.T, map[string]any)
			}{
				{
					name: "anthropic caller and tool scopes", backend: "anthropic",
					body: `{"maxOutputTokens":64,"providerOptions":{"anthropic":{"container":{"id":"supplied-container"},"unknown":{},"model":"ignored"},"other":{"model":"ignored"}},"tools":[{"type":"function","name":"lookup","inputSchema":{},"providerOptions":{"anthropic":{"eagerInputStreaming":false,"allowedCallers":[],"unknown":{}}}}],"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":{"anthropic":{"caller":{"type":"direct"}}}}]},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"lookup","output":{"type":"text","value":"done"},"providerOptions":{"anthropic":{"caller":{"type":"direct"}}}}]}]}`,
					check: func(t *testing.T, body map[string]any) {
						assert.Equal(t, "supplied-container", body["container"].(map[string]any)["id"])
						tool := body["tools"].([]any)[0].(map[string]any)
						assert.NotContains(t, tool, "eager_input_streaming")
						assert.Equal(t, []any{}, tool["allowed_callers"])
						messages := body["messages"].([]any)
						call := messages[0].(map[string]any)["content"].([]any)[0].(map[string]any)
						assert.Equal(t, map[string]any{"type": "direct"}, call["caller"])
						result := messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)
						assert.NotContains(t, result, "caller")
					},
				},
				{
					name: "anthropic provider definitions", backend: "anthropic",
					body: `{"prompt":[],"tools":[{"type":"provider","id":"anthropic.code_execution_20260120","name":"python","args":{}}]}`,
					check: func(t *testing.T, body map[string]any) {
						tools := body["tools"].([]any)
						require.Len(t, tools, 1)
						assert.Equal(t, map[string]any{"type": "code_execution_20260120", "name": "code_execution"}, tools[0])
					},
				},
				{
					name: "anthropic hosted MCP configuration and history", backend: "anthropic",
					body: `{"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"server","url":"https://mcp.example.test/tools","authorizationToken":"remote-token","toolConfiguration":{"enabled":true,"allowedTools":["lookup"]}}]}},"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"server"}}},{"type":"tool-result","toolCallId":"call","toolName":"lookup","output":{"type":"json","value":"done"}}]}]}`,
					check: func(t *testing.T, body map[string]any) {
						assert.Equal(t, []any{map[string]any{"type": "url", "name": "server", "url": "https://mcp.example.test/tools", "authorization_token": "remote-token", "tool_configuration": map[string]any{"enabled": true, "allowed_tools": []any{"lookup"}}}}, body["mcp_servers"])
						parts := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
						assert.Equal(t, map[string]any{"type": "mcp_tool_use", "id": "call", "name": "lookup", "input": map[string]any{}, "server_name": "server"}, parts[0])
						assert.Equal(t, map[string]any{"type": "mcp_tool_result", "tool_use_id": "call", "content": "done", "is_error": false}, parts[1])
					},
				},
				{
					name: "openai hosted MCP definition", backend: "openai",
					body: `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"remote","args":{"serverLabel":"server","serverUrl":"https://mcp.example.test/tools","authorization":"remote-token","headers":{"X-MCP-Key":"remote-header"},"allowedTools":["lookup"]}}]}`,
					check: func(t *testing.T, body map[string]any) {
						assert.Equal(t, []any{map[string]any{"type": "mcp", "server_label": "server", "server_url": "https://mcp.example.test/tools", "authorization": "remote-token", "headers": map[string]any{"X-MCP-Key": "remote-header"}, "allowed_tools": []any{"lookup"}, "require_approval": "never"}}, body["tools"])
					},
				},
				{
					name: "compatible nested extensions", backend: "openai-compatible",
					body: `{"providerOptions":{"my-vllm":{"user":"raw","extension":{"ignored":true}},"openaiCompatible":{"user":"fixed"},"myVllm":{"user":"camel","extension":{"null":null,"false":false,"zero":0,"empty":"","array":[],"object":{}},"headers":{"Authorization":"ordinary"}},"anthropic":{"model":"irrelevant"}},"prompt":[{"role":"user","providerOptions":{"openaiCompatible":{"priority":"high"}},"content":[{"type":"text","text":"one","providerOptions":{"openaiCompatible":{"sentiment":"positive","nested":{}}}},{"type":"text","text":"two"}]}]}`,
					check: func(t *testing.T, body map[string]any) {
						assert.Equal(t, "camel", body["user"])
						assert.Equal(t, map[string]any{"null": nil, "false": false, "zero": float64(0), "empty": "", "array": []any{}, "object": map[string]any{}}, body["extension"])
						message := body["messages"].([]any)[0].(map[string]any)
						assert.Equal(t, "high", message["priority"])
						part := message["content"].([]any)[0].(map[string]any)
						assert.Equal(t, "positive", part["sentiment"])
						assert.Equal(t, map[string]any{}, part["nested"])
					},
				},
				{
					name: "openai azure phase and reasoning", backend: "openai",
					body: `{"providerOptions":{"azure":{"store":false,"model":"ignored","reasoningEffort":"low"}},"prompt":[{"role":"assistant","content":[{"type":"text","text":"answer","providerOptions":{"azure":{"phase":"final_answer","itemId":"msg_supplied"}}},{"type":"reasoning","text":"thought","providerOptions":{"azure":{"itemId":"rs_supplied","reasoningEncryptedContent":"opaque"}}}]}]}`,
					check: func(t *testing.T, body map[string]any) {
						input := body["input"].([]any)
						assert.Equal(t, "final_answer", input[0].(map[string]any)["phase"])
						assert.Equal(t, "rs_supplied", input[1].(map[string]any)["id"])
						assert.Equal(t, "opaque", input[1].(map[string]any)["encrypted_content"])
					},
				},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/fallback=%t", tc.name, streaming, configuredFallback), func(t *testing.T) {
					response, requests := nativeOptionsRequest(t, tc.backend, tc.body, streaming, configuredFallback)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					count := 1
					if configuredFallback {
						count = 2
					}
					require.Len(t, requests, count)
					for _, request := range requests {
						assert.Equal(t, "backend", request["model"])
						tc.check(t, request)
					}
				})
			}
		}
	}

}

func TestNativeOptions_BypassAndOrdinaryControls(t *testing.T) {
	for _, configuredFallback := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name, backend, body string
				denied              bool
			}{
				{"MCP configuration case alias", "anthropic", `{"prompt":[],"providerOptions":{"anthropic":{"MCPServers":[{"type":"url","url":"https://other.example","name":"server"}]}}}`, false},
				{"skills", "anthropic", `{"prompt":[],"providerOptions":{"anthropic":{"container":{"skills":[{"type":"anthropic","skillId":"skill"}]}}}}`, true},
				{"fallback", "anthropic", `{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":"default"}}}`, true},
				{"unconfigured MCP history type alias", "anthropic", `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"Type":"mcp-tool-use","serverName":"server"}}}]}]}`, true},
				{"inert local-call MCP marker", "anthropic", `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":{"anthropic":{"Type":"mcp-tool-use","serverName":"server"}}}]}]}`, false},
				{"inert MCP result metadata", "anthropic", `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"call","toolName":"lookup","output":{"type":"json","value":{}},"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"server"}}}]}]}`, false},
				{"noop execution", "anthropic", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[],"container":null,"fallbacks":null,"mcp_servers":[{"ordinary":true}]}}}`, false},
				{"compaction", "anthropic", `{"prompt":[{"role":"assistant","content":[{"type":"text","text":"summary","providerOptions":{"anthropic":{"type":"compaction"}}}]}]}`, false},
				{"irrelevant namespace", "anthropic", `{"prompt":[],"providerOptions":{"openaiCompatible":{"role":"tool","model":"ignored","content":[]}}}`, false},
				{"model", "openai-compatible", `{"prompt":[],"providerOptions":{"my-vllm":{"model":"other"}}}`, true},
				{"camel model", "openai-compatible", `{"prompt":[],"providerOptions":{"myVllm":{"model":"other"}}}`, true},
				{"format", "openai-compatible", `{"prompt":[],"providerOptions":{"myVllm":{"response_format":{"type":"json_object"}}}}`, true},
				{"legacy tools", "openai-compatible", `{"prompt":[],"providerOptions":{"myVllm":{"functions":[]}}}`, true},
				{"message role", "openai-compatible", `{"prompt":[{"role":"system","content":"system","providerOptions":{"openaiCompatible":{"role":"tool"}}}]}`, true},
				{"single user role", "openai-compatible", `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi","providerOptions":{"openaiCompatible":{"role":"tool"}}}]}]}`, true},
				{"user content type", "openai-compatible", `{"prompt":[{"role":"user","content":[{"type":"text","text":"one","providerOptions":{"openaiCompatible":{"type":"image_url"}}},{"type":"text","text":"two"}]}]}`, true},
				{"assistant call", "openai-compatible", `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":{"openaiCompatible":{"function":{"name":"other","arguments":"{}"}}}}]}]}`, true},
				{"native precedence", "openai-compatible", `{"prompt":[],"providerOptions":{"myVllm":{"messages":[],"tools":[],"tool_choice":"required","stream":true,"stream_options":{}},"openaiCompatible":{"model":"ignored"}}}`, false},
				{"ignored scope", "openai-compatible", `{"prompt":[{"role":"assistant","content":[{"type":"text","text":"hi","providerOptions":{"openaiCompatible":{"role":"tool","type":"ordinary"}}}]}]}`, false},
				{"case significant", "openai-compatible", `{"prompt":[],"providerOptions":{"MyVllm":{"model":"ignored"},"myVllm":{"Model":"ordinary","baseURL":"https://other.example"}}}`, false},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/fallback=%t", tc.name, streaming, configuredFallback), func(t *testing.T) {
					response, requests := nativeOptionsRequest(t, tc.backend, tc.body, streaming, configuredFallback)
					if tc.denied {
						assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
						assert.Empty(t, requests)
					} else {
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						count := 1
						if configuredFallback {
							count = 2
						}
						require.Len(t, requests, count)
						assert.Equal(t, "backend", requests[count-1]["model"])
					}
				})
			}
		}
	}

}

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
				handler, err := providerv4.New(providerv4.Config{Selector: providerv4.CatalogSelector(catalog), Limits: serviceTestLimits()})
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

func nativeOptionsRequest(t *testing.T, backend, body string, streaming, configuredFallback bool, factories ...ModelFactory) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		if backend == "anthropic" {
			assert.Equal(t, "native-key", r.Header.Get("X-Api-Key"))
		} else {
			assert.Equal(t, "Bearer native-key", r.Header.Get("Authorization"))
		}
		if strings.HasPrefix(r.URL.Path, "/primary/") {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"error":{"type":"api_error","message":"temporary"}}`)
			return
		}
		writeNativeOptionsResponse(w, backend, streaming)
	}))
	defer server.Close()
	factory := identityModelFactory
	if len(factories) > 0 {
		factory = factories[0]
	}
	model := config.Model{Name: "Public", Primary: config.Primary{Provider: "native", Model: "backend"}}
	if configuredFallback {
		model.Primary.Provider = "primary"
		model.Fallback = []config.Primary{{Provider: "native", Model: "backend"}}
	}
	created, err := BuildCatalog(config.File{Models: map[string]config.Model{"public": model}}, map[string]config.ResolvedProvider{
		"native":  {Type: backend, APIKey: "native-key", BaseURL: server.URL, ProviderName: "my-vllm.chat"},
		"primary": {Type: backend, APIKey: "native-key", BaseURL: server.URL + "/primary", ProviderName: "my-vllm.chat"},
	}, server.Client(), factory)
	require.NoError(t, err)
	handler, err := providerv4.New(providerv4.Config{Selector: providerv4.CatalogSelector(created), Limits: serviceTestLimits()})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "public")
	request.Header.Set(providerv4.HeaderStreaming, fmt.Sprint(streaming))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code == http.StatusOK && streaming {
		assertNativeOptionsStream(t, response.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	return response, append([]map[string]any(nil), requests...)
}

func assertNativeOptionsStream(t *testing.T, body string) {
	t.Helper()
	finishes := 0
	var last provider.StreamPartType
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var part struct {
			Type         provider.StreamPartType `json:"type"`
			FinishReason *provider.FinishReason  `json:"finishReason"`
			Usage        *provider.Usage         `json:"usage"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &part))
		assert.NotEqual(t, provider.PartError, part.Type, data)
		last = part.Type
		if part.Type == provider.PartFinish {
			finishes++
			require.NotNil(t, part.FinishReason)
			assert.Equal(t, provider.FinishReasonStop, part.FinishReason.Unified)
			require.NotNil(t, part.Usage)
			require.NotNil(t, part.Usage.InputTokens.Total)
			require.NotNil(t, part.Usage.OutputTokens.Total)
			assert.Equal(t, 1, *part.Usage.InputTokens.Total)
			assert.Equal(t, 1, *part.Usage.OutputTokens.Total)
		}
	}
	assert.Equal(t, 1, finishes)
	assert.Equal(t, provider.PartFinish, last)
}

func TestNativeOptions_CaptureIndependence(t *testing.T) {
	for _, configuredFallback := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/fallback=%t", streaming, configuredFallback), func(t *testing.T) {
				env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
					configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
					configuration.Hooks = agento11y.HooksConfig{Enabled: false}
				})
				var logs lockedBuffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				telemetry, err := NewTelemetry(logger)
				require.NoError(t, err)
				runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
				factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
				require.NoError(t, err)
				body := `{"providerOptions":{"myVllm":{"option_marker":{"opaque":"private-option-marker"}}},"prompt":[{"role":"user","content":[{"type":"text","text":"request","providerOptions":{"openaiCompatible":{"scope_marker":"private-scope-marker"}}}]}]}`
				plain, plainRequests := nativeOptionsRequest(t, "openai-compatible", body, streaming, configuredFallback)
				observed, observedRequests := nativeOptionsRequest(t, "openai-compatible", body, streaming, configuredFallback, factory)
				require.Equal(t, http.StatusOK, plain.Code)
				require.Equal(t, http.StatusOK, observed.Code)
				assert.Equal(t, plainRequests, observedRequests)
				assert.Equal(t, plain.Body.String(), observed.Body.String())
				denied, deniedRequests := nativeOptionsRequest(t, "openai-compatible", `{"prompt":[],"providerOptions":{"gateway":{"byok":{"apiKey":"rejected-credential-marker"}}}}`, streaming, configuredFallback, factory)
				assert.Equal(t, http.StatusBadRequest, denied.Code)
				assert.Empty(t, deniedRequests)
				assert.NotContains(t, denied.Body.String(), "rejected-credential-marker")
				env.Shutdown(t)
				generation, err := json.Marshal(env.SingleGenerationJSON(t))
				require.NoError(t, err)
				capture := string(generation) + logs.String() + testMetrics(t, telemetry)
				for _, marker := range []string{"private-option-marker", "private-scope-marker", "rejected-credential-marker", "native-key"} {
					assert.NotContains(t, capture, marker)
				}
			})
		}
	}

}

func writeNativeOptionsResponse(w http.ResponseWriter, backend string, streaming bool) {
	w.Header().Set("Content-Type", "application/json")
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		switch backend {
		case "anthropic":
			_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"backend\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		case "openai":
			_, _ = fmt.Fprint(w, `data: {"type":"response.completed","response":{"id":"resp","model":"backend","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`+"\n\n")
		default:
			_, _ = fmt.Fprint(w, `data: {"id":"chat","model":"backend","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`+"\n\ndata: [DONE]\n\n")
		}
		return
	}
	switch backend {
	case "anthropic":
		_, _ = fmt.Fprint(w, `{"id":"msg","type":"message","role":"assistant","model":"backend","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	case "openai":
		_, _ = fmt.Fprint(w, `{"id":"resp","model":"backend","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	default:
		_, _ = fmt.Fprint(w, `{"id":"chat","model":"backend","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}
}
