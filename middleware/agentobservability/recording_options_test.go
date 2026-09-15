package agentobservability

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordingMiddleware_IdentitySource(t *testing.T) {
	for _, identity := range []IdentitySource{"", IdentityPreferResponse, IdentityRequested} {
		for _, response := range []provider.ResponseMetadata{
			{}, {ID: "private-response", Provider: "private-provider", ModelID: "private-model"},
			{ID: "private-response", Provider: "private-provider"},
			{ID: "private-response", ModelID: "private-model"},
		} {
			for _, streaming := range []bool{false, true} {
				t.Run(string(identity)+"/"+response.Provider+"/"+response.ModelID+"/"+map[bool]string{true: "stream", false: "generate"}[streaming], func(t *testing.T) {
					env := testkit.NewEnv(t)
					original := &provider.GenerateResult{Response: &provider.GenerateResponse{ResponseMetadata: response}}
					request := &provider.RequestMetadata{Body: json.RawMessage(`{"private":"request"}`)}
					headers := &provider.ResponseHeaders{Headers: map[string]string{"private": "header"}}
					part := provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: response.ID, Provider: response.Provider, ModelID: response.ModelID}
					model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
						doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return original, nil },
						doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
							ch := make(chan provider.StreamPart, 1)
							ch <- part
							close(ch)
							return &provider.StreamResult{Stream: ch, Request: request, Response: headers}, nil
						},
					}
					opts := RecordingOptions{IdentitySource: identity,
						ClientResolver:  func(context.Context) *agento11y.Client { return env.Client },
						ContextProvider: func(context.Context) ContextInfo { return ContextInfo{} },
					}
					if streaming {
						result, err := streamWith(t, model, opts)
						require.NoError(t, err)
						assert.Same(t, request, result.Request)
						assert.Same(t, headers, result.Response)
						var got []provider.StreamPart
						for p := range result.Stream {
							got = append(got, p)
						}
						assert.Equal(t, []provider.StreamPart{part}, got)
						awaitRecordedGeneration(t, env)
					} else {
						result, err := generateWith(t, model, opts)
						require.NoError(t, err)
						assert.Same(t, original, result)
						assert.Equal(t, response, original.Response.ResponseMetadata)
					}
					require.NoError(t, env.Client.Shutdown(context.Background()))
					gen := env.SingleGenerationJSON(t)
					wantProvider, wantModel := "grafana", "grafana/assistant"
					if identity != IdentityRequested && response.Provider != "" && response.ModelID != "" {
						wantProvider, wantModel = response.Provider, response.ModelID
					}
					assert.Equal(t, wantProvider, testkit.StringValue(t, gen, "model", "provider"))
					assert.Equal(t, wantModel, testkit.StringValue(t, gen, "model", "name"))
					if identity == IdentityRequested {
						encoded, err := json.Marshal(gen)
						require.NoError(t, err)
						for _, forbidden := range []string{"private-provider", "private-model", "private-response", transportProviderMetadataKey, transportModelMetadataKey, "response_model", "response_id"} {
							assert.NotContains(t, string(encoded), forbidden)
						}
						spans := env.Spans.Ended()
						require.Len(t, spans, 1)
						attrs := spanAttributes(spans[0])
						assert.Equal(t, "grafana", attrs["gen_ai.provider.name"])
						assert.Equal(t, "grafana/assistant", attrs["gen_ai.request.model"])
						assert.NotContains(t, attrs, "gen_ai.response.model")
						assert.NotContains(t, attrs, "gen_ai.response.id")
					}
				})
			}
		}
	}
}

func TestRecordingMiddleware_NilSuccessfulResult(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
			env := testkit.NewEnv(t)
			model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, nil },
				doStream:   func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, nil },
			}
			opts := RecordingOptions{IdentitySource: IdentityRequested,
				ClientResolver:  func(context.Context) *agento11y.Client { return env.Client },
				ContextProvider: func(context.Context) ContextInfo { return ContextInfo{} },
			}
			if streaming {
				result, err := streamWith(t, model, opts)
				require.NoError(t, err)
				assert.Nil(t, result)
			} else {
				result, err := generateWith(t, model, opts)
				require.NoError(t, err)
				assert.Nil(t, result)
			}
			require.NoError(t, env.Client.Shutdown(context.Background()))
			testkit.RequireRequestCount(t, env, 1)
			gen := env.SingleGenerationJSON(t)
			assert.Equal(t, "grafana/assistant", testkit.StringValue(t, gen, "model", "name"))
			assert.NotContains(t, gen, "call_error")
		})
	}
}

func TestDrainStreamUntil(t *testing.T) {
	for _, scenario := range []string{"closed", "silent", "ready", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := make(chan provider.StreamPart, 1)
			stop := make(chan struct{})
			producerDone := make(chan struct{})
			if scenario == "closed" {
				close(upstream)
			}
			if scenario == "ready" {
				go func() {
					defer close(producerDone)
					for {
						select {
						case upstream <- provider.StreamPart{}:
						case <-stop:
							return
						}
					}
				}()
				t.Cleanup(func() { close(stop); <-producerDone })
			}
			deadline := time.Now().Add(25 * time.Millisecond)
			if scenario == "expired" {
				deadline = time.Now().Add(-time.Second)
				upstream <- provider.StreamPart{}
			}
			done := make(chan struct{})
			go func() { drainStreamUntil(upstream, deadline); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("bounded drain did not exit")
			}
			if scenario == "silent" || scenario == "ready" {
				assert.False(t, time.Now().Before(deadline))
			}
			if scenario == "expired" {
				assert.Len(t, upstream, 1)
			}
		})
	}
}

