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
	"sync"
	"sync/atomic"
	"syscall"
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
		err := Run(
			context.Background(),
			[]string{"--server.write-timeout=1s"},
			func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
				}
				values := map[string]string{
					"GRAFANA_AI_GATEWAY_CONFIG_FILE":               "/nonexistent/private.yaml",
					"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL":             "https://auth.example/jwks",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED":         "true",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT":        "collector.example:4317",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV": "AGENTO11Y_SECRET",
				}
				value, ok := values[name]
				return value, ok
			},
			func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			testLogger(),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write timeout")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{name: "scalar failure", args: []string{"--server.write-timeout=1s"}, message: "write timeout"},
		{name: "unknown auth mode", args: []string{"--auth.mode=invalid"}, message: "auth"},
		{name: "cloud with JWKS", args: []string{"--auth.mode=cloud-gateway"}, message: "jwks"},
		{name: "cloud with unsafe", args: []string{"--auth.mode=cloud-gateway", "--auth.unsafe", "--auth.jwks-url="}, message: "unsafe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secretCalls := 0
			listenCalls := 0
			err := Run(
				context.Background(),
				tc.args,
				func(name string) (string, bool) {
					if name == "ANTHROPIC_SECRET" {
						secretCalls++
					}
					values := map[string]string{
						"GRAFANA_AI_GATEWAY_CONFIG_FILE":               "/nonexistent/private.yaml",
						"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL":             "https://auth.example/jwks",
						"GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED":         "true",
						"GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT":        "collector.example:4317",
						"GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV": "AGENTO11Y_SECRET",
					}
					value, ok := values[name]
					return value, ok
				},
				func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
				testLogger(),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
			assert.Zero(t, secretCalls)
			assert.Zero(t, listenCalls)
		})
	}

	t.Run("ambient agent observability environment before yaml", func(t *testing.T) {
		secretCalls := 0
		listenCalls := 0
		err := Run(
			context.Background(),
			nil,
			func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
				}
				values := map[string]string{
					"GRAFANA_AI_GATEWAY_CONFIG_FILE":               "/nonexistent/private.yaml",
					"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL":             "https://auth.example/jwks",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED":         "true",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT":        "collector.example:4317",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV": "AGENTO11Y_SECRET",
					"SIGIL_TAGS": "private=ambient",
				}
				value, ok := values[name]
				return value, ok
			},
			func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			testLogger(),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ambient agent observability SDK environment")
		assert.NotContains(t, err.Error(), "SIGIL_TAGS")
		assert.NotContains(t, err.Error(), "private=ambient")
		assert.NotContains(t, err.Error(), "nonexistent/private.yaml")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})

	t.Run("invalid production listen address", func(t *testing.T) {
		secretCalls := 0
		listenCalls := 0
		err := Run(
			context.Background(),
			[]string{"--server.listen-address=not-a-tcp-address"},
			func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
				}
				values := map[string]string{
					"GRAFANA_AI_GATEWAY_CONFIG_FILE":               "/nonexistent/private.yaml",
					"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL":             "https://auth.example/jwks",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED":         "true",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT":        "collector.example:4317",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV": "AGENTO11Y_SECRET",
				}
				value, ok := values[name]
				return value, ok
			},
			func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			testLogger(),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TCP host:port")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})

	t.Run("invalid production jwks before yaml", func(t *testing.T) {
		secretCalls := 0
		listenCalls := 0
		err := Run(
			context.Background(),
			nil,
			func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
				}
				values := map[string]string{
					"GRAFANA_AI_GATEWAY_CONFIG_FILE":               "/nonexistent/private.yaml",
					"GRAFANA_AI_GATEWAY_AUTH_JWKS_URL":             "http://auth.example/jwks",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED":         "true",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT":        "collector.example:4317",
					"GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV": "AGENTO11Y_SECRET",
				}
				value, ok := values[name]
				return value, ok
			},
			func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			testLogger(),
		)
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
		err := Run(
			context.Background(),
			[]string{"--deployment.mode=development", "--auth.unsafe", "--server.listen-address=127.0.0.1:0"},
			func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" {
					secretCalls++
					return "secret-value", true
				}
				if name == "GRAFANA_AI_GATEWAY_CONFIG_FILE" {
					return path, true
				}
				return "", false
			},
			func(string, string) (net.Listener, error) { listenCalls++; return nil, assert.AnError },
			testLogger(),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "userinfo")
		assert.Zero(t, secretCalls)
		assert.Zero(t, listenCalls)
	})
}

