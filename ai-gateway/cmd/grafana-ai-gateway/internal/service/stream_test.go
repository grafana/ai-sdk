package service

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_PreservesProviderWireFlushWhileStreamIsOpen(t *testing.T) {
	for _, tc := range []struct {
		name   string
		build  func(RouterDependencies) http.Handler
		source gatewayauth.Source
		split  bool
	}{
		{"combined", NewRouter, gatewayauth.SourceAccessToken, false},
		{"internal split", NewAPIRouter, gatewayauth.SourceAccessToken, true},
		{"Cloud API", NewAPIRouter, gatewayauth.SourceCloudGateway, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testRouterOpenStream(t, tc.build, tc.source, tc.split)
		})
	}
}

func testRouterOpenStream(t *testing.T, build func(RouterDependencies) http.Handler, source gatewayauth.Source, split bool) {
	t.Helper()
	model := &openStreamModel{canceled: make(chan struct{})}
	modelCatalog, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "public"}, Model: model}})
	require.NoError(t, err)
	language, err := providerv4.New(providerv4.Config{Resolver: modelCatalog, Limits: serviceTestLimits()})
	require.NoError(t, err)

	var logs synchronizedBuffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	readiness := &Readiness{}
	readiness.Set(true)
	errorWriter := providerv4.NewHostErrorWriter()
	authenticator, headers := serviceModeAuthentication(source)
	router := build(RouterDependencies{
		Readiness:     readiness,
		Telemetry:     telemetry,
		AuthSource:    source,
		Authenticator: authenticator,
		ErrorWriter:   errorWriter,
		Discovery:     http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		LanguageModel: language,
	})
	server := httptest.NewServer(router)
	defer server.Close()

	operational := server
	if split {
		operational = httptest.NewServer(NewOperationalRouter(RouterDependencies{Readiness: readiness, Telemetry: telemetry}))
		defer operational.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/aisdk/language-model", strings.NewReader(`{"prompt":[]}`))
	require.NoError(t, err)
	request.Header = headers.Clone()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "public")
	request.Header.Set(providerv4.HeaderStreaming, "true")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusOK, response.StatusCode)

	frame := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(response.Body)
		var lines strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				frame <- ""
				return
			}
			lines.WriteString(line)
			if line == "\n" {
				frame <- lines.String()
				return
			}
		}
	}()
	select {
	case first := <-frame:
		assert.Contains(t, first, "data:")
	case <-time.After(2 * time.Second):
		t.Fatal("initial ProviderWire frame was not flushed while stream remained open")
	}
	select {
	case <-model.canceled:
		t.Fatal("stream closed before client cancellation")
	default:
	}

	metricsResponse, err := operational.Client().Get(operational.URL + "/metrics")
	require.NoError(t, err)
	metrics, err := io.ReadAll(metricsResponse.Body)
	_ = metricsResponse.Body.Close()
	require.NoError(t, err)
	assert.Contains(t, string(metrics), `grafana_ai_gateway_http_requests_in_flight{method="POST",route="language_model"} 1`)
	assert.Contains(t, string(metrics), `grafana_ai_gateway_authentication_total{outcome="authenticated",source="`+string(source)+`"} 1`)

	cancel()
	_ = response.Body.Close()
	select {
	case <-model.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("provider context was not canceled")
	}
	require.Eventually(t, func() bool {
		return strings.Count(logs.String(), `"route":"language_model"`) == 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Contains(t, logs.String(), `"authentication_source":"`+string(source)+`"`)
	metricsAfter := telemetryMetrics(telemetry)
	assert.Contains(t, metricsAfter, `grafana_ai_gateway_http_requests_in_flight{method="POST",route="language_model"} 0`)
	assert.Contains(t, metricsAfter, `grafana_ai_gateway_http_requests_total{method="POST",route="language_model",status="2xx"} 1`)
	for _, private := range []string{"private-access-token", "private-policy", "123456789", "987654321", "backend-private"} {
		assert.NotContains(t, logs.String(), private)
		assert.NotContains(t, metricsAfter, private)
	}
}

func serviceTestLimits() providerv4.Limits {
	return providerv4.Limits{
		RequestBytes:        1 << 20,
		UnaryResponseBytes:  1 << 20,
		StreamParts:         1000,
		StreamFrameBytes:    1 << 20,
		ModelDuration:       5 * time.Second,
		StreamIdleDuration:  5 * time.Second,
		StreamDrainDuration: 100 * time.Millisecond,
	}
}

type openStreamModel struct {
	canceled chan struct{}
}

func (model *openStreamModel) SpecificationVersion() string               { return "v4" }
func (model *openStreamModel) Provider() string                           { return "test" }
func (model *openStreamModel) ModelID() string                            { return "backend-private" }
func (model *openStreamModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (model *openStreamModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, assert.AnError
}
func (model *openStreamModel) DoStream(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	parts := make(chan provider.StreamPart)
	go func() {
		defer close(parts)
		parts <- provider.StreamPart{Type: provider.PartStreamStart}
		<-ctx.Done()
		close(model.canceled)
	}()
	return &provider.StreamResult{Stream: parts}, nil
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *synchronizedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(value)
}

func (buffer *synchronizedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}
