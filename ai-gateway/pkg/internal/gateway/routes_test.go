package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/manifestdata"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/plugin/appadapter"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

const (
	testNamespace       = "stacks-1"
	testInternalService = "svc-a"
	providerSecret      = "provider-secret-value"
	unaryMessage        = `{"id":"msg_1","type":"message","role":"assistant","model":"backend-private","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
)

type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type fakeAnthropic struct {
	server   *httptest.Server
	requests atomic.Int32
	block    atomic.Bool
	blocked  chan error
	release  chan struct{}
	canceled chan struct{}
	mu       sync.Mutex
	bodies   []string
	headers  []http.Header
}

func newFakeAnthropic(t *testing.T) *fakeAnthropic {
	t.Helper()
	fake := &fakeAnthropic{canceled: make(chan struct{}), blocked: make(chan error, 8)}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.bodies = append(fake.bodies, string(body))
		fake.headers = append(fake.headers, r.Header.Clone())
		release := fake.release
		fake.mu.Unlock()
		if fake.block.Load() {
			<-r.Context().Done()
			fake.blocked <- r.Context().Err()
			return
		}
		if !strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, unaryMessage)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		event := func(name, data string) {
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
			w.(http.Flusher).Flush()
		}
		event("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"backend-private","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
		event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`)
		if release != nil {
			select {
			case <-release:
			case <-r.Context().Done():
				close(fake.canceled)
				return
			}
		}
		event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"second"}}`)
		event("content_block_stop", `{"type":"content_block_stop","index":0}`)
		event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":2}}`)
		event("message_stop", `{"type":"message_stop"}`)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeAnthropic) hold() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.release = make(chan struct{})
	return f.release
}

func (f *fakeAnthropic) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	values := append([]string(nil), f.bodies...)
	for _, headers := range f.headers {
		for name, entries := range headers {
			values = append(values, name+": "+strings.Join(entries, ","))
		}
	}
	return values
}

