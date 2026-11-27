package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	"github.com/grafana/ai-sdk/schema"
)

const (
	routeCatalogKey     = "sentinel-route-catalog-key"
	editedCatalogKey    = "sentinel-edited-catalog-key"
	streamedCatalogBody = `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"maxOutputTokens":16}`
)

// teamCatalog is the document of catalog `team`, whose model `team-model`
// (alias `team`) uses backend model primary on an Anthropic provider.
func teamCatalog(t *testing.T, primary string, secure map[string]any) string {
	t.Helper()
	object := map[string]any{
		"metadata": map[string]any{"name": testCatalogName},
		"spec": map[string]any{
			"providers": map[string]any{"claude": map[string]any{"type": "anthropic"}},
			"models":    map[string]any{"team-model": map[string]any{"name": "Team model", "aliases": []string{"team"}, "primary": map[string]any{"provider": "claude", "model": primary}}},
		},
	}
	if secure != nil {
		object["secure"] = secure
	}
	body, err := json.Marshal(object)
	require.NoError(t, err)
	return string(body)
}

func (h *harness) catalogURL(route string) string {
	return h.url("modelcatalogs/" + testCatalogName + "/" + route)
}

func (h *harness) createTeamCatalog(t *testing.T) {
	t.Helper()
	code, body := sendObject(t, context.Background(), http.MethodPost, catalogURL(h.server.URL, ""), h.serviceToken(t, testInternalService), teamCatalog(t, "claude-sonnet", map[string]any{"claude": map[string]any{"create": routeCatalogKey}}))
	require.Equal(t, http.StatusCreated, code, body)
}

func catalogLanguageRequest(t *testing.T, url, modelID, body string, streaming bool, auth credential) *http.Request {
	t.Helper()
	request := languageRequest(t, context.Background(), url, body, streaming, auth)
	request.Header.Set(providerv4.HeaderModelID, modelID)
	return request
}

// nativeCalls returns the API key and body of each request fake received.
func (f *fakeAnthropic) nativeCalls() (keys, bodies []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, headers := range f.headers {
		keys = append(keys, headers.Get("X-Api-Key"))
		bodies = append(bodies, f.bodies[i])
	}
	return keys, bodies
}