func TestRun_ResolvesAgentCredentialOnceBeforeListener(t *testing.T) {
	unsetAmbientAgentObservabilityEnvironment(t)
	path := writeProcessConfig(t, "https://provider.example")
	agentSecretCalls := 0
	listenCalls := 0
	var logs bytes.Buffer
	err := Run(
		context.Background(),
		[]string{
			"--deployment.mode=development",
			"--auth.unsafe",
			"--server.listen-address=127.0.0.1:0",
			"--agento11y.enabled",
			"--agento11y.protocol=grpc",
			"--agento11y.endpoint=127.0.0.1:1",
			"--agento11y.auth-secret-env=AO_SECRET",
			"--agento11y.max-retries=1",
			"--agento11y.initial-backoff=1ms",
			"--agento11y.max-backoff=1ms",
			"--agento11y.flush-timeout=10ms",
			"--agento11y.shutdown-timeout=10ms",
		},
		func(name string) (string, bool) {
			switch name {
			case "GRAFANA_AI_GATEWAY_CONFIG_FILE":
				return path, true
			case "ANTHROPIC_SECRET":
				return "provider-secret", true
			case "GRAFANA_AI_GATEWAY_AGENTO11Y_TLS":
				return "false", true
			case "AO_SECRET":
				agentSecretCalls++
				return "agent-secret", true
			default:
				return "", false
			}
		},
		func(string, string) (net.Listener, error) {
			listenCalls++
			return nil, assert.AnError
		},
		slog.New(slog.NewJSONHandler(&logs, nil)),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "binding listener")
	assert.Equal(t, 1, agentSecretCalls)
	assert.Equal(t, 1, listenCalls)
	for _, private := range []string{"provider-secret", "agent-secret", "AO_SECRET", "127.0.0.1:1"} {
		assert.NotContains(t, err.Error(), private)
		assert.NotContains(t, logs.String(), private)
	}
}

func TestRun_HTTPAgentObservabilityConfigurationReachesListener(t *testing.T) {
	unsetAmbientAgentObservabilityEnvironment(t)
	collector := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer collector.Close()
	path := writeProcessConfig(t, "http://provider.example")
	listenCalls := 0
	err := Run(
		context.Background(),
		[]string{
			"--deployment.mode=development",
			"--auth.unsafe",
			"--server.listen-address=127.0.0.1:0",
			"--agento11y.enabled",
			"--agento11y.protocol=http",
			"--agento11y.endpoint=" + collector.URL,
			"--no-agento11y.tls",
			"--agento11y.auth-secret-env=AO_SECRET",
			"--agento11y.queue-size=16",
			"--agento11y.batch-size=1",
			"--agento11y.payload-max-bytes=1048576",
			"--agento11y.max-retries=1",
			"--agento11y.initial-backoff=1ms",
			"--agento11y.max-backoff=1ms",
			"--agento11y.flush-interval=1ms",
			"--agento11y.flush-timeout=2s",
			"--agento11y.shutdown-timeout=2s",
		},
		func(name string) (string, bool) {
			switch name {
			case "GRAFANA_AI_GATEWAY_CONFIG_FILE":
				return path, true
			case "ANTHROPIC_SECRET":
				return "provider-secret", true
			case "AO_SECRET":
				return "agent-secret", true
			default:
				return "", false
			}
		},
		func(string, string) (net.Listener, error) {
			listenCalls++
			return nil, assert.AnError
		},
		testLogger(),
	)
	require.EqualError(t, err, "gateway process: binding listener: assert.AnError general error for testing")
	assert.Equal(t, 1, listenCalls)
}

