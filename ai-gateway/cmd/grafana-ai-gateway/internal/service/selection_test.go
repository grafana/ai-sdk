package service

import (
	"context"
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
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
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
	const body = `{"prompt":[],"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"dummy-request-key","organization":"request-organization","project":"request-project"}]}},"openai":{"store":false}}}`
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
				return &http.Response{StatusCode: 401, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"dummy-request-key","type":"authentication_error"}}`)), Request: r}, nil
			})}
			writer := providerv4.NewHostErrorWriter()
			configuredHandler, err := providerv4.New(providerv4.Config{Selector: NewConfiguredSelector(configured), Limits: serviceTestLimits()})
			require.NoError(t, err)
			byokHandler, err := providerv4.New(providerv4.Config{Selector: NewBYOKSelector(client, factory), Limits: serviceTestLimits()})
			require.NoError(t, err)
			for _, tc := range []struct {
				name    string
				cloud   bool
				handler http.Handler
				status  int
			}{
				{"byok inference", true, byokHandler, http.StatusFailedDependency},
				{"configured rejects BYOK", false, configuredHandler, http.StatusBadRequest},
				{"cloud cannot use configured selector", true, configuredHandler, http.StatusForbidden},
				{"private cannot use BYOK selector", false, byokHandler, http.StatusForbidden},
				{"BYOK discovery", true, BYOKDiscovery(writer), http.StatusBadRequest},
				{"cloud cannot use configured discovery", true, ConfiguredDiscovery(discovery.New(configured, writer), writer), http.StatusForbidden},
			} {
				t.Run(tc.name, func(t *testing.T) {
					source := gatewayauth.SourceAccessToken
					if tc.cloud {
						source = gatewayauth.SourceCloudGateway
					}
					authenticator, headers := serviceModeAuthentication(source)
					h := gatewayauth.Middleware(authenticator, func(w http.ResponseWriter) { writer.Write(w, providerv4.HostErrorAuthentication) }, func(context.Context, gatewayauth.Observation) {}, tc.handler)
					r := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
					r.Header = headers
					r.Header.Set("Content-Type", "application/json")
					r.Header.Set(providerv4.HeaderModelID, "openai/native-model")
					r.Header.Set(providerv4.HeaderSpecificationVersion, "4")
					r.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					assert.Equal(t, tc.status, w.Code)
					assert.NotContains(t, w.Body.String(), "dummy-request-key")
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
}

func TestBYOKSelection_PublicAttributionOmitted(t *testing.T) {
	const nativeMessage = "provider-echo dummy-first dummy-second request-org request-project"
	for _, name := range []string{"anthropic", "openai"} {
		for _, mode := range []string{"unary", "setup", "committed"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				var calls, observed atomic.Int32
				client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
					calls.Add(1)
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
				factory := func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) { return model, nil }
				wire, err := providerv4.New(providerv4.Config{Selector: NewBYOKSelector(client, factory), Limits: serviceTestLimits()})
				require.NoError(t, err)
				writer := providerv4.NewHostErrorWriter()
				authenticator, headers := serviceModeAuthentication(gatewayauth.SourceCloudGateway)
				handler := gatewayauth.Middleware(authenticator, func(w http.ResponseWriter) { writer.Write(w, providerv4.HostErrorAuthentication) }, func(context.Context, gatewayauth.Observation) {}, wire)
				accountFields := ""
				if name == "openai" {
					accountFields = `,"organization":"request-org","project":"request-project"`
				}
				body := fmt.Sprintf(`{"prompt":[],"maxOutputTokens":32,"providerOptions":{"gateway":{"byok":{%q:[{"apiKey":"dummy-first"%s},{"apiKey":"dummy-second"}]}}}}`, name, accountFields)
				ctx := fallback.WithAttemptObserver(t.Context(), func(context.Context, fallback.Attempt) { observed.Add(1) })
				request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body)).WithContext(ctx)
				request.Header = headers
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(providerv4.HeaderModelID, name+"/native-model")
				request.Header.Set(providerv4.HeaderSpecificationVersion, "4")
				request.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(mode != "unary"))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if mode == "committed" {
					assert.Equal(t, http.StatusOK, response.Code)
					assert.Contains(t, response.Body.String(), `"type":"error"`)
				} else {
					assert.Equal(t, http.StatusFailedDependency, response.Code)
					assert.JSONEq(t, `{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`, response.Body.String())
				}
				assert.Equal(t, int32(1), calls.Load())
				assert.Equal(t, int32(1), observed.Load())
				for _, private := range []string{"dummy-first", "dummy-second", "request-org", "request-project", "provider-echo", `"execution"`, `"nativeError"`} {
					assert.NotContains(t, response.Body.String(), private)
				}
			})
		}
	}
}

type selectionTransport func(*http.Request) (*http.Response, error)

func (f selectionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
