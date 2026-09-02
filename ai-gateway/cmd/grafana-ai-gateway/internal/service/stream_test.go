package service

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_PreservesProviderWireFlushWhileStreamIsOpen(t *testing.T) {
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
	router, err := NewRouter(RouterConfig{
		Readiness:     readiness,
		Telemetry:     telemetry,
		Authenticator: &serviceAuthenticator{info: serviceAuthInfo()},
		ErrorWriter:   errorWriter,
		Discovery:     http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		LanguageModel: language,
	})
	require.NoError(t, err)
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/aisdk/language-model", strings.NewReader(`{"prompt":[]}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Access-Token", "access")
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

	cancel()
	_ = response.Body.Close()
	select {
	case <-model.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("provider context was not canceled")
	}
	require.Eventually(t, func() bool {
		return strings.Count(logs.String(), "http request completed") == 1
	}, 2*time.Second, 10*time.Millisecond)
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