func TestRun_LocalReadinessDoesNotProbeProvider(t *testing.T) {
	for _, tc := range []struct {
		name     string
		authArgs []string
		split    bool
	}{
		{name: "internal unsafe", authArgs: []string{"--auth.unsafe"}},
		{name: "internal split", split: true, authArgs: []string{"--auth.unsafe", "--server.operational-listen-address=127.0.0.1:0"}},
		{name: "cloud without JWKS", split: true, authArgs: []string{"--server.operational-listen-address=127.0.0.1:0", "--auth.mode=cloud-gateway", "--auth.jwks-timeout=0s", "--auth.jwks-response-bytes=0", "--auth.jwks-max-keys=0", "--auth.jwks-max-age=0s", "--auth.jwks-refresh-interval=0s", "--server.write-timeout=155s"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var providerCalls atomic.Int64
			providerServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				providerCalls.Add(1)
			}))
			defer providerServer.Close()
			path := writeProcessConfig(t, providerServer.URL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			addresses := make(chan string, 2)
			result := make(chan error, 1)
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			go func() {
				result <- Run(
					ctx,
					append([]string{"--deployment.mode=development", "--server.listen-address=127.0.0.1:0"}, tc.authArgs...),
					func(name string) (string, bool) {
						switch name {
						case "GRAFANA_AI_GATEWAY_CONFIG_FILE":
							return path, true
						case "ANTHROPIC_SECRET":
							return "secret-value", true
						default:
							return "", false
						}
					},
					func(network, address string) (net.Listener, error) {
						listener, err := net.Listen(network, address)
						if err == nil {
							addresses <- listener.Addr().String()
						}
						return listener, err
					},
					logger,
				)
			}()
			var address string
			select {
			case address = <-addresses:
			case err := <-result:
				require.FailNow(t, "process exited before binding", "%v", err)
			case <-time.After(2 * time.Second):
				require.FailNow(t, "process did not bind")
			}
			operationalAddress := address
			if tc.split {
				select {
				case operationalAddress = <-addresses:
				case err := <-result:
					require.FailNow(t, "process exited before operational bind", "%v", err)
				case <-time.After(2 * time.Second):
					require.FailNow(t, "operational listener did not bind")
				}
			}
			require.Eventually(t, func() bool {
				response, err := http.Get("http://" + operationalAddress + "/ready")
				if err != nil {
					return false
				}
				defer func() { _ = response.Body.Close() }()
				return response.StatusCode == http.StatusOK
			}, 2*time.Second, 10*time.Millisecond)
			assert.Zero(t, providerCalls.Load())
			if tc.split {
				for _, url := range []string{"http://" + address + "/ready", "http://" + operationalAddress + "/api/v1/aisdk/config"} {
					response, err := http.Get(url)
					require.NoError(t, err)
					_ = response.Body.Close()
					assert.Equal(t, http.StatusNotFound, response.StatusCode)
				}
			}
			if tc.name == "cloud without JWKS" {
				request, err := http.NewRequest(http.MethodGet, "http://"+address+"/api/v1/aisdk/config", nil)
				require.NoError(t, err)
				request.Header.Set("X-Scope-OrgID", "123")
				response, err := http.DefaultClient.Do(request)
				require.NoError(t, err)
				_ = response.Body.Close()
				assert.Equal(t, http.StatusOK, response.StatusCode)
			}
			cancel()
			require.NoError(t, <-result)
			assert.Equal(t, []string{processEventStarting, processEventReady, processEventShutdownStarted, processEventShutdownCompleted}, processLifecycleEvents(t, logs.String()))
			for _, private := range []string{"secret-value", providerServer.URL, "backend-private", "ANTHROPIC_SECRET"} {
				assert.NotContains(t, logs.String(), private)
			}
		})
	}
}

