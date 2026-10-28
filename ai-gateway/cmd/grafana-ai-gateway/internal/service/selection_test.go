package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/discovery"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountCatalogSpy struct {
	resolved, listed atomic.Int32
	model            provider.LanguageModel
}

func (c *accountCatalogSpy) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	c.resolved.Add(1)
	return catalog.ResolvedModel{ID: "configured-canonical", Model: c.model}, nil
}

func (c *accountCatalogSpy) ListModels(context.Context) ([]catalog.ModelInfo, error) {
	c.listed.Add(1)
	return []catalog.ModelInfo{{ID: "configured-canonical", Name: "Configured"}}, nil
}

func TestAccountSelection_Isolation(t *testing.T) {
	for _, configuredMode := range []gatewayauth.AccessMode{gatewayauth.AccessModeCloud, gatewayauth.AccessModeSelfHosted} {
		t.Run(string(configuredMode), func(t *testing.T) {
			const body = `{"prompt":[],"providerOptions":{"gateway":{"byok":{"openai":[{"APIKEY":"dummy-request-key","Organization":"request-organization","Project":"request-project","ModelMappings":[],"headers":{"Authorization":"untrusted-key"},"maxRetries":100,"future":true}]}},"openai":{"store":false}}}`
			for _, streaming := range []bool{false, true} {
				t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
					configuredModel := &observabilityTestModel{}
					configured := &accountCatalogSpy{model: configuredModel}
					var byokCalls atomic.Int32
					factory := func(id string, lower provider.LanguageModel) (provider.LanguageModel, error) {
						byokCalls.Add(1)
						assert.Equal(t, "openai/native-model", id)
						assert.Equal(t, "native-model", lower.ModelID())
						return lower, nil
					}
					var nativeCalls atomic.Int32
					client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
						nativeCalls.Add(1)
						assert.Equal(t, "api.openai.com", r.URL.Host)
						assert.Equal(t, "Bearer dummy-request-key", r.Header.Get("Authorization"))
						assert.Equal(t, "request-organization", r.Header.Get("OpenAI-Organization"))
						assert.Equal(t, "request-project", r.Header.Get("OpenAI-Project"))
						raw, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						assert.Contains(t, string(raw), `"store":false`)
						assert.NotContains(t, string(raw), "dummy-request-key")
						assert.NotContains(t, string(raw), "byok")
						assert.NotContains(t, string(raw), "untrusted-key")
						return &http.Response{StatusCode: 401, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"dummy-request-key","type":"authentication_error"}}`)), Request: r}, nil
					})}
					writer := providerv4.NewHostErrorWriter()
					configuredHandler, err := newWireHandler(execution.Config{Selector: NewConfiguredSelector(configured), Limits: serviceTestLimits()})
					require.NoError(t, err)
					byokHandler, err := newWireHandler(execution.Config{Selector: NewBYOKSelector(client, factory), Limits: serviceTestLimits()})
					require.NoError(t, err)
					for _, tc := range []struct {
						name    string
						cloud   bool
						handler http.Handler
						status  int
					}{
						{"byok inference", true, byokHandler, http.StatusFailedDependency},
						{"configured rejects BYOK", false, configuredHandler, http.StatusBadRequest},
						{"BYOK cannot use configured selector", true, configuredHandler, http.StatusForbidden},
						{"configured cannot use BYOK selector", false, byokHandler, http.StatusForbidden},
						{"BYOK discovery", true, discovery.New(configured, providerv4.DiscoveryProjection{}, writer), http.StatusBadRequest},
					} {
						t.Run(tc.name, func(t *testing.T) {
							mode, access := configuredMode, gatewayauth.ConfiguredAccounts
							if tc.cloud {
								mode, access = gatewayauth.AccessModeCloud, gatewayauth.RequestBYOK
							}
							h, headers := withAccess(t, mode, access, tc.handler)
							r := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
							r.Header = headers
							r.Header.Set("Content-Type", "application/json")
							r.Header.Set(providerv4.HeaderModelID, "openai/native-model")
							r.Header.Set(providerv4.HeaderSpecificationVersion, "4")
							r.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
							w := httptest.NewRecorder()
							h.ServeHTTP(w, r)
							assert.Equal(t, tc.status, w.Code)
							if tc.name == "byok inference" {
								assert.Equal(t, []execution.Overview{{RequestedModelID: "openai/native-model", Attempts: []execution.Attempt{
									{Provider: "openai", ModelID: "native-model", Outcome: fallback.AttemptFailed, Error: &execution.Failure{Message: "dummy-request-key", Type: "authentication_error", StatusCode: http.StatusUnauthorized}},
								}}}, responseOverviews(t, w.Body.String()))
								assert.NotContains(t, withoutProviderMetadata(t, w.Body.String()), "dummy-request-key")
							} else {
								assert.NotContains(t, w.Body.String(), "dummy-request-key")
							}
							if tc.name == "BYOK discovery" {
								assert.Contains(t, w.Body.String(), "catalog discovery is unsupported for BYOK")
							}
						})
					}
					assert.Zero(t, configured.resolved.Load())
					assert.Zero(t, configured.listed.Load())
					assert.Equal(t, int32(1), byokCalls.Load())
					assert.Equal(t, int32(1), nativeCalls.Load())
				})
			}
		})
	}
}

func TestBYOKSelection_NativeSummariesAndAttempts(t *testing.T) {
	const nativeMessage = "provider-echo dummy-first dummy-second request-org request-project configured-provider-key configured-exporter-key static-gateway-key"
	for _, policy := range []struct {
		name string
		keys []string
	}{
		{name: "BYOK-only"},
		{name: "configured sources", keys: []string{"configured-provider-key", "configured-exporter-key"}},
		{name: "configured value collision", keys: []string{"configured-provider-key", "configured-exporter-key", "dummy-first"}},
	} {
		expectedMessage := nativeMessage
		for _, key := range policy.keys {
			expectedMessage = strings.ReplaceAll(expectedMessage, key, "[REDACTED]")
		}
		for _, name := range []string{"anthropic", "openai"} {
			for _, mode := range []string{"unary", "setup", "committed"} {
				t.Run(policy.name+"/"+name+"/"+mode, func(t *testing.T) {
					var calls, observed atomic.Int32
					client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)
						if name == "anthropic" {
							assert.Equal(t, "dummy-first", r.Header.Get("X-Api-Key"))
						} else {
							assert.Equal(t, "Bearer dummy-first", r.Header.Get("Authorization"))
						}
						status, contentType := http.StatusUnauthorized, "application/json"
						body := fmt.Sprintf(`{"type":"error","error":{"type":"authentication_error","message":%q}}`, nativeMessage)
						if mode == "committed" {
							status, contentType = http.StatusOK, "text/event-stream"
							if name == "anthropic" {
								body = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"native-model\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
									fmt.Sprintf("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":%q}}\n\n", nativeMessage)
							} else {
								body = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"model\":\"native-model\",\"output\":[]}}\n\n" +
									"event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[]}}\n\n" +
									fmt.Sprintf("event: error\ndata: {\"type\":\"error\",\"code\":\"invalid_api_key\",\"message\":%q}\n\n", nativeMessage)
							}
						}
						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})}
					factory := func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) {
						ordered := model
						switch wrapped := model.(type) {
						case nativeoptions.Model:
							ordered = wrapped.LanguageModel
						}
						switch chain := ordered.(type) {
						case *fallback.Model:
							chain.WithAttemptObserver(func(context.Context, fallback.Attempt) { observed.Add(1) })
						default:
							require.FailNow(t, "expected a BYOK fallback chain")
						}
						return model, nil
					}
					wire, err := newWireHandler(execution.Config{Selector: NewBYOKSelector(client, factory), Limits: serviceTestLimits(), ConfiguredAPIKeys: policy.keys})
					require.NoError(t, err)
					handler, headers := withAccess(t, gatewayauth.AccessModeCloud, gatewayauth.RequestBYOK, wire)
					accountFields := ""
					if name == "openai" {
						accountFields = `,"organization":"request-org","project":"request-project"`
					}
					body := fmt.Sprintf(`{"prompt":[],"maxOutputTokens":32,"providerOptions":{"gateway":{"byok":{%q:[{"apiKey":"dummy-first"%s},{"apiKey":"dummy-second"}]}}}}`, name, accountFields)
					request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
					request.Header = headers
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set(providerv4.HeaderModelID, name+"/native-model")
					request.Header.Set(providerv4.HeaderSpecificationVersion, "4")
					request.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(mode != "unary"))
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					overviews := responseOverviews(t, response.Body.String())
					if mode == "committed" {
						assert.Equal(t, http.StatusOK, response.Code)
						var nativeErrors []json.RawMessage
						errorFrames := 0
						for _, frame := range strings.Split(strings.TrimSpace(response.Body.String()), "\n\n") {
							var event struct {
								Type  string
								Error struct {
									StatusCode int
									Data       struct {
										NativeError json.RawMessage
									}
								}
							}
							require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event))
							if event.Type == "error" {
								errorFrames++
							}
							if len(event.Error.Data.NativeError) != 0 {
								nativeErrors = append(nativeErrors, event.Error.Data.NativeError)
								if name == "anthropic" {
									assert.Equal(t, http.StatusBadGateway, event.Error.StatusCode)
								} else {
									assert.Equal(t, http.StatusFailedDependency, event.Error.StatusCode)
								}
							}
						}
						require.Len(t, nativeErrors, 1)
						if name == "anthropic" {
							assert.JSONEq(t, fmt.Sprintf(`{"message":%q,"type":"api_error","statusCode":500}`, expectedMessage), string(nativeErrors[0]))
						} else {
							assert.JSONEq(t, fmt.Sprintf(`{"message":%q,"type":"error","code":"invalid_api_key","statusCode":400}`, expectedMessage), string(nativeErrors[0]))
						}
						selected := execution.Overview{RequestedModelID: name + "/native-model", Attempts: []execution.Attempt{{Provider: name, ModelID: "native-model", Outcome: fallback.AttemptSelected}}}
						require.Len(t, overviews, errorFrames)
						for _, overview := range overviews {
							assert.Equal(t, selected, overview)
						}
					} else {
						assert.Equal(t, http.StatusFailedDependency, response.Code)
						assert.JSONEq(t, `{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`, withoutProviderMetadata(t, response.Body.String()))
						assert.Equal(t, []execution.Overview{{RequestedModelID: name + "/native-model", Attempts: []execution.Attempt{
							{Provider: name, ModelID: "native-model", Outcome: fallback.AttemptFailed, Error: &execution.Failure{Message: expectedMessage, Type: "authentication_error", StatusCode: http.StatusUnauthorized}},
						}}}, overviews)
					}
					assert.Equal(t, int32(1), calls.Load())
					assert.Equal(t, int32(1), observed.Load())
					assert.NotContains(t, response.Body.String(), `"canonicalModelId"`)
					assert.NotContains(t, response.Body.String(), `"providerInstance"`)
				})
			}
		}
	}
}

func TestBYOKSelection_NativeAttemptIdentity(t *testing.T) {
	const suffix = "native/model/with-slashes"
	for _, name := range []string{"anthropic", "openai"} {
		for _, streaming := range []bool{false, true} {
			t.Run(name+"/"+strconv.FormatBool(streaming), func(t *testing.T) {
				var calls atomic.Int32
				client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
					calls.Add(1)
					raw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(raw), fmt.Sprintf(`"model":%q`, suffix))
					body := fmt.Sprintf(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded %d"}}`, calls.Load())
					return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})}
				body := fmt.Sprintf(`{"prompt":[],"maxOutputTokens":32,"providerOptions":{"gateway":{"byok":{%q:[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}}}`, name)
				response := serveBYOK(t, client, name+"/"+suffix, body, streaming)
				assert.Equal(t, http.StatusServiceUnavailable, response.Code)
				assert.Equal(t, int32(2), calls.Load())
				failed := func(message string) execution.Attempt {
					return execution.Attempt{Provider: name, ModelID: suffix, Outcome: fallback.AttemptFailed, Error: &execution.Failure{Message: message, Type: "overloaded_error", StatusCode: http.StatusServiceUnavailable}}
				}
				assert.Equal(t, []execution.Overview{{RequestedModelID: name + "/" + suffix, Attempts: []execution.Attempt{failed("overloaded 1"), failed("overloaded 2")}}}, responseOverviews(t, response.Body.String()))
			})
		}
	}
}

