package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeOptions_SharedHandlerIsolation(t *testing.T) {
	const count = 24
	captured := make(chan map[string]any, count)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "Bearer native-key", r.Header.Get("Authorization"))
		captured <- body
		writeNativeOptionsResponse(w, "openai-compatible", body["stream"] == true)
	}))
	defer server.Close()
	created, err := BuildCatalog(config.File{Models: map[string]config.Model{
		"public": {Name: "Public", Primary: config.Primary{Provider: "native", Model: "backend"}},
	}}, map[string]config.ResolvedProvider{
		"native": {Type: "openai-compatible", APIKey: "native-key", BaseURL: server.URL, ProviderName: "my-vllm.chat"},
	}, server.Client(), identityModelFactory)
	require.NoError(t, err)
	handler, err := providerv4.New(providerv4.Config{Resolver: created, Limits: serviceTestLimits()})
	require.NoError(t, err)
	var wg sync.WaitGroup
	for index := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := []byte(fmt.Sprintf(`{"providerOptions":{"myVllm":{"call_marker":%d},"irrelevant":{}},"prompt":[{"role":"user","content":[{"type":"text","text":"request-%d","providerOptions":{"openaiCompatible":{"scope_marker":%d}}}]}]}`, index, index, index))
			original := bytes.Clone(body)
			request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
			request.Header.Set(providerv4.HeaderModelID, "public")
			request.Header.Set(providerv4.HeaderStreaming, fmt.Sprint(index%2 == 0))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, original, body)
		}()
	}
	wg.Wait()
	require.Len(t, captured, count)
	seen := make(map[float64]bool, count)
	for range count {
		body := <-captured
		marker := body["call_marker"].(float64)
		assert.False(t, seen[marker])
		seen[marker] = true
		message := body["messages"].([]any)[0].(map[string]any)
		assert.Equal(t, marker, message["scope_marker"])
		assert.Equal(t, fmt.Sprintf("request-%.0f", marker), message["content"])
		assert.Equal(t, "backend", body["model"])
	}
}
