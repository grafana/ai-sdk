package gateway

import (
	"bytes"
	"context"
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

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/plugin/appadapter"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/manifestdata"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
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
	release  chan struct{}
	canceled chan struct{}
	mu       sync.Mutex
	bodies   []string
	headers  []http.Header
}

func newFakeAnthropic(t *testing.T) *fakeAnthropic {
	t.Helper()
	fake := &fakeAnthropic{canceled: make(chan struct{})}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.bodies = append(fake.bodies, string(body))
		fake.headers = append(fake.headers, r.Header.Clone())
		release := fake.release
		fake.mu.Unlock()
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

func writeModelConfig(t *testing.T, baseURL string) string {
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

func newHarness(t *testing.T, topology platformhost.Topology) *harness {
	t.Helper()
	data := manifestdata.LocalManifest().ManifestData
	host, err := platformhost.New(platformhost.Config{Topology: topology, PluginID: PluginID, Group: data.Group, Version: data.PreferredVersion})
	require.NoError(t, err)
	keys, err := host.JWKS()
	require.NoError(t, err)
	anthropic := newFakeAnthropic(t)
	env := map[string]string{
		"GRAFANA_AI_GATEWAY_CONFIG_FILE":     writeModelConfig(t, anthropic.server.URL),
		"GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE": "development",
		"GRAFANA_JWKS":                       string(keys),
		"ANTHROPIC_SECRET":                   providerSecret,
	}
	if topology == platformhost.TopologyCloud {
		env["GRAFANA_AI_GATEWAY_ACCESS_MODE"] = "cloud"
		env["GRAFANA_AI_GATEWAY_ACCESS_INTERNAL_SERVICES"] = testInternalService
	} else {
		env["GRAFANA_AI_GATEWAY_ACCESS_MODE"] = "self-hosted"
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
	token, err := h.host.Exchange(context.Background(), service, authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{PluginID}, SubjectToken: user})
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
