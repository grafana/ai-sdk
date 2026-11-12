package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

const (
	serverFallbackChain   = `{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":[{"model":"claude-b","max_tokens":2048,"thinking":{"type":"disabled"}}]}}}`
	serverFallbackDefault = `{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":"default"}}}`
	explicitFallbackBeta  = "server-side-fallback-2026-06-01"
	defaultFallbackBeta   = "server-side-fallback-2026-07-01"
)

type serverFallbackRequest struct {
	host string
	beta string
	body map[string]any
}

// serverFallbackBackend records Anthropic requests by host and answers them.
// A marker returns an anthropic fallback block from claude-a to that model.
type serverFallbackBackend struct {
	marker string
	mu     sync.Mutex
	seen   []serverFallbackRequest
}

func (b *serverFallbackBackend) client() *http.Client {
	return &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
		b.mu.Lock()
		b.seen = append(b.seen, serverFallbackRequest{host: r.URL.Host, beta: r.Header.Get("Anthropic-Beta"), body: body})
		b.mu.Unlock()
		streaming, _ := body["stream"].(bool)
		contentType, response := "application/json", anthropicFallbackResponse(b.marker, streaming)
		if streaming {
			contentType = "text/event-stream"
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
	})}
}

func (b *serverFallbackBackend) requests() []serverFallbackRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]serverFallbackRequest(nil), b.seen...)
}

