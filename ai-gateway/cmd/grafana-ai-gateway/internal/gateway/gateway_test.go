package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const unaryBody = `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"maxOutputTokens":16}`

func TestPluginID_MatchesPluginMetadata(t *testing.T) {
	raw, err := os.ReadFile("../../../../src/plugin.json")
	require.NoError(t, err)
	var metadata struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(raw, &metadata))
	assert.Equal(t, metadata.ID, PluginID)
}

func TestGateway_CloudAccess(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	internal := accessToken(h.serviceToken(t, testInternalService))
	onBehalf := accessToken(h.onBehalfOf(t, testInternalService, types.TypeUser, "private-user-uid"))
	user := session(h.host.AddSession(types.TypeUser, "private-user-uid", testNamespace))
	account := bearer(h.host.AddServiceAccount("private-account-uid", testNamespace))
	anonymous := session(h.host.AddSession(types.TypeAnonymous, "anon", testNamespace))
	other := accessToken(h.serviceToken(t, "svc-b"))

	t.Run("configured discovery for internal callers", func(t *testing.T) {
		for _, auth := range []credential{internal, onBehalf} {
			status, body := do(t, discoveryRequest(t, h.url("config"), auth))
			assert.Equal(t, http.StatusOK, status)
			assert.Contains(t, body, `"id":"public"`)
		}
	})
	t.Run("configured inference for internal callers", func(t *testing.T) {
		for _, auth := range []credential{internal, onBehalf} {
			status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, false, auth))
			assert.Equal(t, http.StatusOK, status, body)
			assert.Contains(t, body, "hello")
		}
	})
	t.Run("BYOK discovery is unsupported for users and service accounts", func(t *testing.T) {
		for _, auth := range []credential{user, account} {
			status, body := do(t, discoveryRequest(t, h.url("config"), auth))
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Contains(t, body, "catalog discovery is unsupported for BYOK")
		}
	})
	t.Run("BYOK callers cannot select configured models", func(t *testing.T) {
		before := h.anthropic.requests.Load()
		status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, false, account))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, body, "BYOK requires a supported provider/model selector")
		assert.Equal(t, before, h.anthropic.requests.Load())
	})
	t.Run("refused callers do no work", func(t *testing.T) {
		before := h.anthropic.requests.Load()
		for _, auth := range []credential{anonymous, other} {
			status, body := do(t, discoveryRequest(t, h.url("config"), auth))
			assert.Equal(t, http.StatusForbidden, status)
			assert.Contains(t, body, "error")
			status, _ = do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, false, auth))
			assert.Equal(t, http.StatusForbidden, status)
		}
		assert.Equal(t, before, h.anthropic.requests.Load())
	})
	t.Run("route metrics and access decisions", func(t *testing.T) {
		metrics := h.metrics(t)
		assert.Contains(t, metrics, `grafana_ai_gateway_http_requests_total{method="GET",route="config",status="2xx"}`)
		assert.Contains(t, metrics, `grafana_ai_gateway_http_requests_total{method="POST",route="language_model",status="2xx"}`)
		assert.Contains(t, metrics, `grafana_ai_gateway_access_decisions_total{decision="configured"}`)
		assert.Contains(t, metrics, `grafana_ai_gateway_access_decisions_total{decision="byok"}`)
		assert.Contains(t, metrics, `grafana_ai_gateway_access_decisions_total{decision="refused"}`)
		for _, private := range []string{testNamespace, testInternalService, "svc-b", "private-user-uid", "private-account-uid", "front-door", providerSecret} {
			assert.NotContains(t, metrics, private)
		}
		sinks := strings.Join(append(h.anthropic.recorded(), h.logs.String()), "\n")
		for _, private := range []string{"private-user-uid", "private-account-uid", "front-door", "svc-b"} {
			assert.NotContains(t, sinks, private)
		}
		assert.Contains(t, h.logs.String(), `"caller_service":"`+testInternalService+`"`)
	})
}

func TestGateway_SelfHostedAccess(t *testing.T) {
	h := newHarness(t, platformhost.TopologySelfHosted)
	account := bearer(h.host.AddServiceAccount("sa1", testNamespace))
	user := session(h.host.AddSession(types.TypeUser, "u1", testNamespace))
	anonymous := session(h.host.AddSession(types.TypeAnonymous, "anon", testNamespace))

	for _, auth := range []credential{account, user} {
		status, body := do(t, discoveryRequest(t, h.url("config"), auth))
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, body, `"id":"public"`)
		status, body = do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, false, auth))
		assert.Equal(t, http.StatusOK, status, body)
	}

	before := h.anthropic.requests.Load()
	byok := `{"prompt":[],"providerOptions":{"gateway":{"byok":{"anthropic":[{"apiKey":"dummy-request-key"}]}}}}`
	status, body := do(t, languageRequest(t, context.Background(), h.url("language-model"), byok, false, account))
	assert.Equal(t, http.StatusBadRequest, status)
	assert.NotContains(t, body, "dummy-request-key")
	status, _ = do(t, discoveryRequest(t, h.url("config"), anonymous))
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, before, h.anthropic.requests.Load())
	assert.NotContains(t, h.metrics(t), `decision="byok"`)
}

