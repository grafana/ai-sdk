package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatBYOK_SharedSelection(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, backend := range []string{"openai", "anthropic"} {
			for _, tc := range []struct {
				name              string
				access            gatewayauth.AccountAccess
				controls, options string
				status, calls     int
			}{
				{"all failed", gatewayauth.RequestBYOK, `{"byok":{"%s":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`, `{}`, 503, 2},
				{"fallback", gatewayauth.RequestBYOK, `{"byok":{"%s":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`, `{}`, 200, 2},
				{"native options", gatewayauth.RequestBYOK, `{"byok":{"%s":[{"apiKey":"dummy-second"}]}}`, `{"openai":{"user":"chat-user"},"anthropic":{"container":{"id":"supplied-container"}}}`, 200, 1},
				{"missing controls", gatewayauth.RequestBYOK, `{}`, `{}`, 400, 0},
				{"configured rejects credentials", gatewayauth.ConfiguredAccounts, `{"byok":{"%s":[{"apiKey":"dummy-second"}]}}`, `{}`, 400, 0},
				{"bad credentials", gatewayauth.RequestBYOK, `{"byok":{"%s":[{"apiKey":""}]}}`, `{}`, 400, 0},
			} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", backend, tc.name, streaming), func(t *testing.T) {
					calls := 0
					client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						assert.Contains(t, []string{"api.openai.com", "api.anthropic.com"}, r.URL.Host)
						data, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						assert.NotContains(t, string(data), "dummy-")
						assert.NotContains(t, string(data), "byok")
						var body map[string]any
						require.NoError(t, json.Unmarshal(data, &body))
						if tc.name == "native options" {
							if backend == "openai" {
								assert.Equal(t, "chat-user", body["user"])
								assert.Equal(t, false, body["store"])
							} else {
								assert.Equal(t, map[string]any{"id": "supplied-container"}, body["container"])
							}
						}
						key := r.Header.Get("Authorization")
						if backend == "anthropic" {
							key = r.Header.Get("X-Api-Key")
						}
						if strings.Contains(key, "dummy-first") || tc.name == "all failed" {
							return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"overloaded_error","message":"dummy-first dummy-second"}}`)), Request: r}, nil
						}
						assert.Contains(t, key, "dummy-second")
						w := httptest.NewRecorder()
						writeNativeOptionsResponse(w, backend, streaming)
						response := w.Result()
						response.Request = r
						return response, nil
					})}
					runtime, err := execution.New(execution.Config{Selector: byokSelector(client, identityModelFactory), Limits: serviceTestLimits()})
					require.NoError(t, err)
					chat, err := chatcompletions.New(runtime)
					require.NoError(t, err)
					handler, headers := withAccess(t, gatewayauth.AccessModeCloud, tc.access, chat)
					var options map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(tc.options), &options))
					controls := tc.controls
					if strings.Contains(controls, "%s") {
						controls = fmt.Sprintf(controls, backend)
					}
					options["gateway"] = json.RawMessage(controls)
					data, err := json.Marshal(map[string]any{"model": backend + "/native-model", "messages": []any{map[string]any{"role": "user", "content": "Hi"}}, "stream": streaming, "providerOptions": options})
					require.NoError(t, err)
					request := httptest.NewRequest(http.MethodPost, chatcompletions.Path, strings.NewReader(string(data)))
					request.Header = headers
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					assert.Equal(t, tc.status, response.Code, response.Body.String())
					assert.Equal(t, tc.calls, calls)
					assert.NotContains(t, response.Body.String(), "dummy-first")
					assert.NotContains(t, response.Body.String(), "dummy-second")
					if tc.status == 200 {
						if !streaming {
							var envelope map[string]any
							require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
							assert.NotContains(t, envelope, "error")
						} else {
							for _, frame := range strings.Split(response.Body.String(), "\n\n") {
								if !strings.HasPrefix(frame, "data: {") {
									continue
								}
								var envelope map[string]any
								require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &envelope))
								assert.NotContains(t, envelope, "error")
							}
						}
						if streaming {
							assert.Contains(t, response.Body.String(), "[DONE]")
						} else {
							assert.Contains(t, response.Body.String(), `"finish_reason":"stop"`)
						}
					}
				})
			}
		}
	}
}

func TestChatProviderOptions_NativeGuards(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct{ backend, namespace, options string }{
			{"anthropic", "anthropic", `{"mcpServers":[{"name":"external","url":"http://unsafe.invalid"}]}`},
			{"anthropic", "anthropic", `{"container":{"skills":[{"type":"custom","skillId":"external"}]}}`},
			{"openai-compatible", "custom", `{"model":"other-account-model"}`},
			{"openai-compatible", "custom", `{"response_format":{"type":"text"}}`},
		} {
			t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.backend, tc.options, streaming), func(t *testing.T) {
				calls := 0
				client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					return nil, fmt.Errorf("unexpected native I/O")
				})}
				catalog, err := BuildCatalog(config.File{Models: map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "native", Model: "backend"}}}}, map[string]config.ResolvedProvider{"native": {Type: tc.backend, APIKey: "synthetic-key", BaseURL: "https://provider.invalid", ProviderName: "custom.chat"}}, client, identityModelFactory)
				require.NoError(t, err)
				runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(catalog), Limits: serviceTestLimits()})
				require.NoError(t, err)
				chat, err := chatcompletions.New(runtime)
				require.NoError(t, err)
				body := fmt.Sprintf(`{"model":"public","messages":[{"role":"user","content":"Hi"}],"stream":%t,"providerOptions":{%q:%s}}`, streaming, tc.namespace, tc.options)
				request := httptest.NewRequest(http.MethodPost, chatcompletions.Path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				chat.ServeHTTP(response, request)
				assert.Equal(t, 400, response.Code, response.Body.String())
				assert.Zero(t, calls)
			})
		}
	}
}

func TestChatBYOK_OfficialGoClient(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			calls := 0
			native := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, "Bearer synthetic-key", r.Header.Get("Authorization"))
				w := httptest.NewRecorder()
				writeNativeOptionsResponse(w, "openai", streaming)
				response := w.Result()
				response.Request = r
				return response, nil
			})}
			runtime, err := execution.New(execution.Config{Selector: byokSelector(native, identityModelFactory), Limits: serviceTestLimits()})
			require.NoError(t, err)
			chat, err := chatcompletions.New(runtime)
			require.NoError(t, err)
			handler, _ := withAccess(t, gatewayauth.AccessModeCloud, gatewayauth.RequestBYOK, chat)
			ingress := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				response := w.Result()
				response.Request = r
				return response, nil
			})}
			client := openai.NewClient(option.WithBaseURL("https://gateway.test/v1/"), option.WithHTTPClient(ingress), option.WithAPIKey("platform-token"), option.WithMaxRetries(0))
			params := openai.ChatCompletionNewParams{Model: "openai/gpt-4.1", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")}}
			extra := option.WithJSONSet("providerOptions", map[string]any{"gateway": map[string]any{"byok": map[string]any{"openai": []any{map[string]any{"apiKey": "synthetic-key"}}}}})
			if streaming {
				stream := client.Chat.Completions.NewStreaming(t.Context(), params, extra)
				count := 0
				for stream.Next() {
					count++
				}
				require.NoError(t, stream.Err())
				assert.Positive(t, count)
				require.NoError(t, stream.Close())
			} else {
				result, err := client.Chat.Completions.New(t.Context(), params, extra)
				require.NoError(t, err)
				require.Len(t, result.Choices, 1)
				assert.Equal(t, "stop", result.Choices[0].FinishReason)
			}
			assert.Equal(t, 1, calls)
		})
	}
}
