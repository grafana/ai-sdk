package service

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhysicalAttempts_SaturationDoesNotSuppressConsumerEvidence(t *testing.T) {
	writer, reader := net.Pipe()
	defer func() { _ = reader.Close() }()
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(writer, telemetry, 1, time.Second, 20*time.Millisecond)
	defer sink.Close()
	for range 10000 {
		sink.enqueue(physicalAttemptRecord{})
	}
	require.Contains(t, testMetrics(t, telemetry), `class="queue_full"`)
	created, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ string, id string, _ ...anthropicprovider.Option) provider.LanguageModel {
		return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			if id == "backend-primary" {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "source failure"})
			}
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}}
	}, identityModelFactory, sink)
	require.NoError(t, err)
	handler, err := providerv4.New(providerv4.Config{Resolver: created, Limits: serviceTestLimits()})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "alias")
	request.Header.Set(providerv4.HeaderStreaming, "false")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, 200, response.Code)
	assert.Contains(t, response.Body.String(), `"outcome":"selected"`)
	assert.Contains(t, response.Body.String(), `"message":"source failure"`)
	assert.Contains(t, response.Body.String(), `"outcome":"failed"`)
}