func TestCatalogRoutes_InvokeAndDiscover(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	native := newFakeAnthropic(t)
	useNativeAnthropic(t, native)
	h.createTeamCatalog(t)
	user := session(h.host.AddSession(types.TypeUser, "private-user-uid", testNamespace))

	t.Run("discovery lists only the catalog", func(t *testing.T) {
		status, body := do(t, discoveryRequest(t, h.catalogURL("config"), user))
		require.Equal(t, http.StatusOK, status, body)
		assert.JSONEq(t, `{"models":[{"id":"team-model","name":"Team model","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"team-model"},"gateway":{"aliases":["team"],"primary":{"providerInstance":"claude","provider":"anthropic","providerModelId":"claude-sonnet"},"fallbacks":[]}}]}`, body)
		assert.NotContains(t, body, routeCatalogKey)
	})
	t.Run("unary and streamed calls use the catalog key", func(t *testing.T) {
		status, body := do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "team", unaryBody, false, user))
		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, "hello")
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "team-model", streamedCatalogBody, true, user))
		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, "first")
		assert.Contains(t, body, `"type":"finish"`)
		keys, bodies := native.nativeCalls()
		assert.Equal(t, []string{routeCatalogKey, routeCatalogKey}, keys)
		for _, body := range bodies {
			assert.Contains(t, body, `"model":"claude-sonnet"`)
		}
		assert.Zero(t, h.anthropic.requests.Load(), "the operator catalog's provider is not used")
	})
	t.Run("an edit applies on the next call", func(t *testing.T) {
		code, body := sendObject(t, context.Background(), http.MethodPut, catalogURL(h.server.URL, testCatalogName), h.serviceToken(t, testInternalService), teamCatalog(t, "claude-opus", map[string]any{"claude": map[string]any{"create": editedCatalogKey}}))
		require.Equal(t, http.StatusOK, code, body)
		status, body := do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "team", unaryBody, false, user))
		require.Equal(t, http.StatusOK, status, body)
		keys, bodies := native.nativeCalls()
		assert.Equal(t, editedCatalogKey, keys[len(keys)-1])
		assert.Contains(t, bodies[len(bodies)-1], `"model":"claude-opus"`)
	})
	t.Run("refusals before any provider call", func(t *testing.T) {
		before := native.requests.Load()
		status, body := do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "public", unaryBody, false, user))
		assert.Equal(t, http.StatusNotFound, status, "an operator-catalog model is not in the stack catalog")
		assert.Contains(t, body, `"code":"model_not_found"`)
		byok := `{"prompt":[],"providerOptions":{"gateway":{"byok":{"anthropic":[{"apiKey":"dummy-request-key"}]}}}}`
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "team", byok, false, user))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, body, "unsupported gateway control")
		status, _ = do(t, catalogLanguageRequest(t, h.url("modelcatalogs/missing/language-model"), "team", unaryBody, false, user))
		assert.Equal(t, http.StatusNotFound, status, "the platform answers for a missing catalog")
		assert.Equal(t, before, native.requests.Load())
	})
	t.Run("namespace routes are unchanged", func(t *testing.T) {
		status, body := do(t, discoveryRequest(t, h.url("config"), user))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, body, "catalog discovery is unsupported for BYOK")
		status, body = do(t, discoveryRequest(t, h.url("config"), accessToken(h.serviceToken(t, testInternalService))))
		require.Equal(t, http.StatusOK, status)
		assert.Contains(t, body, `"id":"public"`)
		assert.NotContains(t, body, "team-model")
	})
	t.Run("telemetry", func(t *testing.T) {
		metrics := h.metrics(t)
		assert.Contains(t, metrics, `grafana_ai_gateway_access_decisions_total{decision="catalog"}`)
		assert.Contains(t, metrics, `model="catalog"`)
		for _, private := range []string{routeCatalogKey, editedCatalogKey, testCatalogName + `"`, "team-model", "private-user-uid"} {
			assert.NotContains(t, metrics, private)
		}
		logs := h.logs.String()
		assert.Contains(t, logs, `"route":"catalog_config"`)
		assert.Contains(t, logs, `"route":"catalog_language_model"`)
		assert.Contains(t, logs, `"catalog":"team"`)
		for _, private := range []string{routeCatalogKey, editedCatalogKey} {
			assert.NotContains(t, logs, private)
		}
		assert.Contains(t, logs, `"caller_identity":"user:private-user-uid"`)
	})
}