func writeModelConfig(t *testing.T, baseURL string, fallback bool) string {
	t.Helper()
	contents := fmt.Sprintf(`providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_SECRET
    baseURL: %s
models:
  public:
    name: Public
    primary:
      provider: anthropic-primary
      model: backend-private
`, baseURL)
	if fallback {
		contents += `    fallback:
      - provider: anthropic-primary
        model: backend-fallback
`
	}
	path := filepath.Join(t.TempDir(), "models.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

type harness struct {
	host      *platformhost.Host
	served    *platformhost.InProcess
	server    *httptest.Server
	gateway   *Gateway
	registry  *prometheus.Registry
	logs      *syncBuffer
	anthropic *fakeAnthropic
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

type harnessOptions struct {
	routeDeadline time.Duration
	fallback      bool
}

func newHarness(t *testing.T, topology platformhost.Topology) *harness {
	t.Helper()
	return newHarnessWithOptions(t, topology, harnessOptions{})
}

func newHarnessWithOptions(t *testing.T, topology platformhost.Topology, options harnessOptions) *harness {
	t.Helper()
	data := manifestdata.LocalManifest().ManifestData
	host, err := platformhost.New(platformhost.Config{Topology: topology, PluginID: PluginID, Group: data.Group, Version: data.PreferredVersion, RouteDeadline: options.routeDeadline})
	require.NoError(t, err)
	keys, err := host.JWKS()
	require.NoError(t, err)
	anthropic := newFakeAnthropic(t)
	env := map[string]string{
		"GF_PLUGIN_CONFIG_FILE": writeModelConfig(t, anthropic.server.URL, options.fallback),
		"GRAFANA_JWKS":          string(keys),
		"ANTHROPIC_SECRET":      providerSecret,
	}
	if topology == platformhost.TopologyCloud {
		env["GF_PLUGIN_ACCESS_MODE"] = "cloud"
		env["GF_PLUGIN_ACCESS_INTERNAL_SERVICES"] = testInternalService
	} else {
		env["GF_PLUGIN_ACCESS_MODE"] = "self-hosted"
	}
	logs := &syncBuffer{}
	registry := prometheus.NewRegistry()
	gw, err := New(Options{LookupEnv: lookup(env), Logger: slog.New(slog.NewJSONHandler(logs, nil)), Registerer: registry})
	require.NoError(t, err)
	provider := gw.Provider()
	built, err := provider.NewApp(app.Config{ManifestData: *provider.Manifest().ManifestData})
	require.NoError(t, err)
	served, err := host.ServeInProcess(grpcplugin.ServeOpts{RouteServer: appadapter.NewRouteAdapter(built), Authenticator: gw.Authenticator(), PluginID: PluginID})
	require.NoError(t, err)
	server := httptest.NewServer(served.Handler())
	t.Cleanup(func() {
		server.Close()
		served.Close()
		gw.Close()
	})
	return &harness{host: host, served: served, server: server, gateway: gw, registry: registry, logs: logs, anthropic: anthropic}
}

func (h *harness) url(route string) string {
	return h.server.URL + strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + "/" + route
}

func (h *harness) serviceToken(t *testing.T, service string) string {
	t.Helper()
	token, err := h.host.ServiceToken(service, testNamespace)
	require.NoError(t, err)
	return token
}

func (h *harness) onBehalfOf(t *testing.T, service string, identityType types.IdentityType, uid string) string {
	t.Helper()
	user, err := h.host.IDToken(identityType, uid, testNamespace)
	require.NoError(t, err)
	token, err := h.host.Exchange(context.Background(), service, delegation(user))
	require.NoError(t, err)
	return token
}

func (h *harness) metrics(t *testing.T) string {
	t.Helper()
	response := httptest.NewRecorder()
	promhttp.HandlerFor(h.registry, promhttp.HandlerOpts{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return response.Body.String()
}

type credential func(*http.Request)

func accessToken(token string) credential {
	return func(r *http.Request) { r.Header.Set("X-Access-Token", token) }
}

func bearer(token string) credential {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func session(value string) credential {
	return func(r *http.Request) { r.AddCookie(platformhost.SessionCookie(value)) }
}

func discoveryRequest(t *testing.T, url string, auth credential) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	auth(request)
	return request
}

func languageRequest(t *testing.T, ctx context.Context, url, body string, streaming bool, auth credential) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "public")
	request.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
	auth(request)
	return request
}

func do(t *testing.T, request *http.Request) (int, string) {
	t.Helper()
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(body)
}

func delegation(subject string) authn.TokenExchangeRequest {
	return authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{PluginID}, SubjectToken: subject}
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
	t.Run("access decisions", func(t *testing.T) {
		metrics := h.metrics(t)
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
	head := discoveryRequest(t, h.url("config"), internal)
	head.Method = http.MethodHead
	status, _ = do(t, head)
	assert.Equal(t, http.StatusMethodNotAllowed, status)
	status, _ = do(t, discoveryRequest(t, h.url("models"), internal))
	assert.Equal(t, http.StatusNotFound, status)

	for _, path := range []string{"/api/v1/aisdk/config", "/api/v1/aisdk/language-model", "/live", "/ready", "/metrics"} {
		response := httptest.NewRecorder()
		h.gateway.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusNotFound, response.Code, path)
	}
	assert.Equal(t, before, h.anthropic.requests.Load())
	assert.Contains(t, h.logs.String(), `"route":"unmatched"`)
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
	assert.NotContains(t, h.logs.String(), `"route":"language_model"`)

	cancel()
	select {
	case <-h.anthropic.canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("caller cancellation did not cancel the provider call")
	}
	close(release)
	require.Eventually(t, func() bool {
		return strings.Contains(h.logs.String(), `"route":"language_model"`)
	}, 3*time.Second, 10*time.Millisecond)
}

func TestGateway_AcceptsRequestsAboveTheFormerBodyBound(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	internal := accessToken(h.serviceToken(t, testInternalService))
	data := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 384<<10))
	body := `{"prompt":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"file","mediaType":"image/png","data":{"type":"data","data":"` + data + `"}}]}],"maxOutputTokens":16}`
	require.Greater(t, len(body), 2<<20)

	status, response := do(t, languageRequest(t, context.Background(), h.url("language-model"), body, false, internal))
	assert.Equal(t, http.StatusOK, status, response)
	assert.Contains(t, response, "hello")
	require.Equal(t, int32(1), h.anthropic.requests.Load())
	assert.Contains(t, h.anthropic.recorded()[0], data)
}

func TestGateway_RouteDeadlineEndsTheModelCall(t *testing.T) {
	const routeDeadline = 200 * time.Millisecond
	for _, streaming := range []bool{false, true} {
		t.Run("deadline/streaming="+strconv.FormatBool(streaming), func(t *testing.T) {
			h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{routeDeadline: routeDeadline, fallback: true})
			internal := accessToken(h.serviceToken(t, testInternalService))
			h.anthropic.block.Store(true)
			start := time.Now()
			_, _ = do(t, languageRequest(t, context.Background(), h.url("language-model"), unaryBody, streaming, internal))
			select {
			case err := <-h.anthropic.blocked:
				require.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("route deadline did not cancel the native request")
			}
			assert.GreaterOrEqual(t, time.Since(start), routeDeadline)
			assert.Less(t, time.Since(start), 5*time.Second)
			time.Sleep(100 * time.Millisecond)
			assert.Equal(t, int32(1), h.anthropic.requests.Load())
		})
		t.Run("caller cancellation without a deadline/streaming="+strconv.FormatBool(streaming), func(t *testing.T) {
			h := newHarness(t, platformhost.TopologyCloud)
			internal := accessToken(h.serviceToken(t, testInternalService))
			h.anthropic.block.Store(true)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				response, err := http.DefaultClient.Do(languageRequest(t, ctx, h.url("language-model"), unaryBody, streaming, internal))
				if err == nil {
					_ = response.Body.Close()
				}
			}()
			require.Eventually(t, func() bool { return h.anthropic.requests.Load() == 1 }, 3*time.Second, 10*time.Millisecond)
			cancel()
			select {
			case err := <-h.anthropic.blocked:
				require.Error(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("caller cancellation did not cancel the native request")
			}
			<-done
		})
	}
}

const unaryBody = `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"maxOutputTokens":16}`
