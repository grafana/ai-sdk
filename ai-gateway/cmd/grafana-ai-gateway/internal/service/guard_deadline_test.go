package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type guardCheckpointContext struct {
	context.Context
	target   string
	after    int
	hits     int
	expired  context.Context
	onExpire func()
}

func newGuardCheckpointContext(t *testing.T, target string, after int) *guardCheckpointContext {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return &guardCheckpointContext{Context: ctx, target: target, after: after}
}

func (ctx *guardCheckpointContext) Err() error {
	if ctx.expired != nil {
		return ctx.expired.Err()
	}
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ctx.target) {
			ctx.hits++
			if ctx.hits == ctx.after {
				var cancel context.CancelFunc
				ctx.expired, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				cancel()
				if ctx.onExpire != nil {
					ctx.onExpire()
				}
				return ctx.expired.Err()
			}
			break
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func TestGuardCancellation_SynchronousWork(t *testing.T) {
	options := guardTestOptions()
	input, err := makeGuardInput(options, nil)
	require.NoError(t, err)
	raw, err := json.Marshal(guardTestResponse(t, input))
	require.NoError(t, err)
	for _, tc := range []struct {
		name, target string
		work         func(context.Context) bool
	}{
		{"supported projection", "service.guardJSONToken", func(ctx context.Context) bool {
			_, err := makeGuardInputContext(ctx, options, nil)
			return err != nil
		}},
		{"transform parsing", "service.guardJSONShape", func(ctx context.Context) bool {
			_, err := applyGuardTransformContext(ctx, options, input, raw)
			return err != nil
		}},
		{"transform source comparison", "service.guardSameValue", func(ctx context.Context) bool {
			_, err := applyGuardTransformContext(ctx, options, input, raw)
			return err != nil
		}},
		{"transform clone", "service.guardCloneOptions", func(ctx context.Context) bool {
			_, err := applyGuardTransformContext(ctx, options, input, raw)
			return err != nil
		}},
		{"large token parsing", "service.(*guardContextReader).Read", func(ctx context.Context) bool {
			_, err := guardJSONValueContext(ctx, []byte(`"`+strings.Repeat("x", 16384)+`"`))
			return err != nil
		}},
		{"transform comparison", "service.guardSafeValue", func(ctx context.Context) bool {
			return !guardSafeJSONContext(ctx, []byte(`{"a":[1e10000,2,3]}`), []byte(`{"a":[10e9999,2,3]}`))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newGuardCheckpointContext(t, tc.target, 2)
			assert.True(t, tc.work(ctx), "work must stop at a canceled internal checkpoint")
			require.NotNil(t, ctx.expired, "cancellation checkpoint was not reached")
			assert.ErrorIs(t, ctx.expired.Err(), context.DeadlineExceeded)
			assert.Equal(t, 2, ctx.hits, "no additional synchronous traversal after cancellation")
			assert.Equal(t, options, guardTestOptions(), "source must remain unchanged")
		})
	}
}

func TestGuardRuntime_FinalPhaseExpiry(t *testing.T) {
	for _, open := range []bool{false, true} {
		for _, tc := range []struct {
			name, body, outcome string
			phase               guardPhase
			want                error
		}{
			{"preflight allow", `{"action":"allow"}`, "service_failure", guardPreflight, providerv4.ErrGuardFailed},
			{"postflight allow", `{"action":"allow"}`, "service_failure", guardPostflight, providerv4.ErrGuardFailed},
			{"valid authoritative transform", `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"text","text":"redacted"}]}]}}`, "transform_failure", guardPreflight, providerv4.ErrGuardFailed},
		} {
			t.Run(tc.name+"/open="+strconv.FormatBool(open), func(t *testing.T) {
				settings := guardRuntimeSettings("http://127.0.0.1:1")
				settings.FailOpen = open
				settings.MaxConcurrent = 1
				rt, _ := newTestGuardRuntime(t, settings)
				closed := false
				rt.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &guardClosingBody{Reader: strings.NewReader(tc.body), close: func() { closed = true }}, Request: r}, nil
				})
				release, err := rt.Acquire(context.Background())
				require.NoError(t, err)
				phaseCtx := newGuardCheckpointContext(t, "service.(*GuardRuntime).evaluatePhase.func1", 1)
				phaseCtx.onExpire = func() {
					_, err := rt.Acquire(context.Background())
					assert.ErrorIs(t, err, providerv4.ErrGuardFailed)
				}
				options := guardRuntimeOptions("original-canary")
				var output []provider.ContentPart
				if tc.phase == guardPostflight {
					output = []provider.ContentPart{provider.TextPart("output-canary")}
				}
				effective, err := rt.evaluatePhase(context.Background(), phaseCtx, tc.phase, "canonical", options, output)
				require.NotNil(t, phaseCtx.expired, "final phase check must run before success")
				assert.ErrorIs(t, phaseCtx.expired.Err(), context.DeadlineExceeded)
				outcome := tc.outcome
				if open && tc.outcome == "service_failure" {
					require.NoError(t, err)
					outcome = "fail_open"
					assert.Equal(t, options, effective)
				} else {
					require.ErrorIs(t, err, tc.want)
				}
				assert.True(t, closed)
				assert.Equal(t, float64(1), testutil.ToFloat64(rt.evaluations.WithLabelValues(string(tc.phase), outcome)))
				assert.Zero(t, testutil.ToFloat64(rt.evaluations.WithLabelValues(string(tc.phase), "allow")))
				release()
				release()
				releaseNext, err := rt.Acquire(context.Background())
				require.NoError(t, err)
				releaseNext()
			})
		}
	}
}