func TestGateway_UndeclaredAndLegacyRoutesDoNoWork(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	internal := accessToken(h.serviceToken(t, testInternalService))
	before := h.anthropic.requests.Load()

	status, _ := do(t, languageRequest(t, context.Background(), h.url("config"), unaryBody, false, internal))
	assert.Equal(t, http.StatusMethodNotAllowed, status)
	status, _ = do(t, discoveryRequest(t, h.url("language-model"), internal))
	assert.Equal(t, http.StatusMethodNotAllowed, status)
	status, _ = do(t, discoveryRequest(t, h.url("models"), internal))
	assert.Equal(t, http.StatusNotFound, status)

	for _, path := range []string{"/api/v1/aisdk/config", "/api/v1/aisdk/language-model", "/live", "/ready", "/metrics"} {
		response := httptest.NewRecorder()
		h.gateway.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusNotFound, response.Code, path)
	}
	assert.Equal(t, before, h.anthropic.requests.Load())
	assert.Contains(t, h.metrics(t), `route="unmatched"`)
	assert.NotContains(t, h.metrics(t), `decision=`)
}

func TestGateway_CredentialHeadersFromTheHostStayPrivate(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	token, err := h.host.Exchange(context.Background(), platformhost.RouterService, delegation(h.serviceToken(t, testInternalService)))
	require.NoError(t, err)
	sentinels := map[string]string{
		"Authorization":  "Bearer sentinel-authorization",
		"X-Access-Token": "sentinel-access-token",
		"X-Grafana-Id":   "sentinel-id-token",
		"Cookie":         "grafana_session=sentinel-cookie",
	}
	for _, streaming := range []bool{false, true} {
		request := &pluginv3.CallRouteRequest{}
		request.SetMethod(http.MethodPost)
		request.SetGroup(h.host.Config().Group)
		request.SetVersion(h.host.Config().Version)
		request.SetNamespace(testNamespace)
		request.SetPath("/language-model")
		request.SetUrl(strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + "/language-model")
		headers := map[string]*pluginv3.StringList{
			"Content-Type":                        pluginv3.StringList_builder{Values: []string{"application/json"}}.Build(),
			providerv4.HeaderModelID:              pluginv3.StringList_builder{Values: []string{"public"}}.Build(),
			providerv4.HeaderSpecificationVersion: pluginv3.StringList_builder{Values: []string{providerv4.SpecificationVersion}}.Build(),
			providerv4.HeaderStreaming:            pluginv3.StringList_builder{Values: []string{strconv.FormatBool(streaming)}}.Build(),
		}
		for name, value := range sentinels {
			headers[name] = pluginv3.StringList_builder{Values: []string{value}}.Build()
		}
		request.SetHeaders(headers)
		request.SetBody([]byte(unaryBody))
		stream, err := h.served.BareRouteClient().CallRoute(metadata.AppendToOutgoingContext(context.Background(), "x-access-token", token), request)
		require.NoError(t, err)
		var body strings.Builder
		for {
			response, err := stream.Recv()
			if err != nil {
				break
			}
			body.Write(response.GetBody())
		}
		if streaming {
			assert.Contains(t, body.String(), `"delta":"second"`)
		} else {
			assert.Contains(t, body.String(), "hello")
		}
		for _, value := range sentinels {
			assert.NotContains(t, body.String(), strings.TrimPrefix(value, "Bearer "))
		}
	}
	serialized := strings.Join(append(h.anthropic.recorded(), h.logs.String(), h.metrics(t)), "\n")
	for _, value := range []string{"sentinel-authorization", "sentinel-access-token", "sentinel-id-token", "sentinel-cookie"} {
		assert.NotContains(t, serialized, value)
	}
}

func TestGateway_StreamsIncrementallyAndCancels(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	internal := accessToken(h.serviceToken(t, testInternalService))
	release := h.anthropic.hold()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response, err := http.DefaultClient.Do(languageRequest(t, ctx, h.url("language-model"), unaryBody, true, internal))
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)

	reader := bufio.NewReader(response.Body)
	readUntil := func(marker string) {
		t.Helper()
		done := make(chan bool, 1)
		go func() {
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					done <- false
					return
				}
				if strings.Contains(line, marker) {
					done <- true
					return
				}
			}
		}()
		select {
		case found := <-done:
			require.True(t, found, marker)
		case <-time.After(3 * time.Second):
			t.Fatalf("frame %q did not arrive while the provider was paused", marker)
		}
	}
	readUntil(`"delta":"first"`)
	select {
	case <-h.anthropic.canceled:
		t.Fatal("provider stream ended before cancellation")
	default:
	}
	assert.Contains(t, h.metrics(t), `grafana_ai_gateway_http_requests_in_flight{method="POST",route="language_model"} 1`)

	cancel()
	select {
	case <-h.anthropic.canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("caller cancellation did not cancel the provider call")
	}
	close(release)
	require.Eventually(t, func() bool {
		return strings.Contains(h.metrics(t), `grafana_ai_gateway_http_requests_in_flight{method="POST",route="language_model"} 0`)
	}, 3*time.Second, 10*time.Millisecond)
}
