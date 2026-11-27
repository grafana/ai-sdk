package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

func TestOperatorCatalog_QualifiedModels(t *testing.T) {
	native := newFakeAnthropic(t)
	var probes atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			probes.Add(1)
			assert.Equal(t, providerSecret, r.Header.Get("X-Api-Key"))
			_, _ = io.WriteString(w, `{"data":[{"id":"org/new"},{"id":"blocked"},{"id":"alias"},{"id":"listed"},{"id":"`+providerSecret+`"}],"has_more":false}`)
			return
		}
		native.handler().ServeHTTP(w, r)
	}))
	t.Cleanup(backend.Close)
	other := newFakeAnthropic(t)
	const otherKey = "dummy-other-key"
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{env: map[string]string{
		"OTHER_KEY": otherKey,
		"GF_PLUGIN_MODEL_CATALOG": fmt.Sprintf(`providers:
  team:
    type: anthropic
    apiKeyEnv: ANTHROPIC_SECRET
    baseURL: %s
    serveAllModels: true
    modelBlocklist: [blocked]
  other:
    type: anthropic
    apiKeyEnv: OTHER_KEY
    baseURL: %s
models:
  team/listed:
    name: Listed
    aliases: [team/alias]
    primary: {provider: other, model: listed-backend}
  with-grants:
    name: Route with grants
    primary:
      provider: team
      model: org/new
      serverFallbacks: {models: [allowed]}
    fallback: [{provider: other, model: backup}]
`, backend.URL, other.server.URL),
	}})
	internal := accessToken(h.serviceToken(t, testInternalService))
	byok := bearer(h.host.AddServiceAccount("request-account", testNamespace))
	status, body := do(t, discoveryRequest(t, h.url("config"), byok))
	require.Equal(t, http.StatusBadRequest, status, body)
	assert.Zero(t, probes.Load())

	const routed = `{"prompt":[],"providerOptions":{"gateway":{"only":["anthropic"]}}}`
	for _, tc := range []struct{ id, request string }{
		{"team/blocked", unaryBody},
		{"team/blocked", routed},
		{"other/unlisted", unaryBody},
		{"other/unlisted", routed},
		{"team/org/new", `{"prompt":[],"providerOptions":{"gateway":{"models":["team/blocked"]}}}`},
	} {
		status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), tc.id, tc.request, false, internal))
		assert.Equal(t, http.StatusNotFound, status, body)
		assert.Contains(t, body, `"code":"model_not_found"`)
	}
	for _, request := range []string{
		`{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":[{"model":"allowed"}]}}}`,
		`{"prompt":[],"providerOptions":{"anthropic":{"fallbacks":[{"model":"allowed"}]},"gateway":{"only":["anthropic"]}}}`,
	} {
		status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), "team/org/new", request, false, internal))
		assert.Equal(t, http.StatusBadRequest, status, body)
	}
	assert.Zero(t, native.requests.Load())
	assert.Zero(t, other.requests.Load(), "unlisted selectors do not inherit the listed fallback")

	for _, request := range []string{unaryBody, routed} {
		status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), "team/org/new", request, false, internal))
		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, `"canonicalModelId":"team/org/new"`)
		assert.Contains(t, body, `"modelId":"org/new"`)
		for _, id := range []string{"team/listed", "team/alias"} {
			status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), id, request, false, internal))
			require.Equal(t, http.StatusOK, status, body)
			assert.Contains(t, body, `"modelId":"listed-backend"`)
		}
	}
	keys, calls := native.nativeCalls()
	assert.Equal(t, []string{providerSecret, providerSecret}, keys)
	for _, call := range calls {
		assert.Contains(t, call, `"model":"org/new"`)
	}
	keys, _ = other.nativeCalls()
	assert.Equal(t, []string{otherKey, otherKey, otherKey, otherKey}, keys)
	assert.Zero(t, probes.Load(), "invocation did not need inventory")

	for range 2 {
		response, err := http.DefaultClient.Do(discoveryRequest(t, h.url("config"), internal))
		require.NoError(t, err)
		raw, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		require.NoError(t, err)
		body = string(raw)
		require.Equal(t, http.StatusOK, response.StatusCode, body)
		assert.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		assert.Contains(t, body, `"id":"team/listed"`)
		assert.Contains(t, body, `"aliases":["team/alias"]`)
		assert.Contains(t, body, `"id":"team/org/new"`)
		assert.Contains(t, body, `"modelId":"team/org/new"`)
		assert.NotContains(t, body, `"id":"team/alias"`)
		assert.NotContains(t, body, "team/blocked")
		assert.NotContains(t, body, providerSecret)
	}
	assert.EqualValues(t, 1, probes.Load())

	native.failModel.Store("org/new")
	status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), "team/org/new", unaryBody, false, internal))
	assert.Equal(t, http.StatusServiceUnavailable, status, body)
	assert.EqualValues(t, 4, other.requests.Load(), "a retryable error must not invoke the listed route's fallback")

	const explicit = `{"prompt":[],"providerOptions":{"gateway":{"models":["team/another"]}}}`
	status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), "team/org/new", explicit, false, internal))
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"modelId":"org/new"`)
	assert.Contains(t, body, `"modelId":"another"`)
	assert.EqualValues(t, 4, other.requests.Load())
	status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), "team/another", routed, false, internal))
	require.Equal(t, http.StatusOK, status, body)
	longID := "team/" + strings.Repeat("z", 124)
	priorLogs := h.logs.String()
	assert.Contains(t, priorLogs, `"ai_sdk.model":"team/another"`)
	before := unlistedRequestCount(t, h)
	status, body = do(t, catalogLanguageRequest(t, h.url("language-model"), longID, unaryBody, false, internal))
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"canonicalModelId":"`+longID+`"`)
	var records int
	for line := range strings.SplitSeq(strings.TrimSpace(strings.TrimPrefix(h.logs.String(), priorLogs)), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		switch record["ai_sdk.event"] {
		case "aisdk.model.generate.start", "aisdk.model.generate.finish":
			records++
			assert.NotContains(t, record, "ai_sdk.model")
			assert.NotContains(t, record, "gen_ai.request.model")
		}
	}
	assert.Equal(t, 2, records)
	assert.Equal(t, before+1, unlistedRequestCount(t, h))
	metrics := h.metrics(t)
	assert.Contains(t, metrics, `model="unlisted"`)
	assert.NotContains(t, metrics, `model="team/org/new"`)
	assert.NotContains(t, metrics, `model="team/another"`)
	assert.Contains(t, metrics, `model="team/listed"`)
	assert.NotContains(t, metrics, longID)
}

func unlistedRequestCount(t *testing.T, h *harness) float64 {
	t.Helper()
	families, err := h.registry.Gather()
	require.NoError(t, err)
	var total float64
	for _, family := range families {
		if family.GetName() != "aisdk_model_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "model" && label.GetValue() == "unlisted" {
					total += metric.GetCounter().GetValue()
				}
			}
		}
	}
	return total
}