func TestGuardRuntime_InterruptedWorkAndAuthority(t *testing.T) {
	options := guardTestOptions()
	input, err := makeGuardInput(options, nil)
	require.NoError(t, err)
	transformed, err := json.Marshal(guardTestResponse(t, input))
	require.NoError(t, err)
	transformBody := `{"action":"allow","transformed_input":` + string(transformed) + `}`
	for _, open := range []bool{false, true} {
		for _, tc := range []struct {
			name, target, body, outcome string
			calls                       int
			want                        error
		}{
			{"input projection", "service.guardJSONToken", `{"action":"allow"}`, "resource_failure", 0, providerv4.ErrGuardFailed},
			{"response decode", "service.decodeGuardRuntimeResponseContext", `{"reason":"optional","action":"allow"}`, "service_failure", 1, providerv4.ErrGuardFailed},
			{"late deny", "service.decodeGuardRuntimeResponseContext", `{"reason":[],"transformed_input":null,"action":"deny"}`, "deny", 1, providerv4.ErrGuardDenied},
			{"late valid transform", "service.decodeGuardRuntimeResponseContext", transformBody, "transform_failure", 1, providerv4.ErrGuardFailed},
			{"late unusable transform", "service.decodeGuardRuntimeResponseContext", `{"action":"allow","transformed_input":null}`, "transform_failure", 1, providerv4.ErrGuardFailed},
			{"transform parse", "service.guardJSONShape", transformBody, "transform_failure", 1, providerv4.ErrGuardFailed},
			{"transform compare", "service.guardSafeValue", transformBody, "transform_failure", 1, providerv4.ErrGuardFailed},
		} {
			for _, cause := range []string{"phase", "parent", "close"} {
				t.Run(tc.name+"/"+cause+"/open="+strconv.FormatBool(open), func(t *testing.T) {
					settings := guardRuntimeSettings("http://127.0.0.1:1")
					settings.FailOpen, settings.MaxConcurrent = open, 1
					rt, _ := newTestGuardRuntime(t, settings)
					parent, cancel := context.WithCancel(context.Background())
					defer cancel()
					phaseCtx := newGuardCheckpointContext(t, tc.target, 2)
					calls, closes := 0, 0
					rt.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
						calls++
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &guardClosingBody{Reader: strings.NewReader(tc.body), close: func() { closes++ }}, Request: r}, nil
					})
					release, err := rt.Acquire(parent)
					require.NoError(t, err)
					phaseCtx.onExpire = func() {
						assert.Equal(t, tc.calls, calls)
						assert.Zero(t, closes)
						_, err := rt.Acquire(context.Background())
						assert.ErrorIs(t, err, providerv4.ErrGuardFailed, "slot cannot be reused during synchronous work")
						switch cause {
						case "parent":
							cancel()
						case "close":
							rt.Close()
						}
					}
					_, err = rt.evaluatePhase(parent, phaseCtx, guardPreflight, "canonical", options, nil)
					require.NotNil(t, phaseCtx.expired)
					outcome, want := tc.outcome, tc.want
					switch cause {
					case "parent":
						outcome, want = "canceled", context.Canceled
					case "close":
						outcome, want = "resource_failure", providerv4.ErrGuardFailed
					default:
						if open && tc.outcome == "service_failure" {
							outcome, want = "fail_open", nil
						}
					}
					if want == nil {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, want)
					}
					assert.Equal(t, tc.calls, calls)
					assert.Equal(t, tc.calls, closes)
					count := float64(1)
					if outcome == "resource_failure" {
						count++
					}
					assert.Equal(t, count, testutil.ToFloat64(rt.evaluations.WithLabelValues("preflight", outcome)))
					assert.Zero(t, testutil.ToFloat64(rt.evaluations.WithLabelValues("preflight", "allow")))
					release()
					release()
					assert.Empty(t, rt.slots)
					if cause != "close" {
						releaseNext, err := rt.Acquire(context.Background())
						require.NoError(t, err)
						releaseNext()
					}
				})
			}
		}
	}
}