func TestBYOKSelection_RejectedBeforeAttemptsHasNoOverview(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			var calls atomic.Int32
			client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, fmt.Errorf("unexpected native call")
			})}
			body := `{"prompt":[],"providerOptions":{"gateway":{"byok":{"anthropic":[{"apiKey":"dummy-first"}]}},"anthropic":{"container":{"skills":[{"type":"anthropic","skillId":"skill"}]}}}}`
			response := serveBYOK(t, client, "anthropic/native-model", body, streaming)
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Zero(t, calls.Load())
			assert.NotContains(t, response.Body.String(), "providerMetadata")
		})
	}
}

func TestBYOKSelection_SelectedProviderAccountCount(t *testing.T) {
	accounts := func(count int) string {
		entries := make([]string, count)
		for i := range entries {
			entries[i] = fmt.Sprintf(`{"apiKey":"dummy-%d"}`, i)
		}
		return "[" + strings.Join(entries, ",") + "]"
	}
	for _, tc := range []struct {
		name        string
		byok        string
		status      int
		nativeCalls int32
	}{
		{name: "nine selected accounts", byok: `{"openai":` + accounts(9) + `}`, status: http.StatusBadRequest},
		{name: "nine unused accounts", byok: `{"openai":` + accounts(1) + `,"anthropic":` + accounts(9) + `}`, status: http.StatusFailedDependency, nativeCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"denied"}}`)), Request: r}, nil
			})}
			response := serveBYOK(t, client, "openai/native-model", `{"prompt":[],"providerOptions":{"gateway":{"byok":`+tc.byok+`}}}`, false)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.nativeCalls, calls.Load())
			if tc.status == http.StatusBadRequest {
				assert.JSONEq(t, `{"error":{"message":"providerOptions.gateway.byok requires supported provider arrays with valid accounts","type":"invalid_request_error","param":null,"code":"invalid_request"}}`, response.Body.String())
			}
		})
	}
}

// serveBYOK serves one BYOK ProviderWire request for modelID through the
// Cloud request-account selector with an identity model factory.
func serveBYOK(t *testing.T, client *http.Client, modelID, body string, streaming bool) *httptest.ResponseRecorder {
	t.Helper()
	factory := func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) { return model, nil }
	wire, err := newWireHandler(execution.Config{Selector: NewBYOKSelector(client, factory), Limits: serviceTestLimits()})
	require.NoError(t, err)
	handler, headers := withAccess(t, gatewayauth.AccessModeCloud, gatewayauth.RequestBYOK, wire)
	request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
	request.Header = headers
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderModelID, modelID)
	request.Header.Set(providerv4.HeaderSpecificationVersion, "4")
	request.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// responseOverviews returns every execution overview in a ProviderWire response
// body: the unary document's, or each stream frame's, in order.
func responseOverviews(t *testing.T, body string) []execution.Overview {
	t.Helper()
	var overviews []execution.Overview
	for _, frame := range strings.Split(strings.TrimSpace(body), "\n\n") {
		if frame == "" {
			continue
		}
		var document struct {
			ProviderMetadata map[string]json.RawMessage `json:"providerMetadata"`
			Error            struct {
				Data struct {
					ProviderMetadata map[string]json.RawMessage `json:"providerMetadata"`
				} `json:"data"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &document))
		for _, metadata := range []map[string]json.RawMessage{document.ProviderMetadata, document.Error.Data.ProviderMetadata} {
			var namespace struct {
				Execution *execution.Overview `json:"execution"`
			}
			if raw, ok := metadata["gateway"]; ok {
				require.NoError(t, json.Unmarshal(raw, &namespace))
			}
			if namespace.Execution != nil {
				overviews = append(overviews, *namespace.Execution)
			}
		}
	}
	return overviews
}

