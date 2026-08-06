package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runtimeTestModel struct {
	providerID    string
	modelID       string
	generate      func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream        func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
	generateCalls atomic.Int32
	streamCalls   atomic.Int32
}

func (model *runtimeTestModel) SpecificationVersion() string { return "v4" }
func (model *runtimeTestModel) Provider() string             { return model.providerID }
func (model *runtimeTestModel) ModelID() string              { return model.modelID }
func (model *runtimeTestModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}
func (model *runtimeTestModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	model.generateCalls.Add(1)
	if model.generate == nil {
		return &provider.GenerateResult{}, nil
	}
	return model.generate(ctx, options)
}
func (model *runtimeTestModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	model.streamCalls.Add(1)
	if model.stream == nil {
		parts := make(chan provider.StreamPart)
		close(parts)
		return &provider.StreamResult{Stream: parts}, nil
	}
	return model.stream(ctx, options)
}

func TestNew_ValidationAndDefaults(t *testing.T) {
	resolver := ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{}, nil
	})
	runtime, err := New(resolver)
	require.NoError(t, err)
	assert.Equal(t, DefaultTotalTimeout, runtime.totalTimeout)

	cases := []struct {
		name     string
		resolver ModelResolver
		options  []Option
	}{
		{name: "nil resolver"},
		{name: "typed nil resolver", resolver: ModelResolverFunc(nil)},
		{name: "nil option", resolver: resolver, options: []Option{nil}},
		{name: "zero timeout", resolver: resolver, options: []Option{WithTotalTimeout(0)}},
		{name: "negative timeout", resolver: resolver, options: []Option{WithTotalTimeout(-time.Second)}},
		{name: "nil policy", resolver: resolver, options: []Option{WithCallPolicies(CallPolicyFunc(nil))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := New(tc.resolver, tc.options...)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestGenerate_IdentityPolicyMiddlewareAndContext(t *testing.T) {
	model := &runtimeTestModel{providerID: "provider-a", modelID: "backend-a"}
	model.generate = func(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		assert.Equal(t, "normalized", options.Headers["X-Policy"])
		assertContextValues(t, ctx)
		attributes := AuthenticatedAttributesFromContext(ctx)
		attributes["tenant"] = "mutated"
		assert.Equal(t, "tenant-a", AuthenticatedAttributesFromContext(ctx)["tenant"])
		metadata := PolicyMetadataFromContext(ctx)
		metadata["decision"][1] = 'X'
		assert.JSONEq(t, `"allowed"`, string(PolicyMetadataFromContext(ctx)["decision"]))
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "ok"}}}, nil
	}

	var sequence []string
	policy := CallPolicyFunc(func(_ context.Context, call GatewayCall) (GatewayCall, error) {
		sequence = append(sequence, "policy")
		call.CallOptions.Headers = map[string]string{"X-Policy": "normalized"}
		call.PolicyMetadata = PolicyMetadata{"decision": []byte(`"allowed"`)}
		return call, nil
	})
	resolver := ModelResolverFunc(func(_ context.Context, call GatewayCall) (catalog.ResolvedModel, error) {
		sequence = append(sequence, "resolve")
		assert.Equal(t, "normalized", call.CallOptions.Headers["X-Policy"])
		return catalog.ResolvedModel{ID: "canonical-a", Model: model}, nil
	})
	middlewareEntries := 0
	mw := middleware.Middleware{TransformParams: func(ctx context.Context, input middleware.TransformParamsInput) (provider.CallOptions, error) {
		sequence = append(sequence, "middleware")
		middlewareEntries++
		assertContextValues(t, ctx)
		return input.Params, nil
	}}
	runtime, err := New(resolver, WithCallPolicies(policy), WithMiddleware(mw))
	require.NoError(t, err)

	outcome := runtime.Generate(context.Background(), validTestCall())
	require.Nil(t, outcome.Failure)
	require.NotNil(t, outcome.Result)
	assert.Equal(t, Identity{
		RequestedModelID: "model", CanonicalModelID: "canonical-a",
		ResolvedProviderID: "provider-a", ResolvedModelID: "backend-a",
	}, outcome.Identity)
	assert.Equal(t, []string{"policy", "resolve", "middleware"}, sequence)
	assert.Equal(t, 1, middlewareEntries)
	assert.Equal(t, int32(1), model.generateCalls.Load())
}