func TestServe_FinalizesObservabilityBeforeProcessCompletionEvent(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	stop()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	readiness := &service.Readiness{}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := service.NewTelemetry(testLogger())
	require.NoError(t, err)
	err = Serve(ctx, func() {}, []boundServer{{server: &http.Server{}, listener: listener}}, readiness, telemetry, logger, time.Second, func() {
		logProcessEvent(logger, "observability_finalized")
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		processEventShutdownStarted,
		"observability_finalized",
		processEventShutdownCompleted,
	}, processLifecycleEvents(t, logs.String()))
}

func TestServe_RejectsInvalidDependenciesBeforeStarting(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		empty   bool
	}{
		{name: "zero", timeout: 0},
		{name: "negative", timeout: -time.Second},
		{name: "no servers", timeout: time.Second, empty: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			telemetry, err := service.NewTelemetry(testLogger())
			require.NoError(t, err)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer func() { _ = listener.Close() }()
			readiness := &service.Readiness{}
			canceled := false
			var logs bytes.Buffer
			servers := []boundServer{{server: &http.Server{}, listener: listener}}
			if tc.empty {
				servers = nil
			}

			err = Serve(
				context.Background(),
				func() { canceled = true },
				servers,
				readiness,
				telemetry,
				slog.New(slog.NewJSONHandler(&logs, nil)),
				tc.timeout,
				nil,
			)

			require.EqualError(t, err, "gateway process: invalid serve dependency")
			assert.False(t, canceled)
			assert.False(t, readiness.Ready())
			assert.Empty(t, logs.String())
		})
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
			result <- Serve(signalContext, cancelRequests, []boundServer{{server: server, listener: listener}}, readiness, telemetry, testLogger(), time.Second, nil)
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
			result <- Serve(signalContext, cancelRequests, []boundServer{{server: server, listener: listener}}, readiness, telemetry, testLogger(), 50*time.Millisecond, nil)
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

func TestRun_BindFailures(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprintf("listener %d", failAt), func(t *testing.T) {
			path := writeProcessConfig(t, "https://provider.example")
			var listeners []net.Listener
			calls := 0
			var logs bytes.Buffer
			err := Run(context.Background(), []string{"--config.file=" + path, "--deployment.mode=development", "--auth.mode=cloud-gateway", "--server.listen-address=127.0.0.1:0", "--server.operational-listen-address=127.0.0.1:0"}, func(name string) (string, bool) { return "secret", name == "ANTHROPIC_SECRET" }, func(network, address string) (net.Listener, error) {
				calls++
				if calls == failAt {
					return nil, assert.AnError
				}
				listener, err := net.Listen(network, address)
				if err == nil {
					listeners = append(listeners, listener)
				}
				return listener, err
			}, slog.New(slog.NewJSONHandler(&logs, nil)))
			require.ErrorIs(t, err, assert.AnError)
			assert.Equal(t, failAt, calls)
			assert.NotContains(t, processLifecycleEvents(t, logs.String()), processEventReady)
			assert.Contains(t, logs.String(), `"stage":"bind"`)
			assert.NotContains(t, logs.String(), assert.AnError.Error())
			for _, listener := range listeners {
				defer func() { _ = listener.Close() }()
				connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
				if connection != nil {
					_ = connection.Close()
				}
				require.Error(t, err)
			}
		})
	}
}

func unsetAmbientAgentObservabilityEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"AGENTO11Y_ENDPOINT", "SIGIL_ENDPOINT", "AGENTO11Y_PROTOCOL", "SIGIL_PROTOCOL",
		"AGENTO11Y_INSECURE", "SIGIL_INSECURE", "AGENTO11Y_HEADERS", "SIGIL_HEADERS",
		"AGENTO11Y_AUTH_MODE", "SIGIL_AUTH_MODE", "AGENTO11Y_AUTH_TENANT_ID", "SIGIL_AUTH_TENANT_ID",
		"AGENTO11Y_AUTH_TOKEN", "SIGIL_AUTH_TOKEN", "AGENTO11Y_AGENT_NAME", "SIGIL_AGENT_NAME",
		"AGENTO11Y_AGENT_VERSION", "SIGIL_AGENT_VERSION", "AGENTO11Y_USER_ID", "SIGIL_USER_ID",
		"AGENTO11Y_TAGS", "SIGIL_TAGS", "AGENTO11Y_CONTENT_CAPTURE_MODE", "SIGIL_CONTENT_CAPTURE_MODE",
		"AGENTO11Y_DEBUG", "SIGIL_DEBUG", "AGENTO11Y_REDACT_INPUT_MESSAGES", "SIGIL_REDACT_INPUT_MESSAGES",
	} {
		t.Setenv(name, "")
	}
}

