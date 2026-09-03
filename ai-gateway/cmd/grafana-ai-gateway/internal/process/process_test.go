package process

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_ValidatesScalarsAndEndpointsBeforeSecretsOrListener(t *testing.T) {
	t.Run("scalar failure", func(t *testing.T) {
		secretCalls := 0
		listenCalls := 0
		err := Run(context.Background(), Dependencies{
			Args: []string{"--server.write-timeout=1s"},
			LookupEnv: func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
				}
				values := map[string]string{
					"GRAFANA_AI_GATEWAY_CONFIG_FILE":   "/nonexistent/private.yaml",
					"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL": "https://auth.example/jwks",
				}
				value, ok := values[name]
				return value, ok
			},
			Listen: func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			Logger: testLogger(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write timeout")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})

	t.Run("invalid production listen address", func(t *testing.T) {
		secretCalls := 0
		listenCalls := 0
		err := Run(context.Background(), Dependencies{
			Args: []string{"--server.listen-address=not-a-tcp-address"},
			LookupEnv: func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
				}
				values := map[string]string{
					"GRAFANA_AI_GATEWAY_CONFIG_FILE":   "/nonexistent/private.yaml",
					"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL": "https://auth.example/jwks",
				}
				value, ok := values[name]
				return value, ok
			},
			Listen: func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			Logger: testLogger(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TCP host:port")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})

	t.Run("invalid production jwks before yaml", func(t *testing.T) {
		secretCalls := 0
		listenCalls := 0
		err := Run(context.Background(), Dependencies{
			LookupEnv: func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
				}
				values := map[string]string{
					"GRAFANA_AI_GATEWAY_CONFIG_FILE":   "/nonexistent/private.yaml",
					"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL": "http://auth.example/jwks",
				}
				value, ok := values[name]
				return value, ok
			},
			Listen: func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			Logger: testLogger(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "production endpoint must use https")
		assert.NotContains(t, err.Error(), "nonexistent/private.yaml")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})

	t.Run("credential-bearing endpoint failure", func(t *testing.T) {
		path := writeProcessConfig(t, "https://user:password@provider.example")
		secretCalls := 0
		listenCalls := 0
		err := Run(context.Background(), Dependencies{
			Args: []string{"--deployment.mode=development", "--auth.unsafe", "--server.listen-address=127.0.0.1:0"},
			LookupEnv: func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
					return "secret-value", true
				}
				if name == "GRAFANA_AI_GATEWAY_CONFIG_FILE" {
					return path, true
				}
				return "", false
			},
			Listen: func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			Logger: testLogger(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "userinfo")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})
}

func TestRun_LocalReadinessDoesNotProbeProvider(t *testing.T) {
	var providerCalls atomic.Int64
	providerServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		providerCalls.Add(1)
	}))
	defer providerServer.Close()
	path := writeProcessConfig(t, providerServer.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addresses := make(chan string, 1)
	result := make(chan error, 1)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	go func() {
		result <- Run(ctx, Dependencies{
			Args: []string{"--deployment.mode=development", "--auth.unsafe", "--server.listen-address=127.0.0.1:0"},
			LookupEnv: func(name string) (string, bool) {
				switch name {
				case "GRAFANA_AI_GATEWAY_CONFIG_FILE":
					return path, true
				case "ANTHROPIC_SECRET":
					return "secret-value", true
				default:
					return "", false
				}
			},
			Listen: func(network, address string) (net.Listener, error) {
				listener, err := net.Listen(network, address)
				if err == nil {
					addresses <- listener.Addr().String()
				}
				return listener, err
			},
			Logger: logger,
		})
	}()
	address := <-addresses
	require.Eventually(t, func() bool {
		response, err := http.Get("http://" + address + "/ready")
		if err != nil {
			return false
		}
		defer func() { _ = response.Body.Close() }()
		return response.StatusCode == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond)
	assert.Zero(t, providerCalls.Load())
	cancel()
	require.NoError(t, <-result)
	assert.Equal(t, []string{processEventStarting, processEventReady, processEventShutdownStarted, processEventShutdownCompleted}, processLifecycleEvents(t, logs.String()))
	for _, private := range []string{"secret-value", providerServer.URL, "backend-private", "ANTHROPIC_SECRET"} {
		assert.NotContains(t, logs.String(), private)
	}
}

func TestServe_CancelFirstGracefulAndForcedShutdown(t *testing.T) {
	t.Run("cancellation-aware handler completes gracefully", func(t *testing.T) {
		signalContext, signalCancel := context.WithCancel(context.Background())
		requestContext, cancelRequests := context.WithCancel(context.Background())
		readiness := &service.Readiness{}
		telemetry, err := service.NewTelemetry(testLogger())
		require.NoError(t, err)
		started := make(chan struct{})
		readinessWasFalse := make(chan bool, 1)
		server := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				close(started)
				<-request.Context().Done()
				readinessWasFalse <- !readiness.Ready()
				w.WriteHeader(http.StatusNoContent)
			}),
			BaseContext: func(net.Listener) context.Context { return requestContext },
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		result := make(chan error, 1)
		go func() {
			result <- Serve(signalContext, cancelRequests, server, listener, readiness, telemetry, testLogger(), time.Second)
		}()
		go func() { _, _ = http.Get("http://" + listener.Addr().String()) }()
		<-started
		signalCancel()
		require.NoError(t, <-result)
		assert.True(t, <-readinessWasFalse)
		assert.False(t, readiness.Ready())
	})

	t.Run("cancellation-ignoring handler is force closed after deadline", func(t *testing.T) {
		signalContext, signalCancel := context.WithCancel(context.Background())
		requestContext, cancelRequests := context.WithCancel(context.Background())
		readiness := &service.Readiness{}
		telemetry, err := service.NewTelemetry(testLogger())
		require.NoError(t, err)
		started := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		server := &http.Server{
			Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				close(started)
				<-release
			}),
			BaseContext: func(net.Listener) context.Context { return requestContext },
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		result := make(chan error, 1)
		go func() {
			result <- Serve(signalContext, cancelRequests, server, listener, readiness, telemetry, testLogger(), 50*time.Millisecond)
		}()
		clientResult := make(chan error, 1)
		go func() {
			response, err := http.Get("http://" + listener.Addr().String())
			if response != nil {
				_ = response.Body.Close()
			}
			clientResult <- err
		}()
		<-started
		start := time.Now()
		signalCancel()
		require.NoError(t, <-result)
		elapsed := time.Since(start)
		assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
		assert.Less(t, elapsed, time.Second)
		assert.False(t, readiness.Ready())
		require.Error(t, <-clientResult, "force close must terminate the active client connection")
	})
}

func TestHTTPServer_MaxHeaderBytesIncludesDocumentedParserSlop(t *testing.T) {
	var handlerCalls atomic.Int64
	server := &http.Server{MaxHeaderBytes: 1024, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	tests := []struct {
		name   string
		length int
		status int
	}{
		{name: "below configured value", length: 512, status: http.StatusOK},
		{name: "inside 4096 byte parser slop", length: 1500, status: http.StatusOK},
		{name: "above effective read bound", length: 6000, status: http.StatusRequestHeaderFieldsTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			connection, err := net.Dial("tcp", listener.Addr().String())
			require.NoError(t, err)
			defer func() { _ = connection.Close() }()
			_, err = fmt.Fprintf(connection, "GET / HTTP/1.1\r\nHost: example\r\nX-Fill: %s\r\nConnection: close\r\n\r\n", strings.Repeat("a", tc.length))
			require.NoError(t, err)
			response, err := http.ReadResponse(bufio.NewReader(connection), nil)
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			assert.Equal(t, tc.status, response.StatusCode)
		})
	}
	assert.Equal(t, int64(2), handlerCalls.Load())
}

func writeProcessConfig(t *testing.T, baseURL string) string {
	t.Helper()
	contents := fmt.Sprintf(`providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_SECRET
    baseURL: %s
models:
  public:
    name: Public
    primary:
      provider: anthropic-primary
      model: backend-private
`, baseURL)
	path := filepath.Join(t.TempDir(), "models.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func processLifecycleEvents(t *testing.T, output string) []string {
	t.Helper()
	var events []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] != "gateway process lifecycle" {
			continue
		}
		require.Len(t, record, 4)
		for _, key := range []string{"time", "level", "msg", "event"} {
			assert.Contains(t, record, key)
		}
		event, ok := record["event"].(string)
		require.True(t, ok)
		events = append(events, event)
	}
	return events
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
