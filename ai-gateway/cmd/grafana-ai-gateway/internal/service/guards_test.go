package service

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ providerv4.Guard = (*GuardRuntime)(nil)

func guardRuntimeSettings(endpoint string) config.GuardSettings {
	return config.GuardSettings{Enabled: true, Endpoint: endpoint, TenantID: "operator", AuthMode: config.GuardAuthModeBasic, AuthUsername: "instance", AuthSecretEnv: "GUARD_SECRET", Timeout: time.Second, BodyBytes: 4096, RetainedBytes: 1 << 20, MaxConcurrent: 2}
}
func newTestGuardRuntime(t *testing.T, settings config.GuardSettings) (*GuardRuntime, *Telemetry) {
	t.Helper()
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	runtime, err := NewGuardRuntime(settings, config.DeploymentDevelopment, func(name string) (string, bool) {
		assert.Equal(t, "GUARD_SECRET", name)
		return "operator-secret", true
	}, telemetry)
	require.NoError(t, err)
	t.Cleanup(runtime.Close)
	return runtime, telemetry
}
func guardRuntimeOptions(text string) provider.CallOptions {
	return provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{provider.TextPart(text)}}}}
}
func TestGuardRuntimeDisabledAndInvalid(t *testing.T) {
	runtime, err := NewGuardRuntime(config.GuardSettings{}, config.DeploymentProduction, func(string) (string, bool) { require.FailNow(t, "secret lookup while disabled"); return "", false }, nil)
	require.NoError(t, err)
	assert.Nil(t, runtime)
	settings := guardRuntimeSettings("http://sigil.example")
	runtime, err = NewGuardRuntime(settings, config.DeploymentProduction, nil, nil)
	require.Error(t, err)
	assert.Nil(t, runtime)
	settings.Timeout = 0
	runtime, err = NewGuardRuntime(settings, config.DeploymentDevelopment, nil, nil)
	require.Error(t, err)
	assert.Nil(t, runtime)
}
func TestGuardRuntimePartialConstructionCleanup(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	blocker := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "grafana_ai_gateway", Name: "guard_evaluation_duration_seconds", Help: "Guard evaluation duration by fixed phase and outcome."}, []string{"phase", "outcome"})
	require.NoError(t, telemetry.Registerer().Register(blocker))
	settings := guardRuntimeSettings("http://127.0.0.1:1")
	lookup := func(string) (string, bool) { return "operator-secret", true }
	runtime, err := NewGuardRuntime(settings, config.DeploymentDevelopment, lookup, telemetry)
	require.Error(t, err)
	assert.Nil(t, runtime)
	require.True(t, telemetry.Registerer().Unregister(blocker))
	runtime, err = NewGuardRuntime(settings, config.DeploymentDevelopment, lookup, telemetry)
	require.NoError(t, err)
	runtime.Close()
	runtime, err = NewGuardRuntime(settings, config.DeploymentDevelopment, lookup, telemetry)
	require.NoError(t, err)
	runtime.Close()
}