func TestServe_DualLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name              string
		failAt            int
		ignoreCancel      bool
		cancelBeforeStart bool
		failAfterStartup  bool
	}{
		{name: "API failure", failAt: 1},
		{name: "operational failure", failAt: 2},
		{name: "API failure after startup", failAt: 1, failAfterStartup: true},
		{name: "operational failure after startup", failAt: 2, failAfterStartup: true},
		{name: "already canceled", cancelBeforeStart: true},
		{name: "shared forced shutdown", ignoreCancel: true},
		{name: "cancel-first graceful shutdown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			requestContext, cancelRequests := context.WithCancel(context.Background())
			defer cancelRequests()
			readiness := &service.Readiness{}
			telemetry, err := service.NewTelemetry(testLogger())
			require.NoError(t, err)
			started := make(chan struct{}, 2)
			completed := make(chan struct{}, 2)
			release := make(chan struct{})
			defer close(release)
			var servers []boundServer
			for i := range 2 {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				defer func() { _ = listener.Close() }()
				if tc.failAt == i+1 {
					listener = &failingListener{Listener: listener, acceptFirst: tc.failAfterStartup}
				}
				server := &http.Server{BaseContext: func(net.Listener) context.Context { return requestContext }, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					started <- struct{}{}
					if tc.ignoreCancel {
						<-release
					} else {
						<-r.Context().Done()
						assert.False(t, readiness.Ready())
					}
					w.WriteHeader(http.StatusNoContent)
				})}
				servers = append(servers, boundServer{server: server, listener: listener})
			}
			var logs bytes.Buffer
			if tc.cancelBeforeStart {
				stop()
			}
			result := make(chan error, 1)
			go func() {
				result <- Serve(ctx, cancelRequests, servers, readiness, telemetry, slog.New(slog.NewJSONHandler(&logs, nil)), 200*time.Millisecond, nil)
			}()
			var shutdownStarted time.Time
			if tc.failAt == 0 && !tc.cancelBeforeStart {
				for _, binding := range servers {
					go func() {
						response, _ := http.Get("http://" + binding.listener.Addr().String())
						if response != nil {
							_ = response.Body.Close()
						}
						completed <- struct{}{}
					}()
				}
				for range 2 {
					select {
					case <-started:
					case <-time.After(2 * time.Second):
						require.FailNow(t, "handler did not start")
					}
				}
				shutdownStarted = time.Now()
				stop()
			}
			select {
			case err := <-result:
				if tc.failAt > 0 {
					require.ErrorIs(t, err, assert.AnError)
				} else {
					require.NoError(t, err)
				}
			case <-time.After(time.Second):
				require.FailNow(t, "servers did not stop under shared deadline")
			}
			if tc.ignoreCancel {
				assert.Less(t, time.Since(shutdownStarted), 350*time.Millisecond, "both servers share one deadline")
			}
			if tc.failAfterStartup {
				assert.Contains(t, logs.String(), `"stage":"serve"`)
				assert.NotContains(t, logs.String(), assert.AnError.Error())
			}
			assert.False(t, readiness.Ready())
			events := processLifecycleEvents(t, logs.String())
			if (tc.failAt > 0 && !tc.failAfterStartup) || tc.cancelBeforeStart {
				assert.Equal(t, []string{processEventShutdownStarted, processEventShutdownCompleted}, events)
			} else {
				assert.Equal(t, []string{processEventReady, processEventShutdownStarted, processEventShutdownCompleted}, events)
			}
			assert.ErrorIs(t, requestContext.Err(), context.Canceled)
			for _, binding := range servers {
				connection, err := net.DialTimeout("tcp", binding.listener.Addr().String(), time.Second)
				if connection != nil {
					_ = connection.Close()
				}
				require.Error(t, err)
			}
			if tc.failAt == 0 && !tc.cancelBeforeStart {
				for range 2 {
					select {
					case <-completed:
					case <-time.After(time.Second):
						require.FailNow(t, "client connection survived shutdown")
					}
				}
			}
		})
	}
}