func TestCatalogRoutes_UnlistedModels(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	native := newFakeAnthropic(t)
	var probes atomic.Int32
	instance := strings.Repeat("z", config.MaxPublicIDBytes-1-config.MaxListedModelIDBytes)
	longBackendID := strings.Repeat("a", config.MaxListedModelIDBytes)
	const versioned = "claude-sonnet-4@20250514"
	listing := []string{"org/new", "org/blocked", routeCatalogKey, versioned, "model+v2", longBackendID, longBackendID + "a", "/leading-slash", "has space", "modèle"}
	var inventory struct {
		Data    []map[string]string `json:"data"`
		HasMore bool                `json:"has_more"`
	}
	for _, id := range listing {
		inventory.Data = append(inventory.Data, map[string]string{"id": id})
	}
	payload, err := json.Marshal(inventory)
	require.NoError(t, err)
	useNativeAnthropicHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			probes.Add(1)
			assert.Contains(t, []string{routeCatalogKey, editedCatalogKey}, r.Header.Get("X-Api-Key"))
			_, _ = w.Write(payload)
			return
		}
		native.server.Config.Handler.ServeHTTP(w, r)
	}))
	writer := h.serviceToken(t, testInternalService)
	user := session(h.host.AddSession(types.TypeUser, "catalog-user", testNamespace))
	status, body := sendObject(t, t.Context(), http.MethodPost, catalogURL(h.server.URL, ""), writer,
		`{"metadata":{"name":"team"},"spec":{"providers":{"claude":{"type":"anthropic","serveAllModels":true,"modelBlocklist":["org/blocked"]},"`+instance+`":{"type":"anthropic","serveAllModels":true}},"models":{}},"secure":{"claude":{"create":"`+routeCatalogKey+`"},"`+instance+`":{"create":"`+editedCatalogKey+`"}}}`)
	require.Equal(t, http.StatusCreated, status, body)
	response, err := http.DefaultClient.Do(discoveryRequest(t, h.catalogURL("config"), user))
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	document, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	ids := validDiscoveryIDs(t, document)
	assert.Equal(t, []string{
		"claude/" + longBackendID, "claude/" + versioned, "claude/model+v2", "claude/org/new",
		instance + "/" + longBackendID, instance + "/" + versioned, instance + "/model+v2", instance + "/org/blocked", instance + "/org/new",
	}, ids)
	assert.Len(t, instance+"/"+longBackendID, config.MaxPublicIDBytes)
	assert.EqualValues(t, 2, probes.Load())
	for _, tc := range []struct{ id, body string }{
		{"claude/org/blocked", unaryBody},
		{"claude/org/new", `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"providerOptions":{"gateway":{"models":["claude/org/blocked"]}}}`},
	} {
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), tc.id, tc.body, false, user))
		assert.Equal(t, http.StatusNotFound, status, body)
		assert.Contains(t, body, `"code":"model_not_found"`)
	}
	assert.Zero(t, native.requests.Load())
	for _, streaming := range []bool{false, true} {
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "claude/org/new", streamedCatalogBody, streaming, user))
		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, `"canonicalModelId":"claude/org/new"`)
		assert.Contains(t, body, `"modelId":"org/new"`)
		assert.Contains(t, body, `"providerInstance":"claude"`)
	}
	status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), instance+"/org/blocked", unaryBody, false, user))
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"providerInstance":"`+instance+`"`)
	keys, bodies := native.nativeCalls()
	assert.Equal(t, []string{routeCatalogKey, routeCatalogKey, editedCatalogKey}, keys)
	require.Len(t, bodies, 3)
	assert.Contains(t, bodies[0], `"model":"org/new"`)
	assert.Contains(t, bodies[1], `"model":"org/new"`)
	assert.Contains(t, bodies[2], `"model":"org/blocked"`)
	assert.EqualValues(t, 2, probes.Load(), "inference never lists provider models")
	status, body = do(t, discoveryRequest(t, h.catalogURL("config"), user))
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"id":"claude/org/new"`)
	assert.EqualValues(t, 2, probes.Load(), "the platform-supplied revision enables cache hits")

	status, body = sendObject(t, t.Context(), http.MethodPut, catalogURL(h.server.URL, testCatalogName), writer,
		`{"metadata":{"name":"team"},"spec":{"providers":{"claude":{"type":"anthropic","serveAllModels":true,"modelBlocklist":["org/blocked"]},"`+instance+`":{"type":"anthropic","serveAllModels":true}},"models":{}},"secure":{"claude":{"create":"`+editedCatalogKey+`"}}}`)
	require.Equal(t, http.StatusOK, status, body)
	status, body = do(t, discoveryRequest(t, h.catalogURL("config"), user))
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"id":"claude/org/new"`)
	assert.EqualValues(t, 4, probes.Load(), "a catalog write invalidates every provider's cached listing")

	for _, id := range []string{versioned, "model+v2", longBackendID} {
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), instance+"/"+id, unaryBody, false, user))
		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, `"modelId":"`+id+`"`)
	}
	_, bodies = native.nativeCalls()
	require.Len(t, bodies, 6)
	assert.Contains(t, bodies[4], `"model":"model+v2"`)
	for _, id := range []string{longBackendID + "a", "/leading-slash", "has space", "modèle"} {
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), instance+"/"+id, unaryBody, false, user))
		assert.Equal(t, http.StatusNotFound, status, body)
	}
}

func validDiscoveryIDs(t *testing.T, document []byte) []string {
	t.Helper()
	schemaJSON, err := os.ReadFile("../../../providerwire/v4/schema/discovery.json")
	require.NoError(t, err)
	compiled, err := schema.CompileSchema(schemaJSON)
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(json.RawMessage(document)))

	var discovery struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(document, &discovery))
	ids := make([]string, len(discovery.Models))
	for i, row := range discovery.Models {
		ids[i] = row.ID
	}
	return ids
}

func TestCatalogRoutes_ProviderWithoutModels(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	var probes atomic.Int32
	var rejectKey atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		assert.Equal(t, "GET /v1/models", r.Method+" "+r.URL.Path)
		assert.Equal(t, routeCatalogKey, r.Header.Get("X-Api-Key"))
		if rejectKey.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("after_id") == "" {
			_, _ = io.WriteString(w, `{"data":[{"id":"ready"}],"has_more":true,"last_id":"ready"}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"next"}],"has_more":false}`)
	}))
	defer upstream.Close()
	useNativeAnthropicURL(t, upstream.URL)
	writer := h.serviceToken(t, testInternalService)
	user := session(h.host.AddSession(types.TypeUser, "catalog-user", testNamespace))
	postAction := func(route string) (int, string) {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.catalogURL(route), nil)
		require.NoError(t, err)
		user(request)
		return do(t, request)
	}
	assertNoModels := func() {
		status, body := do(t, discoveryRequest(t, h.catalogURL("config"), user))
		require.Equal(t, http.StatusOK, status, body)
		assert.JSONEq(t, `{"models":[]}`, body)
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "public", unaryBody, false, user))
		assert.Equal(t, http.StatusNotFound, status, body)
		assert.Contains(t, body, `"code":"model_not_found"`)
	}

	status, body := sendObject(t, t.Context(), http.MethodPost, catalogURL(h.server.URL, ""), writer,
		`{"metadata":{"name":"team"},"spec":{"providers":{},"models":{}}}`)
	assert.Equal(t, http.StatusBadRequest, status, body)
	assert.Contains(t, body, "at least one provider is required")

	status, body = sendObject(t, t.Context(), http.MethodPost, catalogURL(h.server.URL, ""), writer,
		`{"metadata":{"name":"team"},"spec":{"providers":{"claude":{"type":"anthropic"}},"models":{}},"secure":{"claude":{"create":"`+routeCatalogKey+`"}}}`)
	require.Equal(t, http.StatusCreated, status, body)
	status, body = sendObject(t, t.Context(), http.MethodPut, catalogURL(h.server.URL, testCatalogName), writer,
		`{"metadata":{"name":"team"},"spec":{"providers":{},"models":{}},"secure":{"claude":{"remove":true}}}`)
	assert.Equal(t, http.StatusBadRequest, status, body)
	assert.Contains(t, body, "at least one provider is required")
	assertNoModels()
	status, body = postAction("available-models?provider=absent")
	assert.Equal(t, http.StatusNotFound, status, body)
	assert.JSONEq(t, `{"message":"provider not found in stored catalog"}`, body)
	assert.Zero(t, probes.Load())
	status, body = postAction("health")
	require.Equal(t, http.StatusOK, status, body)
	assert.JSONEq(t, `{"providers":{"claude":"ok"},"models":{}}`, body)
	assert.EqualValues(t, 1, probes.Load(), "health stops after the first page without targets")
	status, body = postAction("available-models?provider=claude")
	require.Equal(t, http.StatusOK, status, body)
	assert.JSONEq(t, `{"status":"ok","modelIds":["next","ready"]}`, body)
	assert.EqualValues(t, 3, probes.Load(), "inventory still traverses pages")
	rejectKey.Store(true)
	status, body = postAction("health")
	require.Equal(t, http.StatusOK, status, body)
	assert.JSONEq(t, `{"providers":{"claude":"invalidKey"},"models":{}}`, body)
	assert.EqualValues(t, 4, probes.Load())
	assert.Zero(t, h.anthropic.requests.Load())
}

func TestCatalogRoutes_Access(t *testing.T) {
	for _, topology := range []platformhost.Topology{platformhost.TopologyCloud, platformhost.TopologySelfHosted} {
		t.Run(string(topology), func(t *testing.T) {
			h := newHarness(t, topology)
			useNativeAnthropic(t, newFakeAnthropic(t))
			h.createTeamCatalog(t)
			allowed := map[string]credential{
				"user":            session(h.host.AddSession(types.TypeUser, "u1", testNamespace)),
				"service account": bearer(h.host.AddServiceAccount("sa1", testNamespace)),
			}
			if topology == platformhost.TopologyCloud {
				allowed["access policy alone"] = accessToken(h.serviceToken(t, "svc-b"))
				allowed["access policy for a user"] = accessToken(h.onBehalfOf(t, "svc-b", types.TypeUser, "u1"))
			}
			for name, auth := range allowed {
				t.Run(name, func(t *testing.T) {
					status, body := do(t, discoveryRequest(t, h.catalogURL("config"), auth))
					assert.Equal(t, http.StatusOK, status, body)
					status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "team", unaryBody, false, auth))
					assert.Equal(t, http.StatusOK, status, body)
				})
			}
			t.Run("anonymous", func(t *testing.T) {
				anonymous := session(h.host.AddSession(types.TypeAnonymous, "anon", testNamespace))
				status, document := do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "team", unaryBody, false, anonymous))
				assert.Equal(t, http.StatusForbidden, status)
				assert.Contains(t, document, `"code":"forbidden"`)
				status, _ = do(t, discoveryRequest(t, h.catalogURL("config"), anonymous))
				assert.Equal(t, http.StatusForbidden, status)
			})
		})
	}
}

func TestCatalogRoutes_AttributeTheDirectCaller(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	useNativeAnthropic(t, newFakeAnthropic(t))
	h.createTeamCatalog(t)
	status, body := do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "team", unaryBody, false, accessToken(h.onBehalfOf(t, "svc-b", types.TypeUser, "private-user-uid"))))
	require.Equal(t, http.StatusOK, status, body)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		assert.Contains(collect, h.logs.String(), `"caller_service":"svc-b"`)
	}, time.Second, time.Millisecond)
	assert.Contains(t, h.logs.String(), `"caller_identity":"user:private-user-uid"`)
	metrics := h.metrics(t)
	assert.False(t, strings.Contains(metrics, "svc-b"))
	assert.False(t, strings.Contains(metrics, "private-user-uid"))
}

func TestCatalogRoutes_Messages(t *testing.T) {
	for _, topology := range []platformhost.Topology{platformhost.TopologyCloud, platformhost.TopologySelfHosted} {
		t.Run(string(topology), func(t *testing.T) {
			h := newHarness(t, topology)
			native := newFakeAnthropic(t)
			useNativeAnthropic(t, native)
			h.createTeamCatalog(t)
			auth := bearer(h.host.AddServiceAccount("messages-client", testNamespace))
			for _, streaming := range []bool{false, true} {
				body := `{"model":"team","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
				if streaming {
					body = strings.TrimSuffix(body, "}") + `,"stream":true}`
				}
				status, response := do(t, chatRequest(t, h.catalogURL("v1/messages"), body, auth))
				require.Equal(t, http.StatusOK, status, response)
				assert.Contains(t, response, `"model":"team-model"`)
				if streaming {
					assert.Contains(t, response, "event: message_stop")
				} else {
					assert.Contains(t, response, `"stop_reason":"end_turn"`)
				}
			}
			keys, bodies := native.nativeCalls()
			assert.Equal(t, []string{routeCatalogKey, routeCatalogKey}, keys)
			for _, body := range bodies {
				assert.Contains(t, body, `"model":"claude-sonnet"`)
			}
			assert.Zero(t, h.anthropic.requests.Load())
			before := native.requests.Load()
			body := `{"model":"public","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
			status, response := do(t, chatRequest(t, h.catalogURL("v1/messages"), body, auth))
			assert.Equal(t, http.StatusNotFound, status, response)
			assert.Contains(t, response, `"type":"not_found_error"`)
			anonymous := session(h.host.AddSession(types.TypeAnonymous, "anon", testNamespace))
			status, response = do(t, chatRequest(t, h.catalogURL("v1/messages"), body, anonymous))
			assert.Equal(t, http.StatusForbidden, status, response)
			assert.Contains(t, response, `"type":"permission_error"`)
			status, _ = do(t, chatRequest(t, h.url("modelcatalogs/missing/v1/messages"), body, auth))
			assert.Equal(t, http.StatusNotFound, status)
			assert.Equal(t, before, native.requests.Load())
			assert.Contains(t, h.logs.String(), `"route":"catalog_anthropic_messages"`)
		})
	}
}