// anthropicFallbackResponse returns a native Anthropic response, with a
// fallback block to marker when marker is nonempty.
func anthropicFallbackResponse(marker string, streaming bool) string {
	if !streaming {
		content := `[{"type":"text","text":"answer"}]`
		if marker != "" {
			content = `[{"type":"fallback","from":{"model":"claude-a"},"to":{"model":"` + marker + `"}},{"type":"text","text":"answer"}]`
		}
		return `{"id":"msg","type":"message","role":"assistant","model":"claude-a","content":` + content + `,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	}
	events := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-a\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"
	index := 0
	if marker != "" {
		events += "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"fallback\",\"from\":{\"model\":\"claude-a\"},\"to\":{\"model\":\"" + marker + "\"}}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"
		index = 1
	}
	events += fmt.Sprintf("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":%d,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":%d,\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n", index, index, index)
	return events + "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
}

// serveConfigured sends body to model through a wire handler for a configured
// caller and returns the response and its last execution overview.
func serveConfigured(t *testing.T, selector execution.RequestSelector, model, body string, streaming bool) (*httptest.ResponseRecorder, *execution.Overview) {
	t.Helper()
	handler, err := newWireHandler(execution.Config{Selector: selector, Limits: serviceTestLimits()})
	require.NoError(t, err)
	request := httptest.NewRequestWithContext(callerContext(t, gatewayauth.ConfiguredAccounts), http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, model)
	request.Header.Set(providerv4.HeaderStreaming, fmt.Sprint(streaming))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response, lastOverview(t, response)
}

// lastOverview returns the last execution overview a wire response carries.
func lastOverview(t *testing.T, response *httptest.ResponseRecorder) *execution.Overview {
	t.Helper()
	overviews := responseOverviews(t, response.Body.String())
	if len(overviews) == 0 {
		return nil
	}
	return &overviews[len(overviews)-1]
}

func serverFallbackCatalog(t *testing.T, client *http.Client, primary, backup *config.ServerFallbacks, withBackup bool) *Catalog {
	t.Helper()
	route := config.Model{Name: "Public", Primary: config.Primary{Provider: "primary", Model: "backend", ServerFallbacks: primary}}
	if withBackup {
		route.Fallback = []config.Primary{{Provider: "backup", Model: "backend", ServerFallbacks: backup}}
	}
	created, err := BuildCatalog(config.File{Models: map[string]config.Model{"public": route}}, map[string]config.ResolvedProvider{
		"primary": {Type: "anthropic", APIKey: "primary-key", BaseURL: "https://primary.anthropic.test"},
		"backup":  {Type: "anthropic", APIKey: "backup-key", BaseURL: "https://backup.anthropic.test"},
	}, client, identityModelFactory)
	require.NoError(t, err)
	return created
}

func TestNativeServerFallbacks_ConfiguredPolicy(t *testing.T) {
	chainPolicy := &config.ServerFallbacks{Models: []string{"claude-b"}}
	defaultPolicy := &config.ServerFallbacks{AllowDefault: true}
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name           string
			body           string
			primary        *config.ServerFallbacks
			backup         *config.ServerFallbacks
			withBackup     bool
			allowed        bool
			beta           string
			wantBodyOption any
		}{
			{name: "allowed chain", body: serverFallbackChain, primary: chainPolicy, allowed: true, beta: explicitFallbackBeta, wantBodyOption: []any{map[string]any{"model": "claude-b", "max_tokens": float64(2048), "thinking": map[string]any{"type": "disabled"}}}},
			{name: "allowed default", body: serverFallbackDefault, primary: defaultPolicy, allowed: true, beta: defaultFallbackBeta, wantBodyOption: "default"},
			{name: "allowed chain on a configured fallback entry", body: serverFallbackChain, backup: chainPolicy, withBackup: true, allowed: true, beta: explicitFallbackBeta, wantBodyOption: []any{map[string]any{"model": "claude-b", "max_tokens": float64(2048), "thinking": map[string]any{"type": "disabled"}}}},
			{name: "disallowed model", body: `{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":[{"model":"claude-z"}]}}}`, primary: chainPolicy},
			{name: "repeated entry", body: `{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":[{"model":"claude-b"},{"model":"claude-b"}]}}}`, primary: chainPolicy},
			{name: "default without allowDefault", body: serverFallbackDefault, primary: chainPolicy},
			{name: "chain without default-only policy", body: serverFallbackChain, primary: defaultPolicy},
			{name: "no policy on any candidate", body: serverFallbackDefault, withBackup: true},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				backend := &serverFallbackBackend{}
				created := serverFallbackCatalog(t, backend.client(), tc.primary, tc.backup, tc.withBackup)
				response, overview := serveConfigured(t, execution.CatalogSelector(created), "public", tc.body, streaming)
				requests := backend.requests()
				if !tc.allowed {
					assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
					assert.Empty(t, requests)
					assert.NotContains(t, response.Body.String(), "claude-z")
					return
				}
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Len(t, requests, 1)
				assert.Equal(t, tc.wantBodyOption, requests[0].body["fallbacks"])
				assert.Contains(t, requests[0].beta, tc.beta)
				require.NotNil(t, overview)
				if !tc.withBackup {
					assert.Equal(t, "primary.anthropic.test", requests[0].host)
					return
				}
				assert.Equal(t, "backup.anthropic.test", requests[0].host)
				require.Len(t, overview.Attempts, 2)
				assert.Equal(t, fallback.AttemptFailed, overview.Attempts[0].Outcome)
				assert.Equal(t, &execution.Failure{Message: "gateway: unsupported request"}, overview.Attempts[0].Error)
				assert.Equal(t, fallback.AttemptSelected, overview.Attempts[1].Outcome)
				assert.Equal(t, "backup", overview.Attempts[1].ProviderInstance)
			})
		}
	}

	t.Run("translation-added fallbacks are checked per candidate", func(t *testing.T) {
		addFallbacks := func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
			options.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"fallbacks":[{"model":"claude-b"}]}`)}}
			return options, nil
		}
		for _, tc := range []struct {
			name    string
			primary *config.ServerFallbacks
			hosts   []string
		}{
			{"primary allows", chainPolicy, []string{"primary.anthropic.test"}},
			{"only the fallback entry allows", nil, []string{"backup.anthropic.test"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				backend := &serverFallbackBackend{}
				created := serverFallbackCatalog(t, backend.client(), tc.primary, chainPolicy, true)
				err := runTranslation(t, execution.CatalogSelector(created), translationCall{modelID: "public", translator: addFallbacks})
				require.NoError(t, err)
				requests := backend.requests()
				require.Len(t, requests, len(tc.hosts))
				for i, host := range tc.hosts {
					assert.Equal(t, host, requests[i].host)
					assert.Equal(t, []any{map[string]any{"model": "claude-b"}}, requests[i].body["fallbacks"])
				}
			})
		}
	})
}

func TestNativeServerFallbacks_ServedModel(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured/stream=%t", streaming), func(t *testing.T) {
			backend := &serverFallbackBackend{marker: "claude-b"}
			created := serverFallbackCatalog(t, backend.client(), &config.ServerFallbacks{Models: []string{"claude-b"}}, nil, false)
			response, overview := serveConfigured(t, execution.CatalogSelector(created), "public", serverFallbackChain, streaming)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.NotNil(t, overview)
			require.Len(t, overview.Attempts, 1)
			assert.Equal(t, "backend", overview.Attempts[0].ModelID)
			assert.Equal(t, "claude-b", overview.Attempts[0].ServedModelID)
		})
		t.Run(fmt.Sprintf("configured without a hop/stream=%t", streaming), func(t *testing.T) {
			backend := &serverFallbackBackend{}
			created := serverFallbackCatalog(t, backend.client(), &config.ServerFallbacks{Models: []string{"claude-b"}}, nil, false)
			response, overview := serveConfigured(t, execution.CatalogSelector(created), "public", serverFallbackChain, streaming)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.NotNil(t, overview)
			assert.Empty(t, overview.Attempts[0].ServedModelID)
			assert.NotContains(t, response.Body.String(), "servedModelId")
		})
		t.Run(fmt.Sprintf("BYOK/stream=%t", streaming), func(t *testing.T) {
			backend := &serverFallbackBackend{marker: "claude-b"}
			body := `{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":"default"},"gateway":{"byok":{"anthropic":[{"apiKey":"dummy-request-key"}]}}}}`
			response := serveBYOK(t, backend.client(), "anthropic/native-model", body, streaming)
			overview := lastOverview(t, response)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			requests := backend.requests()
			require.Len(t, requests, 1)
			assert.Equal(t, "default", requests[0].body["fallbacks"])
			assert.Contains(t, requests[0].beta, defaultFallbackBeta)
			require.NotNil(t, overview)
			require.Len(t, overview.Attempts, 1)
			assert.Equal(t, "native-model", overview.Attempts[0].ModelID)
			assert.Equal(t, "claude-b", overview.Attempts[0].ServedModelID)
			assert.NotContains(t, response.Body.String(), "dummy-request-key")
		})
	}
}

func TestNativeServerFallbacks_BYOKIgnoresConfiguredPolicy(t *testing.T) {
	const byokDefault = `{"prompt":[],"providerOptions":{"gateway":{"byok":{"anthropic":[{"apiKey":"dummy-request-key"}]}},"anthropic":{"fallbacks":"default"}}}`
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			backend := &serverFallbackBackend{}
			client := backend.client()
			selector := NewSelector(serverFallbackCatalog(t, client, &config.ServerFallbacks{Models: []string{"claude-b"}}, nil, false), &BYOKOptions{Clients: outbound.ModelClients{Shared: client, Egress: client}, Factory: identityModelFactory}, nil)

			configured, _ := serveConfigured(t, selector, "public", serverFallbackDefault, streaming)
			require.Equal(t, http.StatusBadRequest, configured.Code, configured.Body.String())
			require.Empty(t, backend.requests())

			response := serveBYOKWith(t, selector, "anthropic/backend", byokDefault, streaming)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			requests := backend.requests()
			require.Len(t, requests, 1)
			assert.Equal(t, "default", requests[0].body["fallbacks"])
			assert.Contains(t, strings.Split(requests[0].beta, ","), defaultFallbackBeta)
		})
	}
}