func TestRecordingMiddleware_BoundedDrain(t *testing.T) {
	for _, scenario := range []string{"silent", "full-output", "already-canceled", "deadline", "close-race"} {
		t.Run(scenario, func(t *testing.T) {
			env := testkit.NewEnv(t)
			upstream := make(chan provider.StreamPart)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer deadlineCancel()
			}
			if scenario == "already-canceled" {
				cancel()
			}
			model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
				doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: upstream}, nil
				},
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
				ClientResolver:     func(context.Context) *agento11y.Client { return env.Client },
				ContextProvider:    func(context.Context) ContextInfo { return ContextInfo{} },
				StreamDrainTimeout: 2 * time.Second,
			})}})
			result, err := wrapped.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			if scenario == "full-output" {
				for i := 0; i <= streamRecordingBuffer; i++ {
					select {
					case upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}:
					case <-time.After(time.Second):
						t.Fatal("tee failed to fill")
					}
				}
			}
			if scenario != "deadline" {
				cancel()
			}
			if scenario == "close-race" {
				close(upstream)
			}
			closed := make(chan struct{})
			go func() {
				for range result.Stream {
				}
				close(closed)
			}()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("tee did not close")
			}
			awaitRecordedGeneration(t, env)
			if scenario != "close-race" {
				close(upstream)
			}
			require.NoError(t, env.Client.Shutdown(context.Background()))
			testkit.RequireRequestCount(t, env, 1)
			gen := env.SingleGenerationJSON(t)
			assert.Contains(t, gen["call_error"], ctx.Err().Error())
		})
	}
}

func TestRecordingMiddleware_BoundedDrainDiscardsLateParts(t *testing.T) {
	env := testkit.NewEnv(t)
	upstream := make(chan provider.StreamPart)
	defer close(upstream)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
		ClientResolver:     func(context.Context) *agento11y.Client { return env.Client },
		ContextProvider:    func(context.Context) ContextInfo { return ContextInfo{} },
		StreamDrainTimeout: 2 * time.Second,
	})}})
	result, err := wrapped.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	cancel()
	select {
	case _, ok := <-result.Stream:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("finalization waited for the drain")
	}
	count := 999
	select {
	case upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "late-private-output", Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &count}}}:
	case <-time.After(time.Second):
		t.Fatal("opt-in drain did not consume late upstream part")
	}
	awaitRecordedGeneration(t, env)
	require.NoError(t, env.Client.Shutdown(context.Background()))
	gen := env.SingleGenerationJSON(t)
	assert.NotContains(t, gen, "output")
	usage, _ := gen["usage"].(map[string]any)
	assert.NotContains(t, usage, "input_tokens")
}

func TestRecordingMiddleware_BoundedDrainContinuouslyReadyCancellation(t *testing.T) {
	env := testkit.NewEnv(t)
	upstream := make(chan provider.StreamPart, 64)
	stop, producerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(upstream)
		for {
			select {
			case upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}:
			case <-stop:
				return
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-producerDone })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
		ClientResolver:     func(context.Context) *agento11y.Client { return env.Client },
		ContextProvider:    func(context.Context) ContextInfo { return ContextInfo{} },
		StreamDrainTimeout: 25 * time.Millisecond,
	})}})
	result, err := wrapped.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	select {
	case <-result.Stream:
	case <-time.After(time.Second):
		t.Fatal("no initial output")
	}
	cancel()
	closed := make(chan struct{})
	go func() {
		for range result.Stream {
		}
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("ready upstream retained tee")
	}
	awaitRecordedGeneration(t, env)
	require.NoError(t, env.Client.Shutdown(context.Background()))
	testkit.RequireRequestCount(t, env, 1)
}

func TestRecordingMiddleware_RequestedStreamLifecycle(t *testing.T) {
	env := testkit.NewEnv(t)
	input, output, weaker := 12, 7, 1
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "provider failure", StatusCode: 503})
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, Provider: "private-provider", ModelID: "private-model", ResponseID: "private-response", Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &input}}},
		{Type: provider.PartError, APICallError: apiErr, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &output}}},
		{Type: provider.PartTextDelta, Delta: "later text"},
		{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &weaker}, OutputTokens: provider.OutputTokenUsage{Total: &weaker}}},
	}
	model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, len(parts))
			for _, part := range parts {
				ch <- part
			}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}
	result, err := streamWith(t, model, RecordingOptions{
		IdentitySource: IdentityRequested, StreamDrainTimeout: 25 * time.Millisecond,
		ClientResolver:  func(context.Context) *agento11y.Client { return env.Client },
		ContextProvider: func(context.Context) ContextInfo { return ContextInfo{} },
	})
	require.NoError(t, err)
	var received []provider.StreamPart
	for part := range result.Stream {
		received = append(received, part)
	}
	assert.Equal(t, parts, received)
	assert.Same(t, apiErr, received[1].APICallError)
	assert.Equal(t, 1, model.streamHit)
	awaitRecordedGeneration(t, env)
	require.NoError(t, env.Client.Shutdown(context.Background()))
	gen := env.SingleGenerationJSON(t)
	usage, ok := gen["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "12", usage["input_tokens"])
	assert.Equal(t, "7", usage["output_tokens"])
	assert.Equal(t, "19", usage["total_tokens"])
	assert.Contains(t, gen["call_error"], "provider failure")
	assert.Equal(t, "grafana/assistant", testkit.StringValue(t, gen, "model", "name"))
	assert.NotContains(t, gen, "response_id")
	assert.NotContains(t, gen, "response_model")
}

func awaitRecordedGeneration(t *testing.T, env *testkit.Env) {
	t.Helper()
	require.Eventually(t, func() bool {
		return env.RequestCount() == 1 && len(env.Spans.Ended()) == 1
	}, time.Second, time.Millisecond)
}