func TestGuardRuntimeAdmissionAndClose(t *testing.T) {
	runtime, _ := newTestGuardRuntime(t, guardRuntimeSettings("http://127.0.0.1:1"))
	release1, err := runtime.Acquire(context.Background())
	require.NoError(t, err)
	release2, err := runtime.Acquire(context.Background())
	require.NoError(t, err)
	_, err = runtime.Acquire(context.Background())
	require.ErrorIs(t, err, providerv4.ErrGuardFailed)
	release1()
	release1()
	release3, err := runtime.Acquire(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = runtime.Acquire(ctx)
	require.ErrorIs(t, err, context.Canceled)
	release2()
	release3()
	runtime.Close()
	runtime.Close()
	_, err = runtime.Acquire(context.Background())
	require.ErrorIs(t, err, providerv4.ErrGuardFailed)
}
func TestGuardRuntimeWireIsolation(t *testing.T) {
	for _, auth := range []config.GuardAuthMode{config.GuardAuthModeBasic, config.GuardAuthModeBearer} {
		t.Run(string(auth), func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, "/prefix/api/v1/hooks:evaluate", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "operator", r.Header.Get("X-Scope-OrgID"))
				assert.Empty(t, r.Header.Get("X-Caller"))
				if auth == config.GuardAuthModeBasic {
					u, p, ok := r.BasicAuth()
					assert.True(t, ok)
					assert.Equal(t, "instance", u)
					assert.Equal(t, "operator-secret", p)
				} else {
					assert.Equal(t, "Bearer operator-secret", r.Header.Get("Authorization"))
				}
				millis, err := strconv.Atoi(r.Header.Get("X-Agento11y-Hook-Timeout-Ms"))
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, millis, 1)
				assert.Less(t, millis, 120000)
				var body struct {
					Phase   string `json:"phase"`
					Context struct {
						Agent string `json:"agent_name"`
						Model struct {
							Provider string `json:"provider"`
							Name     string `json:"name"`
						} `json:"model"`
					} `json:"context"`
					Input map[string]json.RawMessage `json:"input"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "grafana-ai-gateway", body.Context.Agent)
				assert.Equal(t, "grafana", body.Context.Model.Provider)
				assert.Equal(t, "canonical", body.Context.Model.Name)
				assert.Contains(t, body.Input, "messages")
				if body.Phase == "postflight" {
					assert.Contains(t, body.Input, "output")
				} else {
					assert.Equal(t, "preflight", body.Phase)
				}
				_, _ = io.WriteString(w, `{"action":"allow"}`)
			}))
			defer server.Close()
			settings := guardRuntimeSettings(server.URL + "/prefix/")
			settings.AuthMode = auth
			if auth == config.GuardAuthModeBearer {
				settings.AuthUsername = ""
			}
			runtime, _ := newTestGuardRuntime(t, settings)
			options := guardRuntimeOptions("private canary")
			effective, err := runtime.Preflight(context.Background(), "canonical", options)
			require.NoError(t, err)
			require.NoError(t, runtime.Postflight(context.Background(), "canonical", effective, []provider.ContentPart{provider.TextPart("output canary")}))
			options.Headers = map[string]string{"Authorization": "caller", "X-Scope-OrgID": "caller", "X-Caller": "caller"}
			_, err = runtime.Preflight(context.Background(), "canonical", options)
			require.ErrorIs(t, err, providerv4.ErrGuardUnsupported)
			assert.Equal(t, int64(2), calls.Load())
		})
	}
}
func TestGuardRuntimeFailurePolicy(t *testing.T) {
	for _, open := range []bool{false, true} {
		for _, tc := range []struct {
			name, body string
			status     int
			want       error
			eligible   bool
			post       bool
		}{
			{name: "deny diagnostics", body: `{"action":"deny","reason":[],"evaluations":42,"transformed_input":null}`, want: providerv4.ErrGuardDenied},
			{name: "deny duplicate diagnostics", body: `{"action":"deny","reason":[],"reason":42}`, want: providerv4.ErrGuardDenied},
			{name: "deny duplicate transform", body: `{"action":"deny","transformed_input":null,"transformed_input":42}`, want: providerv4.ErrGuardDenied},
			{name: "allow ignores optional diagnostics", body: `{"action":"allow","reason":[],"rule_id":{},"evaluations":42}`},
			{name: "transform with malformed diagnostics", body: `{"action":"allow","reason":[],"transformed_input":null}`, want: providerv4.ErrGuardFailed},
			{name: "unknown", body: `{"action":"warn"}`, want: providerv4.ErrGuardFailed, eligible: true},
			{name: "duplicate action with deny", body: `{"action":"deny","action":"allow"}`, want: providerv4.ErrGuardFailed, eligible: true},
			{name: "duplicate transform", body: `{"action":"allow","transformed_input":{},"transformed_input":null}`, want: providerv4.ErrGuardFailed},
			{name: "missing", body: `{}`, want: providerv4.ErrGuardFailed, eligible: true},
			{name: "non 200", status: 503, want: providerv4.ErrGuardFailed, eligible: true},
			{name: "unsafe transform", body: `{"action":"allow","transformed_input":null}`, want: providerv4.ErrGuardFailed},
			{name: "transform malformed diagnostics", body: `{"action":"allow","transformed_input":null,"reason":[]}`, want: providerv4.ErrGuardFailed},
			{name: "postflight transform malformed diagnostics", body: `{"action":"allow","transformed_input":{},"evaluations":42}`, want: providerv4.ErrGuardFailed, post: true},
			{name: "postflight transform", body: `{"action":"allow","transformed_input":{}}`, want: providerv4.ErrGuardFailed, post: true},
		} {
			t.Run(tc.name+"/open="+strconv.FormatBool(open), func(t *testing.T) {
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if tc.status != 0 {
						w.WriteHeader(tc.status)
					}
					_, _ = io.WriteString(w, tc.body)
				}))
				defer server.Close()
				settings := guardRuntimeSettings(server.URL)
				settings.FailOpen = open
				runtime, telemetry := newTestGuardRuntime(t, settings)
				var err error
				if tc.post {
					err = runtime.Postflight(context.Background(), "canonical", guardRuntimeOptions("input"), []provider.ContentPart{provider.TextPart("out")})
				} else {
					_, err = runtime.Preflight(context.Background(), "canonical", guardRuntimeOptions("input"))
				}
				if tc.want == nil || (open && tc.eligible) {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, tc.want)
				}
				assert.Equal(t, int64(1), calls.Load(), "guard RPCs must not retry")
				metrics := httptest.NewRecorder()
				telemetry.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
				assert.Contains(t, metrics.Body.String(), "grafana_ai_gateway_guard_evaluations_total")
				assert.NotContains(t, metrics.Body.String(), "canonical")
				assert.NotContains(t, metrics.Body.String(), "operator-secret")
			})
		}
	}
}
func TestGuardRuntimePreflightTransformAndEmptyPostflight(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request guardHookRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		calls.Add(1)
		if request.Phase == guardPreflight {
			_, _ = io.WriteString(w, `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"text","text":"redacted"}]}]}}`)
		} else {
			require.Len(t, request.Input.Output, 1)
			assert.Equal(t, provider.RoleAssistant, request.Input.Output[0].Role)
			_, _ = io.WriteString(w, `{"action":"allow"}`)
		}
	}))
	defer server.Close()
	runtime, _ := newTestGuardRuntime(t, guardRuntimeSettings(server.URL))
	original := guardRuntimeOptions("original canary")
	effective, err := runtime.Preflight(context.Background(), "canonical", original)
	require.NoError(t, err)
	assert.Equal(t, "redacted", effective.Prompt[0].Content[0].Text)
	assert.Equal(t, "original canary", original.Prompt[0].Content[0].Text)
	require.NoError(t, runtime.Postflight(context.Background(), "canonical", effective, nil))
	assert.Equal(t, int64(2), calls.Load())
}

func TestGuardRuntimeTransportBounds(t *testing.T) {
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	for _, kind := range []string{"redirect", "oversize", "gzip"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				case "gzip":
					w.Header().Set("Content-Encoding", "gzip")
					gz := gzip.NewWriter(w)
					_, _ = io.WriteString(gz, strings.Repeat("x", 8192))
					_ = gz.Close()
				default:
					_, _ = io.WriteString(w, strings.Repeat("x", 8192))
				}
			}))
			defer server.Close()
			settings := guardRuntimeSettings(server.URL)
			settings.FailOpen = true
			runtime, _ := newTestGuardRuntime(t, settings)
			_, err := runtime.Preflight(context.Background(), "canonical", guardRuntimeOptions("input"))
			if kind == "redirect" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, providerv4.ErrGuardFailed)
			}
		})
	}
	assert.Zero(t, redirected.Load())
}
func TestGuardRuntimeLocalLimitsAndUnsupportedNeverOpen(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"action":"allow"}`)
	}))
	defer server.Close()
	for _, kind := range []string{"request", "retained", "unsupported"} {
		t.Run(kind, func(t *testing.T) {
			settings := guardRuntimeSettings(server.URL)
			settings.FailOpen = true
			options := guardRuntimeOptions(strings.Repeat("x", 10000))
			want := providerv4.ErrGuardFailed
			switch kind {
			case "retained":
				settings.RetainedBytes = 1000
			case "unsupported":
				options = guardRuntimeOptions("input")
				options.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"instructions":"hidden"}`)}}
				want = providerv4.ErrGuardUnsupported
			}
			runtime, _ := newTestGuardRuntime(t, settings)
			_, err := runtime.Preflight(context.Background(), "canonical", options)
			require.ErrorIs(t, err, want)
		})
	}
	assert.Zero(t, calls.Load())
}
func TestGuardRuntimeUnsupportedOutputClosed(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	settings := guardRuntimeSettings(server.URL)
	settings.FailOpen = true
	runtime, _ := newTestGuardRuntime(t, settings)
	err := runtime.Postflight(context.Background(), "canonical", guardRuntimeOptions("input"), []provider.ContentPart{{Type: provider.ContentPartTypeCustom, Kind: "private"}})
	require.ErrorIs(t, err, providerv4.ErrGuardFailed)
	assert.Zero(t, calls.Load())
}

func TestGuardRuntimeCancellationTimeoutAndClose(t *testing.T) {
	for _, kind := range []string{"cancel", "timeout", "operation deadline", "close"} {
		t.Run(kind, func(t *testing.T) {
			started := make(chan struct{})
			var once sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				millis, err := strconv.Atoi(r.Header.Get("X-Agento11y-Hook-Timeout-Ms"))
				assert.NoError(t, err)
				assert.Less(t, millis, 50)
				if kind == "operation deadline" {
					assert.Less(t, millis, 30)
				}
				once.Do(func() { close(started) })
				<-r.Context().Done()
			}))
			defer server.Close()
			settings := guardRuntimeSettings(server.URL)
			settings.FailOpen = true
			settings.Timeout = 50 * time.Millisecond
			runtime, _ := newTestGuardRuntime(t, settings)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "operation deadline" {
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
			}
			result := make(chan error, 1)
			go func() { _, err := runtime.Preflight(ctx, "canonical", guardRuntimeOptions("input")); result <- err }()
			<-started
			switch kind {
			case "cancel":
				cancel()
			case "close":
				runtime.Close()
			}
			select {
			case err := <-result:
				switch kind {
				case "cancel":
					require.ErrorIs(t, err, context.Canceled)
				case "close":
					require.ErrorIs(t, err, providerv4.ErrGuardFailed)
				default:
					require.NoError(t, err, "phase service timeout is eligible for fail-open while parent remains active")
				}
			case <-time.After(time.Second):
				require.FailNow(t, "hook did not stop")
			}
		})
	}
}
func TestGuardRuntimeCancellationOverridesLateVerdict(t *testing.T) {
	for _, action := range []string{"allow", "deny"} {
		t.Run(action, func(t *testing.T) {
			runtime, _ := newTestGuardRuntime(t, guardRuntimeSettings("http://127.0.0.1:1"))
			runtime.settings.FailOpen = true
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runtime.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				cancel()
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"action":"` + action + `"}`)), Request: r}, nil
			})
			_, err := runtime.Preflight(ctx, "canonical", guardRuntimeOptions("input"))
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}