func TestRuntime_MiddlewareOrderIsPreserved(t *testing.T) {
	var generateOrder []string
	model := &runtimeTestModel{providerID: "p", modelID: "m", generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		generateOrder = append(generateOrder, "model")
		return &provider.GenerateResult{}, nil
	}}
	middlewares := make([]middleware.Middleware, 0, 3)
	for _, name := range []string{"A", "B", "C"} {
		name := name
		middlewares = append(middlewares, middleware.Middleware{TransformParams: func(_ context.Context, input middleware.TransformParamsInput) (provider.CallOptions, error) {
			generateOrder = append(generateOrder, name)
			return input.Params, nil
		}})
	}
	runtime := newRuntimeForModel(t, model, WithMiddleware(middlewares...))
	outcome := runtime.Generate(context.Background(), validTestCall())
	require.Nil(t, outcome.Failure)
	assert.Equal(t, []string{"A", "B", "C", "model"}, generateOrder)
}

func TestGenerate_FailuresRetainAvailableIdentity(t *testing.T) {
	providerCause := errors.New("provider private cause")
	model := &runtimeTestModel{providerID: "provider-a", modelID: "backend-a", generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 502, Cause: providerCause})
	}}
	resolver := ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{ID: "canonical-a", Model: model}, nil
	})
	runtime, err := New(resolver)
	require.NoError(t, err)

	outcome := runtime.Generate(context.Background(), validTestCall())
	require.NotNil(t, outcome.Failure)
	assert.Equal(t, failure.KindFailedDependency, outcome.Failure.Kind)
	assert.True(t, outcome.Failure.Retryable)
	assert.ErrorIs(t, outcome.Failure.Cause, providerCause)
	assert.Equal(t, "canonical-a", outcome.Identity.CanonicalModelID)
	assert.Equal(t, "backend-a", outcome.Identity.ResolvedModelID)

	model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, nil }
	outcome = runtime.Generate(context.Background(), validTestCall())
	require.NotNil(t, outcome.Failure)
	assert.Equal(t, failure.KindInternal, outcome.Failure.Kind)
	assert.False(t, outcome.Failure.Retryable)
	assert.Equal(t, "canonical-a", outcome.Identity.CanonicalModelID)
}

func TestGenerate_TypedNilProviderErrorFailsClosed(t *testing.T) {
	var apiErr *provider.APICallError
	model := &runtimeTestModel{providerID: "provider-a", modelID: "backend-a", generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, apiErr
	}}
	outcome := newRuntimeForModel(t, model).Generate(context.Background(), validTestCall())
	require.NotNil(t, outcome.Failure)
	assert.Equal(t, failure.KindInternal, outcome.Failure.Kind)
	assert.False(t, outcome.Failure.Retryable)
	assert.Nil(t, outcome.Failure.Cause)
}

func TestGenerate_ResolutionFailures(t *testing.T) {
	t.Run("policy bypasses resolution", func(t *testing.T) {
		resolverCalls := 0
		policyCause := errors.New("private policy cause")
		runtime, err := New(
			ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
				resolverCalls++
				return catalog.ResolvedModel{}, nil
			}),
			WithCallPolicies(CallPolicyFunc(func(context.Context, GatewayCall) (GatewayCall, error) {
				return GatewayCall{}, failure.Wrap(failure.ErrForbidden, policyCause)
			})),
		)
		require.NoError(t, err)
		outcome := runtime.Generate(context.Background(), validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindForbidden, outcome.Failure.Kind)
		assert.ErrorIs(t, outcome.Failure.Cause, policyCause)
		assert.Equal(t, Identity{RequestedModelID: "model"}, outcome.Identity)
		assert.Zero(t, resolverCalls)
	})

	t.Run("unknown model", func(t *testing.T) {
		runtime, err := New(ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
			return catalog.ResolvedModel{}, &catalog.UnknownModelError{ModelID: "model"}
		}))
		require.NoError(t, err)
		outcome := runtime.Generate(context.Background(), validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindUnknownModel, outcome.Failure.Kind)
		assert.Equal(t, "model", outcome.Failure.SafeParameters.RequestedModelID)
	})

	t.Run("nil model retains canonical ID", func(t *testing.T) {
		runtime, err := New(ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
			return catalog.ResolvedModel{ID: "canonical"}, nil
		}))
		require.NoError(t, err)
		outcome := runtime.Generate(context.Background(), validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindInternal, outcome.Failure.Kind)
		assert.Equal(t, "canonical", outcome.Identity.CanonicalModelID)
	})

	t.Run("missing canonical ID retains model-reported identity", func(t *testing.T) {
		model := &runtimeTestModel{providerID: "provider-a", modelID: "backend-a"}
		runtime, err := New(ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
			return catalog.ResolvedModel{Model: model}, nil
		}))
		require.NoError(t, err)
		outcome := runtime.Generate(context.Background(), validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindInternal, outcome.Failure.Kind)
		assert.Equal(t, "provider-a", outcome.Identity.ResolvedProviderID)
		assert.Equal(t, "backend-a", outcome.Identity.ResolvedModelID)
	})
}