func TestServe_ListenerStartup(t *testing.T) {
	for _, tc := range []struct {
		name         string
		count        int
		pauseAt      int
		network      string
		address      string
		ipv4Fallback bool
		cancel       bool
		timeout      bool
		probeFailure bool
	}{
		{name: "combined", count: 1, pauseAt: 1, address: "127.0.0.1:0"},
		{name: "API pending", count: 2, pauseAt: 1, address: "127.0.0.1:0"},
		{name: "operational pending", count: 2, pauseAt: 2, address: "127.0.0.1:0"},
		{name: "IPv4 wildcard", count: 1, pauseAt: 1, address: "0.0.0.0:0"},
		{name: "IPv6 wildcard", count: 1, pauseAt: 1, address: "[::]:0"},
		{name: "IPv4-only wildcard", count: 2, pauseAt: 1, network: "tcp4", address: "0.0.0.0:0"},
		{name: "IPv6-only wildcard", count: 2, pauseAt: 2, network: "tcp6", address: "[::]:0"},
		{name: "wildcard with IPv4 fallback", count: 2, pauseAt: 2, network: "tcp4", address: "0.0.0.0:0", ipv4Fallback: true},
		{name: "cancel pending startup", count: 2, pauseAt: 2, address: "127.0.0.1:0", cancel: true},
		{name: "startup timeout", count: 2, pauseAt: 1, address: "127.0.0.1:0", timeout: true},
		{name: "probe failure", count: 2, pauseAt: 2, address: "127.0.0.1:0", probeFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			requestContext, cancelRequests := context.WithCancel(context.Background())
			defer cancelRequests()
			readiness := &service.Readiness{}
			telemetry, err := service.NewTelemetry(testLogger())
			require.NoError(t, err)
			var servers []boundServer
			var paused *pausedListener
			network := tc.network
			if network == "" {
				network = "tcp"
			}
			if network == "tcp6" {
				probe, err := net.Listen("tcp6", "[::1]:0")
				if err != nil {
					t.Skipf("IPv6 loopback unavailable: %v", err)
				}
				connection, err := net.DialTimeout("tcp6", probe.Addr().String(), time.Second)
				_ = probe.Close()
				if err != nil {
					t.Skipf("IPv6 loopback unavailable: %v", err)
				}
				_ = connection.Close()
			}
			for i := range tc.count {
				listener, err := net.Listen(network, tc.address)
				require.NoError(t, err)
				if i+1 == tc.pauseAt {
					paused = &pausedListener{Listener: listener, entered: make(chan struct{}), resume: make(chan struct{}), closed: make(chan struct{})}
					if tc.ipv4Fallback {
						address, err := net.ResolveTCPAddr("tcp", listener.Addr().String())
						require.NoError(t, err)
						address.IP = net.IPv6unspecified
						paused.address = address
					}
					if tc.probeFailure {
						paused.address = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: -1}
					}
					listener = paused
				}
				defer func() { _ = listener.Close() }()
				servers = append(servers, boundServer{server: &http.Server{}, listener: listener})
			}
			var logs bytes.Buffer
			result := make(chan error, 1)
			go func() {
				result <- Serve(ctx, cancelRequests, servers, readiness, telemetry, slog.New(slog.NewJSONHandler(&logs, nil)), time.Second, nil)
			}()
			if !tc.probeFailure {
				select {
				case <-paused.entered:
				case <-time.After(time.Second):
					require.FailNow(t, "listener did not enter Accept")
				}
			}
			assert.False(t, readiness.Ready())
			metrics := httptest.NewRecorder()
			telemetry.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			assert.Contains(t, metrics.Body.String(), "grafana_ai_gateway_ready 0\n")
			if tc.cancel {
				stop()
			} else if !tc.timeout && !tc.probeFailure {
				close(paused.resume)
				require.Eventually(t, func() bool {
					metrics := httptest.NewRecorder()
					telemetry.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
					return readiness.Ready() && strings.Contains(metrics.Body.String(), "grafana_ai_gateway_ready 1\n")
				}, time.Second, time.Millisecond)
				stop()
			}
			wait := 2 * time.Second
			if tc.timeout {
				wait += listenerStartupTimeout
			}
			select {
			case err := <-result:
				if tc.timeout {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				} else if tc.probeFailure {
					require.ErrorContains(t, err, "probing listener")
				} else {
					require.NoError(t, err)
				}
			case <-time.After(wait):
				require.FailNow(t, "startup did not stop")
			}
			assert.False(t, readiness.Ready())
			assert.ErrorIs(t, requestContext.Err(), context.Canceled)
			events := processLifecycleEvents(t, logs.String())
			if tc.cancel || tc.timeout || tc.probeFailure {
				assert.Equal(t, []string{processEventShutdownStarted, processEventShutdownCompleted}, events)
			} else {
				assert.Equal(t, []string{processEventReady, processEventShutdownStarted, processEventShutdownCompleted}, events)
			}
			if tc.probeFailure {
				assert.Contains(t, logs.String(), `"stage":"probe"`)
				assert.NotContains(t, logs.String(), "127.0.0.1:-1")
			} else if tc.timeout {
				assert.Contains(t, logs.String(), `"reason":"timeout"`)
			} else {
				assert.NotContains(t, logs.String(), "gateway listener failed")
			}
		})
	}
}