func guardReserveGraph(value reflect.Value, budget *int64, depth int) bool {
	return guardReserveGraphContext(context.Background(), value, budget, depth)
}

func TestGuardRuntimeConservativeAllocationBudget(t *testing.T) {
	small := int64(1024)
	assert.False(t, guardReserveGraph(reflect.ValueOf(strings.Repeat("x", 1024)), &small, 0))
	assert.GreaterOrEqual(t, small, int64(0))
	large := int64(1 << 20)
	assert.True(t, guardReserveGraph(reflect.ValueOf(guardRuntimeOptions("input")), &large, 0))
	assert.Less(t, large, int64(1<<20))
	type node struct{ Next *node }
	cycle := &node{}
	cycle.Next = cycle
	large = 1 << 20
	assert.False(t, guardReserveGraph(reflect.ValueOf(cycle), &large, 0))
}

func TestGuardRuntimeParallelIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "blocked") {
			_, _ = io.WriteString(w, `{"action":"deny"}`)
		} else {
			_, _ = io.WriteString(w, `{"action":"allow"}`)
		}
	}))
	defer server.Close()
	runtime, _ := newTestGuardRuntime(t, guardRuntimeSettings(server.URL))
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text := "safe"
			if i%2 == 0 {
				text = "blocked"
			}
			_, err := runtime.Preflight(context.Background(), "canonical", guardRuntimeOptions(text))
			if i%2 == 0 {
				assert.ErrorIs(t, err, providerv4.ErrGuardDenied)
			} else {
				assert.NoError(t, err)
			}
		}()
	}
	wg.Wait()
}
