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
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"content":[{"type":"text","text":"hello from Go"}],
		"finishReason":{"unified":"stop","raw":"test-stop"},
		"usage":{"inputTokens":{"total":2,"noCache":1,"cacheRead":1,"cacheWrite":0},"outputTokens":{"total":1,"text":1,"reasoning":0}}
	}`, response.Body.String())
	assert.Equal(t, int64(1), scenario.stats.successCalls.Load())
	assert.Equal(t, []provider.Message{provider.NewSystemMessage("hello")}, scenario.stats.options().Prompt)
	assert.Nil(t, scenario.stats.options().Headers)
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