func TestGenerate_PreparationContextTerminationPrecedence(t *testing.T) {
	type preparationStage string
	const (
		policyStage   preparationStage = "policy"
		resolverStage preparationStage = "resolver"
	)

	cases := []struct {
		name       string
		stage      preparationStage
		newContext func() (context.Context, context.CancelFunc, error)
		dependency bool
		wantKind   failure.Kind
	}{
		{
			name:  "policy custom cancellation overrides internal",
			stage: policyStage,
			newContext: func() (context.Context, context.CancelFunc, error) {
				private := errors.New("private policy cancellation")
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(private)
				return ctx, func() {}, private
			},
			wantKind: failure.KindCanceled,
		},
		{
			name:       "policy expired deadline overrides failed dependency",
			stage:      policyStage,
			dependency: true,
			newContext: func() (context.Context, context.CancelFunc, error) {
				private := errors.New("private policy deadline")
				ctx, cancel := context.WithDeadlineCause(context.Background(), time.Now().Add(-time.Second), private)
				return ctx, cancel, private
			},
			wantKind: failure.KindTimeout,
		},
		{
			name:       "resolver custom cancellation overrides failed dependency",
			stage:      resolverStage,
			dependency: true,
			newContext: func() (context.Context, context.CancelFunc, error) {
				private := errors.New("private resolver cancellation")
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(private)
				return ctx, func() {}, private
			},
			wantKind: failure.KindCanceled,
		},
		{
			name:  "resolver expired deadline overrides internal",
			stage: resolverStage,
			newContext: func() (context.Context, context.CancelFunc, error) {
				private := errors.New("private resolver deadline")
				ctx, cancel := context.WithDeadlineCause(context.Background(), time.Now().Add(-time.Second), private)
				return ctx, cancel, private
			},
			wantKind: failure.KindTimeout,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			returnedCause := errors.New("private preparation failure")
			returnedErr := error(returnedCause)
			if tc.dependency {
				returnedErr = provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 502, Cause: returnedCause})
			}
			resolver := ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
				if tc.stage == resolverStage {
					return catalog.ResolvedModel{}, returnedErr
				}
				return catalog.ResolvedModel{ID: "canonical", Model: &runtimeTestModel{}}, nil
			})
			options := []Option{}
			if tc.stage == policyStage {
				options = append(options, WithCallPolicies(CallPolicyFunc(func(context.Context, GatewayCall) (GatewayCall, error) {
					return GatewayCall{}, returnedErr
				})))
			}
			gatewayRuntime, err := New(resolver, options...)
			require.NoError(t, err)
			ctx, cancel, contextCause := tc.newContext()
			defer cancel()

			outcome := gatewayRuntime.Generate(ctx, validTestCall())
			require.NotNil(t, outcome.Failure)
			assert.Equal(t, tc.wantKind, outcome.Failure.Kind)
			assert.Equal(t, tc.wantKind == failure.KindTimeout, outcome.Failure.Retryable)
			assert.ErrorIs(t, outcome.Failure.Cause, returnedCause)
			assert.ErrorIs(t, outcome.Failure.Cause, contextCause)
		})
	}
}