// withoutProviderMetadata returns a unary document without its providerMetadata.
func withoutProviderMetadata(t *testing.T, body string) string {
	t.Helper()
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &document))
	delete(document, "providerMetadata")
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	return string(encoded)
}

type selectionTransport func(*http.Request) (*http.Response, error)

func (f selectionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const (
	testInternalService = "internal-service"
	testNamespace       = "stacks-1"
)

type verifiedInfo struct {
	types.AuthInfo
	extra map[string][]string
}

func (info verifiedInfo) GetExtra() map[string][]string { return info.extra }

func verifiedContext(ctx context.Context, service, identity, namespace string) context.Context {
	extra := map[string][]string{}
	if service != "" {
		extra[gatewayauth.DirectCallerServiceKey] = []string{service}
	}
	var claims authn.Claims[authn.AccessTokenClaims]
	claims.Rest.Namespace = namespace
	if typ, uid, err := types.ParseTypeID(identity); err == nil {
		claims.Rest.Actor = &authn.ActorClaims{IDTokenClaims: authn.IDTokenClaims{Type: typ, Identifier: uid}}
	}
	ctx = types.WithAuthInfo(ctx, verifiedInfo{AuthInfo: authn.NewAccessTokenAuthInfo(claims), extra: extra})
	return resource.WithRouteRequestInfo(ctx, &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Namespace: namespace}})
}