type guardExpiringRuntime struct {
	*GuardRuntime
	t      *testing.T
	phase  guardPhase
	expiry *guardCheckpointContext
}

func (guard *guardExpiringRuntime) Preflight(ctx context.Context, id string, options provider.CallOptions) (provider.CallOptions, error) {
	if guard.phase != guardPreflight {
		return guard.GuardRuntime.Preflight(ctx, id, options)
	}
	guard.expiry = newGuardCheckpointContext(guard.t, "service.(*GuardRuntime).evaluatePhase.func1", 1)
	guard.expiry.Context = ctx
	return guard.evaluatePhase(ctx, guard.expiry, guardPreflight, id, options, nil)
}

func (guard *guardExpiringRuntime) Postflight(ctx context.Context, id string, options provider.CallOptions, output []provider.ContentPart) error {
	if guard.phase != guardPostflight {
		return guard.GuardRuntime.Postflight(ctx, id, options, output)
	}
	guard.expiry = newGuardCheckpointContext(guard.t, "service.(*GuardRuntime).evaluatePhase.func1", 1)
	guard.expiry.Context = ctx
	_, err := guard.evaluatePhase(ctx, guard.expiry, guardPostflight, id, options, output)
	return err
}

func TestGuardRuntime_ExpiredPhaseWithholdsHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, open := range []bool{false, true} {
			for _, phase := range []guardPhase{guardPreflight, guardPostflight} {
				for _, transform := range []bool{false, true} {
					if transform && phase == guardPostflight {
						continue
					}
					t.Run(string(phase)+"/stream="+strconv.FormatBool(stream)+"/open="+strconv.FormatBool(open)+"/transform="+strconv.FormatBool(transform), func(t *testing.T) {
						settings := guardRuntimeSettings("http://127.0.0.1:1")
						settings.FailOpen, settings.MaxConcurrent = open, 1
						rt, _ := newTestGuardRuntime(t, settings)
						guard := &guardExpiringRuntime{GuardRuntime: rt, t: t, phase: phase}
						closes, hooks, providers := 0, 0, 0
						rt.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
							hooks++
							body := `{"action":"allow"}`
							if transform {
								body = `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"text","text":"redacted"}]}]}}`
							}
							return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &guardClosingBody{Reader: strings.NewReader(body), close: func() { closes++ }}, Request: r}, nil
						})
						finish := provider.FinishReason{Unified: provider.FinishReasonStop}
						model := &observabilityTestModel{
							generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
								providers++
								return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "withheld-output-canary"}}, FinishReason: finish}, nil
							},
							stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
								providers++
								parts := make(chan provider.StreamPart, 4)
								parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}
								parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "withheld-output-canary"}
								parts <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}
								parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{}}
								close(parts)
								return &provider.StreamResult{Stream: parts}, nil
							},
						}
						handler, err := providerv4.New(providerv4.Config{Resolver: guardRecordingResolver{model}, Guard: guard, GuardRetainedBytes: settings.RetainedBytes / 2, Limits: providerv4.Limits{RequestBytes: 1 << 20, UnaryResponseBytes: 1 << 20, StreamFrameBytes: 1 << 20, StreamParts: 100, ModelDuration: time.Minute, StreamIdleDuration: time.Second, StreamDrainDuration: 10 * time.Millisecond}})
						require.NoError(t, err)
						request := guardRecordingRequest()
						request.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(stream))
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, request)
						require.NotNil(t, guard.expiry)
						require.NotNil(t, guard.expiry.expired)
						assert.ErrorIs(t, guard.expiry.expired.Err(), context.DeadlineExceeded)
						if open && !transform {
							assert.Equal(t, http.StatusOK, response.Code)
							assert.Contains(t, response.Body.String(), "withheld-output-canary")
							assert.Equal(t, 1, providers)
						} else {
							assert.Equal(t, http.StatusFailedDependency, response.Code)
							assert.JSONEq(t, `{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`, response.Body.String())
							assert.NotContains(t, response.Body.String(), "data: ")
							assert.NotContains(t, response.Body.String(), "withheld-output-canary")
							if phase == guardPreflight {
								assert.Zero(t, providers)
							} else {
								assert.Equal(t, 1, providers)
							}
						}
						assert.Equal(t, hooks, closes)
						assert.Empty(t, rt.slots)
						assert.NotContains(t, response.Body.String(), "prompt-guard-canary")
					})
				}
			}
		}
	}
}

type guardClosingBody struct {
	io.Reader
	close func()
}

func (body *guardClosingBody) Close() error { body.close(); return nil }