func TestGenerate_PreparationExplicitCategoriesSurviveCancellation(t *testing.T) {
	private := errors.New("private explicit failure")
	cases := []struct {
		name     string
		policy   error
		resolver error
		wantKind failure.Kind
	}{
		{name: "unauthenticated policy", policy: failure.Wrap(failure.ErrUnauthenticated, private), wantKind: failure.KindUnauthenticated},
		{name: "invalid call policy", policy: failure.Wrap(failure.ErrInvalidCall, private), wantKind: failure.KindInvalidCall},
		{name: "forbidden policy", policy: failure.Wrap(failure.ErrForbidden, private), wantKind: failure.KindForbidden},
		{name: "rate limited policy", policy: failure.Wrap(failure.ErrRateLimited, private), wantKind: failure.KindRateLimited},
		{name: "unknown model resolver", resolver: &catalog.UnknownModelError{ModelID: "model"}, wantKind: failure.KindUnknownModel},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
				if tc.resolver != nil {
					return catalog.ResolvedModel{}, tc.resolver
				}
				return catalog.ResolvedModel{ID: "canonical", Model: &runtimeTestModel{}}, nil
			})
			options := []Option{}
			if tc.policy != nil {
				options = append(options, WithCallPolicies(CallPolicyFunc(func(context.Context, GatewayCall) (GatewayCall, error) {
					return GatewayCall{}, tc.policy
				})))
			}
			gatewayRuntime, err := New(resolver, options...)
			require.NoError(t, err)
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(errors.New("private caller cancellation"))

			outcome := gatewayRuntime.Generate(ctx, validTestCall())
			require.NotNil(t, outcome.Failure)
			assert.Equal(t, tc.wantKind, outcome.Failure.Kind)
			if tc.policy != nil {
				assert.ErrorIs(t, outcome.Failure.Cause, private)
			}
		})
	}
}

func TestInvocationFailure_PreservesProviderCleanupAndCustomContextCauses(t *testing.T) {
	type operation string
	const (
		generateOperation operation = "generate"
		streamOperation   operation = "stream setup"
	)
	cases := []struct {
		name       string
		operation  operation
		newContext func(error) (context.Context, context.CancelFunc)
		wantKind   failure.Kind
	}{
		{name: "generate cancellation", operation: generateOperation, newContext: canceledContextWithCause, wantKind: failure.KindCanceled},
		{name: "generate deadline", operation: generateOperation, newContext: deadlineContextWithCause, wantKind: failure.KindTimeout},
		{name: "stream setup cancellation", operation: streamOperation, newContext: canceledContextWithCause, wantKind: failure.KindCanceled},
		{name: "stream setup deadline", operation: streamOperation, newContext: deadlineContextWithCause, wantKind: failure.KindTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanupErr := errors.New("private provider cleanup failure")
			contextCause := errors.New("private custom context cause")
			ctx, cancel := tc.newContext(contextCause)
			defer cancel()
			model := &runtimeTestModel{providerID: "p", modelID: "m"}
			model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, cleanupErr
			}
			model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return nil, cleanupErr
			}
			gatewayRuntime := newRuntimeForModel(t, model)

			var classification *failure.Classification
			if tc.operation == generateOperation {
				classification = gatewayRuntime.Generate(ctx, validTestCall()).Failure
			} else {
				classification = gatewayRuntime.Stream(ctx, validTestCall()).Failure
			}
			require.NotNil(t, classification)
			assert.Equal(t, tc.wantKind, classification.Kind)
			assert.ErrorIs(t, classification.Cause, cleanupErr)
			assert.ErrorIs(t, classification.Cause, contextCause)
		})
	}
}

func canceledContextWithCause(cause error) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	return ctx, func() {}
}

func deadlineContextWithCause(cause error) (context.Context, context.CancelFunc) {
	return context.WithDeadlineCause(context.Background(), time.Now().Add(-time.Second), cause)
}