func verifiedCaller(mode gatewayauth.AccessMode, internal []string, service, identity, namespace string) gatewayauth.Caller {
	policy := gatewayauth.AccessPolicy{Mode: mode, InternalServices: internal}
	info, _ := types.AuthInfoFrom(verifiedContext(context.Background(), service, identity, namespace))
	return policy.Decide(info, namespace)
}

func withAccess(t *testing.T, mode gatewayauth.AccessMode, access gatewayauth.AccountAccess, next http.Handler) (http.Handler, http.Header) {
	t.Helper()
	service, identity := testInternalService, ""
	switch {
	case access == gatewayauth.RequestBYOK:
		require.Equal(t, gatewayauth.AccessModeCloud, mode)
		service, identity = "front-door", "user:u1"
	case mode == gatewayauth.AccessModeSelfHosted:
		service, identity = "", "service-account:sa1"
	}
	policy := gatewayauth.AccessPolicy{Mode: mode}
	if mode == gatewayauth.AccessModeCloud {
		policy.InternalServices = []string{testInternalService}
	}
	writer := providerv4.NewHostErrorWriter()
	protected := gatewayauth.AccessMiddleware(policy, func(w http.ResponseWriter) { writer.WriteHostError(w, host.ErrorPermission) }, func(context.Context, gatewayauth.Caller) {}, next)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		protected.ServeHTTP(w, request.WithContext(verifiedContext(request.Context(), service, identity, testNamespace)))
	}), http.Header{}
}

func fixedRoute(name string) func(*http.Request) string {
	return func(*http.Request) string { return name }
}
