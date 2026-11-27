package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	chatPermission   = `{"error":{"message":"permission denied","type":"permission_error","param":null,"code":"permission_denied"}}`
	chatUnavailable  = `{"error":{"message":"service unavailable","type":"server_error","param":null,"code":"unavailable"}}`
	privateExtension = "private-chat-guard"
)

func TestChat_ExtensionOutcomes(t *testing.T) {
	for _, scope := range []extension.Scope{extension.ScopeOperator, extension.ScopeTenant} {
		t.Run(string(scope), func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				err      error
				status   int
				document string
			}{
				{name: "denied", err: fmt.Errorf("%s: %w", privateExtension, extension.ErrDenied), status: http.StatusForbidden, document: chatPermission},
				{name: "fail closed", err: errors.New(privateExtension), status: http.StatusServiceUnavailable, document: chatUnavailable},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for _, responsePhase := range []bool{false, true} {
						name := "request"
						if responsePhase {
							name = "response"
						}
						t.Run(name, func(t *testing.T) {
							guard := gatewayExtension{admit: func() error { return tc.err }}
							if responsePhase {
								guard = gatewayExtension{inspect: func(part extension.Part) error {
									if part.Type == extension.PartText && (part.Text == "second" || part.Text == "hello") {
										return tc.err
									}
									return nil
								}}
							}
							assertChatExtension(t, scope, guard, responsePhase, tc.status, tc.document)
						})
					}
				})
			}
		})
	}
}

func assertChatExtension(t *testing.T, scope extension.Scope, guard gatewayExtension, responsePhase bool, wantStatus int, wantDocument string) {
	t.Helper()
	factory := func(json.RawMessage, extension.Dependencies) (any, error) { return guard, nil }
	ext := gatewayDefinition(privateExtension, guard, func(def *extension.Definition) {
		def.Keys = []extension.KeyOwnership{extension.KeysOperator, extension.KeysCustomer}
	})
	options := harnessOptions{fallback: true}
	if scope == extension.ScopeTenant {
		typ := tenanttype.Type{Name: privateExtension}
		options.tenantTypes = []tenanttype.Type{typ}
		ext = typ.Bind(tenanttype.Implementation{Keys: []extension.KeyOwnership{extension.KeysCustomer}, FailurePolicy: extension.FailClosed, New: factory})
	}
	options.extensions = []extension.Definition{ext}
	h := newHarnessWithOptions(t, platformhost.TopologySelfHosted, options)
	native := newFakeAnthropic(t)
	useNativeAnthropic(t, native)
	storeChatFallbackCatalog(t, h)
	if scope == extension.ScopeTenant {
		storeChatMiddleware(t, h, privateExtension, "")
	}
	auth := bearer(h.host.AddServiceAccount("chat-extension", testNamespace))
	for _, route := range []struct {
		name, url, model, primary string
		provider                  *fakeAnthropic
		applies                   bool
	}{
		{name: "namespace", url: h.url("v1/chat/completions"), model: "public", primary: "backend-private", provider: h.anthropic, applies: scope == extension.ScopeOperator},
		{name: "catalog", url: h.catalogURL("v1/chat/completions"), model: "team", primary: "claude-sonnet-4-6", provider: native, applies: true},
	} {
		t.Run(route.name, func(t *testing.T) {
			for _, streaming := range []bool{false, true} {
				before := route.provider.requests.Load()
				request := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"max_completion_tokens":16,"stream":%t`, route.model, streaming)
				if streaming {
					request += `,"stream_options":{"include_usage":true}`
				}
				status, body := do(t, chatRequest(t, route.url, request+"}", auth))
				switch {
				case !route.applies:
					require.Equal(t, http.StatusOK, status, body)
					if streaming {
						assert.Contains(t, body, "[DONE]")
					} else {
						assert.Contains(t, body, "hello")
					}
				case responsePhase && streaming:
					require.Equal(t, http.StatusOK, status, body)
					frames := strings.Split(strings.TrimSpace(body), "\n\n")
					require.Len(t, frames, 3, body)
					assert.Contains(t, frames[0], `"role":"assistant"`)
					assert.Contains(t, frames[1], `"content":"first"`)
					assert.JSONEq(t, wantDocument, strings.TrimPrefix(frames[2], "data: "))
					assert.NotContains(t, body, "second")
				default:
					assert.Equal(t, wantStatus, status, body)
					assert.JSONEq(t, wantDocument, body)
				}
				wantCalls := before
				if !route.applies || responsePhase {
					wantCalls++
				}
				require.Equal(t, wantCalls, route.provider.requests.Load(), "streaming=%t", streaming)
				if wantCalls > before {
					_, bodies := route.provider.nativeCalls()
					require.Len(t, bodies, int(wantCalls))
					var request struct {
						Model string `json:"model"`
					}
					require.NoError(t, json.Unmarshal([]byte(bodies[before]), &request))
					assert.Equal(t, route.primary, request.Model)
				}
				assert.NotContains(t, body, privateExtension)
				assert.NotContains(t, body, routeCatalogKey)
			}
		})
	}
}

func storeChatFallbackCatalog(t *testing.T, h *harness) {
	t.Helper()
	document := `{"metadata":{"name":"` + testCatalogName + `"},"spec":{
		"providers":{"claude":{"type":"anthropic"}},
		"models":{"team-model":{"name":"Team model","aliases":["team"],
			"primary":{"provider":"claude","model":"claude-sonnet-4-6"},
			"fallback":[{"provider":"claude","model":"chat-fallback"}]}}
	},"secure":{"claude":{"create":"` + routeCatalogKey + `"}}}`
	status, response := sendObject(t, context.Background(), http.MethodPost, catalogURL(h.server.URL, ""), h.serviceToken(t, testInternalService), document)
	require.Equal(t, http.StatusCreated, status, response)
}

func storeChatMiddleware(t *testing.T, h *harness, name, member string) {
	t.Helper()
	status, document := sendObject(t, context.Background(), http.MethodPost, middlewareURL(h.server.URL, ""), h.serviceToken(t, testInternalService), middlewareDocument(name, name, true, member))
	require.Equal(t, http.StatusCreated, status, document)
}