func TestGenerate_TimeoutCancellationAndBlockedProvider(t *testing.T) {
	t.Run("cooperative timeout", func(t *testing.T) {
		model := &runtimeTestModel{providerID: "p", modelID: "m", generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		runtime := newRuntimeForModel(t, model, WithTotalTimeout(10*time.Millisecond))
		outcome := runtime.Generate(context.Background(), validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindTimeout, outcome.Failure.Kind)
		assert.True(t, outcome.Failure.Retryable)
	})

	t.Run("caller cancellation", func(t *testing.T) {
		model := &runtimeTestModel{providerID: "p", modelID: "m", generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		runtime := newRuntimeForModel(t, model)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		outcome := runtime.Generate(ctx, validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindCanceled, outcome.Failure.Kind)
	})

	t.Run("late success after timeout", func(t *testing.T) {
		model := &runtimeTestModel{providerID: "p", modelID: "m", generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return &provider.GenerateResult{}, nil
		}}
		runtime := newRuntimeForModel(t, model, WithTotalTimeout(10*time.Millisecond))
		outcome := runtime.Generate(context.Background(), validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Nil(t, outcome.Result)
		assert.Equal(t, failure.KindTimeout, outcome.Failure.Kind)
	})

	t.Run("late success after parent cancellation", func(t *testing.T) {
		model := &runtimeTestModel{providerID: "p", modelID: "m", generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return &provider.GenerateResult{}, nil
		}}
		runtime := newRuntimeForModel(t, model)
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errors.New("caller stopped"))
		outcome := runtime.Generate(ctx, validTestCall())
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindCanceled, outcome.Failure.Kind)
	})

	t.Run("blocked provider receives deadline without runtime invocation goroutine", func(t *testing.T) {
		contextDone := make(chan struct{})
		release := make(chan struct{})
		model := &runtimeTestModel{providerID: "p", modelID: "m", generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			close(contextDone)
			<-release
			return nil, ctx.Err()
		}}
		runtime := newRuntimeForModel(t, model, WithTotalTimeout(10*time.Millisecond))
		returned := make(chan GenerateOutcome, 1)
		go func() { returned <- runtime.Generate(context.Background(), validTestCall()) }()
		require.Eventually(t, func() bool {
			select {
			case <-contextDone:
				return true
			default:
				return false
			}
		}, time.Second, time.Millisecond)
		select {
		case <-returned:
			t.Fatal("Generate returned before the synchronous provider returned")
		default:
		}
		close(release)
		outcome := <-returned
		require.NotNil(t, outcome.Failure)
		assert.Equal(t, failure.KindTimeout, outcome.Failure.Kind)
	})
}

func TestStream_OrderedDataAndCleanCompletion(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "data error"})},
		{Type: provider.PartTextDelta, ID: "text", Delta: "after"},
		{Type: provider.PartFinish},
	}
	model := &runtimeTestModel{providerID: "provider-a", modelID: "backend-a", stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		assertContextValues(t, ctx)
		source := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			source <- part
		}
		close(source)
		return &provider.StreamResult{Stream: source}, nil
	}}
	entries := 0
	runtime := newRuntimeForModel(t, model, WithMiddleware(middleware.Middleware{TransformParams: func(ctx context.Context, input middleware.TransformParamsInput) (provider.CallOptions, error) {
		entries++
		assertContextValues(t, ctx)
		return input.Params, nil
	}}))

	outcome := runtime.Stream(context.Background(), validTestCall())
	require.Nil(t, outcome.Failure)
	require.NotNil(t, outcome.Invocation)
	assert.Equal(t, outcome.Identity, outcome.Invocation.Identity())
	var got []provider.StreamPart
	for part := range outcome.Invocation.Parts() {
		got = append(got, part)
	}
	assert.Equal(t, parts, got)
	assert.NoError(t, outcome.Invocation.Wait())
	assert.Equal(t, 1, entries)
	assert.Equal(t, int32(1), model.streamCalls.Load())
}

