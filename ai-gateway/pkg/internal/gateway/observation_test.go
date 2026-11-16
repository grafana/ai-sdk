package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

func TestGateway_ObservesEachConfiguredCallOnceAfterSelection(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	internal := accessToken(h.serviceToken(t, testInternalService))
	routedBody := `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"maxOutputTokens":16,"providerOptions":{"gateway":{"order":["anthropic"]}}}`
	for _, body := range []string{unaryBody, routedBody} {
		status, response := do(t, languageRequest(t, context.Background(), h.url("language-model"), body, false, internal))
		require.Equal(t, http.StatusOK, status, response)
	}

	var starts []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(h.logs.String()), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["ai_sdk.event"] == "aisdk.model.generate.start" {
			starts = append(starts, record)
		}
	}
	require.Len(t, starts, 2)
	for _, start := range starts {
		assert.Equal(t, "grafana", start["ai_sdk.provider"])
		assert.Equal(t, "public", start["ai_sdk.model"])
	}
	metrics := h.metrics(t)
	assert.Contains(t, metrics, `model="public"`)
	assert.NotContains(t, metrics, "backend-private")
}
