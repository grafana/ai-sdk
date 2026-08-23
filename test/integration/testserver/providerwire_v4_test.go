package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func providerWireV4Request(t *testing.T, ctx context.Context, modelID, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, providerWireV4Prefix+providerwirev4.LanguageModelPath, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerwirev4.HeaderSpecificationVersion, providerwirev4.SpecificationVersion)
	request.Header.Set(providerwirev4.HeaderModelID, modelID)
	request.Header.Set(providerwirev4.HeaderStreaming, "false")
	return request
}

func TestProviderWireV4Scenario_SuccessUsesProductionRoute(t *testing.T) {
	scenario, err := newProviderWireV4Scenario()
	require.NoError(t, err)
	mux := http.NewServeMux()
	scenario.register(mux)

	response := httptest.NewRecorder()
	request := providerWireV4Request(t, context.Background(), "success", `{"prompt":[{"role":"system","content":"hello"}]}`)
	request.Header.Set(providerWireV4ControlHeader, "none")
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"content":[{"type":"text","text":"hello from Go"}],
		"finishReason":{"unified":"stop","raw":"test-stop"},
		"usage":{"inputTokens":{"total":2,"noCache":1,"cacheRead":1,"cacheWrite":0},"outputTokens":{"total":1,"text":1,"reasoning":0}},
		"warnings":[{"type":"other","message":"the model reported a warning"}],
		"response":{"id":"response-1","modelId":"success","timestamp":"2026-08-22T00:00:00Z"}
	}`, response.Body.String())
	assert.Equal(t, int64(1), scenario.stats.successCalls.Load())
	assert.Equal(t, []provider.Message{provider.NewSystemMessage("hello")}, scenario.stats.options().Prompt)
	assert.Nil(t, scenario.stats.options().Headers)
}

func TestProviderWireV4Scenario_StreamingUsesProductionRoute(t *testing.T) {
	scenario, err := newProviderWireV4Scenario()
	require.NoError(t, err)
	mux := http.NewServeMux()
	scenario.register(mux)

	response := httptest.NewRecorder()
	request := providerWireV4Request(t, context.Background(), "success", `{"prompt":[{"role":"user","content":[{"type":"text","text":"stream"}]}]}`)
	request.Header.Set(providerwirev4.HeaderStreaming, "true")
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), `data: {"type":"stream-start","warnings":[{"type":"other","message":"the model reported a warning"}]}`)
	assert.Contains(t, response.Body.String(), `data: {"type":"text-delta","id":"text-1","delta":""}`)
	assert.Contains(t, response.Body.String(), `data: {"type":"text-delta","id":"text-1","delta":"hello from Go stream"}`)
	assert.True(t, strings.HasSuffix(response.Body.String(), "\n\n"))
	assert.NotContains(t, response.Body.String(), "[DONE]")
	assert.Equal(t, int64(1), scenario.stats.streamCalls.Load())
	assert.Equal(t, []provider.Message{provider.UserText("stream")}, scenario.stats.options().Prompt)
}

func TestProviderWireV4Scenario_SafeCategories(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		control string
		body    string
		status  int
	}{
		{name: "invalid request", modelID: "success", body: `{"prompt":[],"headers":{"x-call":"invalid"}}`, status: http.StatusBadRequest},
		{name: "authentication", modelID: "success", control: "authentication", body: `{"prompt":[]}`, status: http.StatusUnauthorized},
		{name: "permission", modelID: "success", control: "permission", body: `{"prompt":[]}`, status: http.StatusForbidden},
		{name: "model not found", modelID: "missing", body: `{"prompt":[]}`, status: http.StatusNotFound},
		{name: "rate limit", modelID: "success", control: "rate-limit", body: `{"prompt":[]}`, status: http.StatusTooManyRequests},
		{name: "overload", modelID: "success", control: "overload", body: `{"prompt":[]}`, status: http.StatusServiceUnavailable},
		{name: "failed dependency", modelID: "failed-dependency", body: `{"prompt":[]}`, status: http.StatusFailedDependency},
		{name: "upstream", modelID: "upstream", body: `{"prompt":[]}`, status: http.StatusBadGateway},
		{name: "timeout", modelID: "timeout", body: `{"prompt":[]}`, status: http.StatusGatewayTimeout},
		{name: "cancellation", modelID: "cancellation", body: `{"prompt":[]}`, status: 499},
		{name: "internal", modelID: "internal", body: `{"prompt":[]}`, status: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scenario, err := newProviderWireV4Scenario()
			require.NoError(t, err)
			mux := http.NewServeMux()
			scenario.register(mux)
			request := providerWireV4Request(t, context.Background(), tc.modelID, tc.body)
			if tc.control != "" {
				request.Header.Set(providerWireV4ControlHeader, tc.control)
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		})
	}
}

func TestProviderWireV4Scenario_ObservesClientCancellation(t *testing.T) {
	scenario, err := newProviderWireV4Scenario()
	require.NoError(t, err)
	mux := http.NewServeMux()
	scenario.register(mux)
	ctx, cancel := context.WithCancel(context.Background())
	request := providerWireV4Request(t, ctx, "blocking", `{"prompt":[]}`)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		mux.ServeHTTP(response, request)
	}()

	require.Eventually(t, func() bool { return scenario.stats.blockingCalls.Load() == 1 }, time.Second, time.Millisecond)
	cancel()
	require.Eventually(t, func() bool { return scenario.stats.cancellations.Load() == 1 }, time.Second, time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}