func TestStream_PostPolicyRawFiltering(t *testing.T) {
	source := make(chan provider.StreamPart, 2)
	source <- provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{"private":true}`)}
	source <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "public"}
	close(source)
	model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
		assert.False(t, options.IncludeRawChunks)
		return &provider.StreamResult{Stream: source}, nil
	}}
	policy := CallPolicyFunc(func(_ context.Context, call GatewayCall) (GatewayCall, error) {
		call.CallOptions.IncludeRawChunks = false
		return call, nil
	})
	runtime := newRuntimeForModel(t, model, WithCallPolicies(policy))
	call := validTestCall()
	call.CallOptions.IncludeRawChunks = true
	outcome := runtime.Stream(context.Background(), call)
	require.Nil(t, outcome.Failure)
	parts := make([]provider.StreamPart, 0)
	for part := range outcome.Invocation.Parts() {
		parts = append(parts, part)
	}
	require.Len(t, parts, 1)
	assert.Equal(t, provider.PartTextDelta, parts[0].Type)
}

func TestStream_SetupFailuresRetainIdentity(t *testing.T) {
	setupCause := errors.New("setup")
	cases := []struct {
		name   string
		stream func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
	}{
		{name: "error", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, setupCause }},
		{name: "nil result", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, nil }},
		{name: "nil channel", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{}, nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &runtimeTestModel{providerID: "provider-a", modelID: "backend-a", stream: tc.stream}
			runtime := newRuntimeForModel(t, model)
			outcome := runtime.Stream(context.Background(), validTestCall())
			require.NotNil(t, outcome.Failure)
			assert.Nil(t, outcome.Invocation)
			assert.Equal(t, "canonical", outcome.Identity.CanonicalModelID)
			assert.Equal(t, "backend-a", outcome.Identity.ResolvedModelID)
		})
	}
}

func TestStream_SetupErrorClassificationPrecedesCleanupCancellation(t *testing.T) {
	retryable := true
	providerError := func(status int) error {
		return provider.NewAPICallError(provider.APICallErrorOptions{Message: "setup failed", StatusCode: status, IsRetryable: &retryable})
	}
	cases := []struct {
		name     string
		ctx      func() context.Context
		err      error
		wantKind failure.Kind
	}{
		{name: "rate limit", ctx: context.Background, err: providerError(429), wantKind: failure.KindRateLimited},
		{name: "dependency", ctx: context.Background, err: providerError(400), wantKind: failure.KindFailedDependency},
		{name: "caller cancellation wins", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, err: providerError(429), wantKind: failure.KindCanceled},
		{name: "caller deadline wins", ctx: func() context.Context {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			cancel()
			return ctx
		}, err: providerError(429), wantKind: failure.KindTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return nil, tc.err
			}}
			runtime := newRuntimeForModel(t, model)
			outcome := runtime.Stream(tc.ctx(), validTestCall())
			require.NotNil(t, outcome.Failure)
			assert.Equal(t, tc.wantKind, outcome.Failure.Kind)
		})
	}
}

func TestStream_CancellationTimeoutAndBackpressure(t *testing.T) {
	t.Run("adapter cancellation", func(t *testing.T) {
		source := make(chan provider.StreamPart)
		model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: source}, nil
		}}
		runtime := newRuntimeForModel(t, model)
		outcome := runtime.Stream(context.Background(), validTestCall())
		writeErr := errors.New("write failed")
		outcome.Invocation.Cancel(writeErr)
		outcome.Invocation.Cancel(errors.New("later"))
		for range outcome.Invocation.Parts() {
		}
		waitErr := outcome.Invocation.Wait()
		assert.ErrorIs(t, waitErr, failure.ErrCanceled)
		assert.ErrorIs(t, waitErr, writeErr)
	})

	t.Run("caller cancellation", func(t *testing.T) {
		source := make(chan provider.StreamPart)
		model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: source}, nil
		}}
		runtime := newRuntimeForModel(t, model)
		ctx, cancel := context.WithCancel(context.Background())
		outcome := runtime.Stream(ctx, validTestCall())
		cancel()
		for range outcome.Invocation.Parts() {
		}
		assert.ErrorIs(t, outcome.Invocation.Wait(), context.Canceled)
	})

	t.Run("total timeout", func(t *testing.T) {
		source := make(chan provider.StreamPart)
		model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: source}, nil
		}}
		runtime := newRuntimeForModel(t, model, WithTotalTimeout(10*time.Millisecond))
		outcome := runtime.Stream(context.Background(), validTestCall())
		for range outcome.Invocation.Parts() {
		}
		assert.ErrorIs(t, outcome.Invocation.Wait(), failure.ErrTimeout)
	})

	t.Run("blocked forwarding exits even when provider ignores cancellation", func(t *testing.T) {
		source := make(chan provider.StreamPart, streamBufferSize+2)
		for i := 0; i < cap(source); i++ {
			source <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "part"}
		}
		model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: source}, nil
		}}
		runtime := newRuntimeForModel(t, model)
		outcome := runtime.Stream(context.Background(), validTestCall())
		require.Eventually(t, func() bool {
			return len(outcome.Invocation.Parts()) == streamBufferSize
		}, time.Second, time.Millisecond)
		cancelCause := errors.New("stop backpressure")
		outcome.Invocation.Cancel(cancelCause)
		for range outcome.Invocation.Parts() {
		}
		assert.ErrorIs(t, outcome.Invocation.Wait(), cancelCause)
	})
}

func TestStreamInvocation_ContextTermination(t *testing.T) {
	t.Run("custom cancellation cause", func(t *testing.T) {
		private := errors.New("private cancellation")
		for range 64 {
			ctx, cancel := context.WithCancelCause(context.Background())
			source := make(chan provider.StreamPart)
			close(source)
			cancel(private)
			invocation := newStreamInvocation(Identity{}, source, ctx, cancel, func() {}, true)
			for range invocation.Parts() {
			}
			waitErr := invocation.Wait()
			assert.ErrorIs(t, waitErr, failure.ErrCanceled)
			assert.ErrorIs(t, waitErr, private)
		}
	})

	t.Run("deadline cause", func(t *testing.T) {
		private := errors.New("private deadline")
		ctx, cancel := context.WithDeadlineCause(context.Background(), time.Now().Add(-time.Second), private)
		defer cancel()
		source := make(chan provider.StreamPart)
		invocation := newStreamInvocation(Identity{}, source, ctx, func(error) {}, func() {}, true)
		for range invocation.Parts() {
		}
		waitErr := invocation.Wait()
		assert.ErrorIs(t, waitErr, failure.ErrTimeout)
		assert.ErrorIs(t, waitErr, private)
	})
}

func TestStream_LateSuccessAfterSetupContextEnds(t *testing.T) {
	cases := []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		options []Option
		kind    failure.Kind
	}{
		{name: "timeout", context: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }, options: []Option{WithTotalTimeout(10 * time.Millisecond)}, kind: failure.KindTimeout},
		{name: "parent cancellation", context: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }, kind: failure.KindCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.context()
			if tc.kind == failure.KindCanceled {
				cancel()
			} else {
				defer cancel()
			}
			model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(invocationCtx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				<-invocationCtx.Done()
				parts := make(chan provider.StreamPart)
				close(parts)
				return &provider.StreamResult{Stream: parts}, nil
			}}
			runtime := newRuntimeForModel(t, model, tc.options...)
			outcome := runtime.Stream(ctx, validTestCall())
			require.NotNil(t, outcome.Failure)
			assert.Nil(t, outcome.Invocation)
			assert.Equal(t, tc.kind, outcome.Failure.Kind)
		})
	}
}

func TestStream_BlockedSetupReceivesDeadlineSynchronously(t *testing.T) {
	contextDone := make(chan struct{})
	release := make(chan struct{})
	model := &runtimeTestModel{providerID: "p", modelID: "m", stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		<-ctx.Done()
		close(contextDone)
		<-release
		return nil, ctx.Err()
	}}
	runtime := newRuntimeForModel(t, model, WithTotalTimeout(10*time.Millisecond))
	returned := make(chan StreamOutcome, 1)
	go func() { returned <- runtime.Stream(context.Background(), validTestCall()) }()
	require.Eventually(t, func() bool {
		select {
		case <-contextDone:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	select {
	case <-returned:
		t.Fatal("Stream returned before synchronous provider setup returned")
	default:
	}
	close(release)
	outcome := <-returned
	require.NotNil(t, outcome.Failure)
	assert.Equal(t, failure.KindTimeout, outcome.Failure.Kind)
}

func newRuntimeForModel(t *testing.T, model provider.LanguageModel, options ...Option) *Runtime {
	t.Helper()
	resolver := ModelResolverFunc(func(context.Context, GatewayCall) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{ID: "canonical", Model: model}, nil
	})
	runtime, err := New(resolver, options...)
	require.NoError(t, err)
	return runtime
}

func assertContextValues(t *testing.T, ctx context.Context) {
	t.Helper()
	protocol, ok := ProtocolFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, ProtocolLanguageModelV4, protocol)
	requestID, ok := RequestIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "request-1", requestID)
	requestedModelID, ok := RequestedModelIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "model", requestedModelID)
	canonicalModelID, ok := CanonicalModelIDFromContext(ctx)
	require.True(t, ok)
	assert.NotEmpty(t, canonicalModelID)
	assert.Equal(t, "tenant-a", AuthenticatedAttributesFromContext(ctx)["tenant"])
}