func TestLogListenerFailure_Privacy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		reason string
	}{
		{name: "unavailable address", err: &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv6loopback, Port: 8080}, Err: &os.SyscallError{Syscall: "connect", Err: syscall.EADDRNOTAVAIL}}, reason: syscall.EADDRNOTAVAIL.Error()},
		{name: "refused connection", err: syscall.ECONNREFUSED, reason: syscall.ECONNREFUSED.Error()},
		{name: "timeout", err: context.DeadlineExceeded, reason: "timeout"},
		{name: "canceled", err: context.Canceled, reason: "canceled"},
		{name: "closed", err: net.ErrClosed, reason: "closed"},
		{name: "unknown", err: assert.AnError, reason: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logListenerFailure(slog.New(slog.NewJSONHandler(&logs, nil)), "probe", fmt.Errorf("private credential and endpoint: %w", tc.err))
			var record map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
			delete(record, "time")
			assert.Equal(t, map[string]any{
				"level":  "ERROR",
				"msg":    "gateway listener failed",
				"stage":  "probe",
				"reason": tc.reason,
			}, record)
		})
	}
}

type pausedListener struct {
	net.Listener
	entered chan struct{}
	resume  chan struct{}
	closed  chan struct{}
	once    sync.Once
	address net.Addr
}

func (listener *pausedListener) Addr() net.Addr {
	if listener.address != nil {
		return listener.address
	}
	return listener.Listener.Addr()
}

func (listener *pausedListener) Accept() (net.Conn, error) {
	if listener.entered != nil {
		close(listener.entered)
		select {
		case <-listener.resume:
		case <-listener.closed:
			return nil, net.ErrClosed
		}
		listener.entered = nil
	}
	return listener.Listener.Accept()
}

func (listener *pausedListener) Close() error {
	listener.once.Do(func() { close(listener.closed) })
	return listener.Listener.Close()
}

type failingListener struct {
	net.Listener
	acceptFirst bool
}

func (listener *failingListener) Accept() (net.Conn, error) {
	if listener.acceptFirst {
		listener.acceptFirst = false
		return listener.Listener.Accept()
	}
	return nil, assert.AnError
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
