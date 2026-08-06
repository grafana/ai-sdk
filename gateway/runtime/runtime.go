package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

const (
	// DefaultTotalTimeout is the default maximum duration of model invocation
	// and stream consumption after successful resolution.
	DefaultTotalTimeout = 120 * time.Second
)

// Identity records public catalog identity and the values reported by the
// resolved model before runtime middleware is attached. Resolved values do not
// necessarily identify a fallback attempt that actually executed.
type Identity struct {
	// RequestedModelID is the exact public model ID originally supplied by the caller.
	RequestedModelID string
	// CanonicalModelID is the canonical public catalog route selected by resolution.
	CanonicalModelID string
	// ResolvedProviderID is the provider ID reported by the resolved model before
	// middleware; it does not identify a fallback attempt that actually executed.
	ResolvedProviderID string
	// ResolvedModelID is the model ID reported by the resolved model before
	// middleware; it does not identify a fallback attempt that actually executed.
	ResolvedModelID string
}

// GenerateOutcome contains the available identity, provider result, or
// classified failure from one generate invocation.
type GenerateOutcome struct {
	// Identity contains all routing identity available at termination.
	Identity Identity
	// Result is the provider result when generation succeeds.
	Result *provider.GenerateResult
	// Failure is the classified failure when generation does not succeed; its
	// cause is private and must not be projected directly to callers.
	Failure *failure.Classification
}

// StreamOutcome contains the available identity, stream invocation, or
// classified setup failure from one stream invocation.
type StreamOutcome struct {
	// Identity contains all routing identity available after stream setup.
	Identity Identity
	// Invocation is the single-consumer stream lifecycle when setup succeeds.
	Invocation *StreamInvocation
	// Failure is the classified setup failure when no usable invocation exists;
	// its cause is private and must not be projected directly to callers.
	Failure *failure.Classification
}

// Option configures a Runtime.
type Option func(*runtimeOptions) error

type runtimeOptions struct {
	totalTimeout time.Duration
	policies     []CallPolicy
	middlewares  []middleware.Middleware
}

// WithTotalTimeout sets the post-resolution invocation timeout.
func WithTotalTimeout(timeout time.Duration) Option {
	return func(options *runtimeOptions) error {
		if timeout <= 0 {
			return fmt.Errorf("gateway runtime: total timeout must be positive")
		}
		options.totalTimeout = timeout
		return nil
	}
}

// WithCallPolicies configures ordered pre-resolution policies.
func WithCallPolicies(policies ...CallPolicy) Option {
	return func(options *runtimeOptions) error {
		for _, policy := range policies {
			if isNilInterface(policy) {
				return fmt.Errorf("gateway runtime: nil call policy")
			}
		}
		options.policies = append(options.policies, policies...)
		return nil
	}
}

// WithMiddleware configures ordered model middleware. The first middleware is
// outermost, matching middleware.WrapLanguageModel.
func WithMiddleware(middlewares ...middleware.Middleware) Option {
	return func(options *runtimeOptions) error {
		options.middlewares = append(options.middlewares, middlewares...)
		return nil
	}
}

// Runtime executes normalized gateway LanguageModel calls.
type Runtime struct {
	resolver     ModelResolver
	totalTimeout time.Duration
	policies     []CallPolicy
	middlewares  []middleware.Middleware
}

// New constructs a Runtime.
func New(resolver ModelResolver, options ...Option) (*Runtime, error) {
	if isNilInterface(resolver) {
		return nil, fmt.Errorf("gateway runtime: nil model resolver")
	}
	config := runtimeOptions{totalTimeout: DefaultTotalTimeout}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("gateway runtime: nil option")
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}
	return &Runtime{
		resolver:     resolver,
		totalTimeout: config.totalTimeout,
		policies:     append([]CallPolicy(nil), config.policies...),
		middlewares:  append([]middleware.Middleware(nil), config.middlewares...),
	}, nil
}

// Generate executes one synchronous provider generate call.
func (runtime *Runtime) Generate(ctx context.Context, call GatewayCall) GenerateOutcome {
	call, identity, model, classification := runtime.prepare(ctx, call)
	if classification != nil {
		return GenerateOutcome{Identity: identity, Failure: classification}
	}

	invocationCtx, cancel := context.WithTimeoutCause(ctx, runtime.totalTimeout, failure.ErrTimeout)
	defer cancel()
	invocationCtx = withInvocationContext(invocationCtx, call, identity)
	wrapped := middleware.WrapLanguageModel(model, runtime.middlewares...)
	result, err := wrapped.DoGenerate(invocationCtx, cloneCallOptions(call.CallOptions))
	if err != nil {
		return GenerateOutcome{Identity: identity, Failure: classifyInvocationError(invocationCtx, err, call.RequestedModelID)}
	}
	if cause := context.Cause(invocationCtx); cause != nil {
		return GenerateOutcome{Identity: identity, Failure: classifyInvocationError(invocationCtx, cause, call.RequestedModelID)}
	}
	if result == nil {
		err = failure.Wrap(failure.ErrInternal, errors.New("gateway runtime: provider returned a nil generate result"))
		return GenerateOutcome{Identity: identity, Failure: classificationPointer(failure.Classify(err, failure.WithRetryable(false)))}
	}
	return GenerateOutcome{Identity: identity, Result: result}
}

// Stream creates one stream invocation after synchronous provider setup.
func (runtime *Runtime) Stream(ctx context.Context, call GatewayCall) StreamOutcome {
	call, identity, model, classification := runtime.prepare(ctx, call)
	if classification != nil {
		return StreamOutcome{Identity: identity, Failure: classification}
	}

	timeoutCtx, timeoutCancel := context.WithTimeoutCause(ctx, runtime.totalTimeout, failure.ErrTimeout)
	invocationCtx, cancelCause := context.WithCancelCause(timeoutCtx)
	invocationCtx = withInvocationContext(invocationCtx, call, identity)
	wrapped := middleware.WrapLanguageModel(model, runtime.middlewares...)
	result, err := wrapped.DoStream(invocationCtx, cloneCallOptions(call.CallOptions))
	if err != nil {
		classification := classifyInvocationError(invocationCtx, err, call.RequestedModelID)
		cancelCause(err)
		timeoutCancel()
		return StreamOutcome{Identity: identity, Failure: classification}
	}
	if cause := context.Cause(invocationCtx); cause != nil {
		cancelCause(cause)
		timeoutCancel()
		return StreamOutcome{Identity: identity, Failure: classifyInvocationError(invocationCtx, cause, call.RequestedModelID)}
	}
	if result == nil || result.Stream == nil {
		err = failure.Wrap(failure.ErrInternal, errors.New("gateway runtime: provider returned a nil stream"))
		cancelCause(err)
		timeoutCancel()
		return StreamOutcome{Identity: identity, Failure: classificationPointer(failure.Classify(err, failure.WithRetryable(false)))}
	}

	invocation := newStreamInvocation(identity, result.Stream, invocationCtx, cancelCause, timeoutCancel, call.CallOptions.IncludeRawChunks)
	return StreamOutcome{Identity: identity, Invocation: invocation}
}

func (runtime *Runtime) prepare(ctx context.Context, original GatewayCall) (GatewayCall, Identity, provider.LanguageModel, *failure.Classification) {
	call := cloneGatewayCall(original)
	identity := Identity{RequestedModelID: call.RequestedModelID}
	if err := validateCall(call); err != nil {
		return GatewayCall{}, identity, nil, classificationPointer(failure.Classify(err, failure.WithRequestedModelID(call.RequestedModelID)))
	}

	policyCall, err := applyPolicies(ctx, call, runtime.policies)
	if err != nil {
		return GatewayCall{}, identity, nil, classifyPreparationError(ctx, err, call.RequestedModelID)
	}
	call = policyCall

	resolved, err := runtime.resolver.ResolveModel(ctx, cloneGatewayCall(call))
	if err != nil {
		if errors.Is(err, catalog.ErrUnknownModel) && !errors.Is(err, failure.ErrUnknownModel) {
			err = failure.Wrap(failure.ErrUnknownModel, err)
		}
		return GatewayCall{}, identity, nil, classifyPreparationError(ctx, err, call.RequestedModelID)
	}
	identity.CanonicalModelID = resolved.ID
	if !isNilInterface(resolved.Model) {
		identity.ResolvedProviderID = resolved.Model.Provider()
		identity.ResolvedModelID = resolved.Model.ModelID()
	}
	if resolved.ID == "" || isNilInterface(resolved.Model) {
		err = failure.Wrap(failure.ErrInternal, errors.New("gateway runtime: resolver returned an invalid model"))
		return GatewayCall{}, identity, nil, classificationPointer(failure.Classify(err, failure.WithRetryable(false)))
	}

	return call, identity, resolved.Model, nil
}

func classifyPreparationError(ctx context.Context, err error, requestedModelID string) *failure.Classification {
	classification := failure.Classify(err, failure.WithRequestedModelID(requestedModelID))
	if classification.Kind != failure.KindInternal && classification.Kind != failure.KindFailedDependency {
		return classificationPointer(classification)
	}

	var category error
	switch ctx.Err() {
	case context.DeadlineExceeded:
		category = failure.ErrTimeout
	case context.Canceled:
		category = failure.ErrCanceled
	default:
		return classificationPointer(classification)
	}
	privateCause := errors.Join(err, context.Cause(ctx))
	classification = failure.Classify(category, failure.WithRequestedModelID(requestedModelID))
	classification.Cause = failure.Wrap(category, privateCause)
	return classificationPointer(classification)
}

func classifyInvocationError(ctx context.Context, err error, requestedModelID string) *failure.Classification {
	contextCause := context.Cause(ctx)
	privateCause := errors.Join(err, contextCause)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(contextCause, failure.ErrTimeout), errors.Is(contextCause, context.DeadlineExceeded):
		err = failure.Wrap(failure.ErrTimeout, privateCause)
	case errors.Is(ctx.Err(), context.Canceled):
		err = failure.Wrap(failure.ErrCanceled, privateCause)
	}
	return classificationPointer(failure.Classify(err, failure.WithRequestedModelID(requestedModelID)))
}

func classificationPointer(classification failure.Classification) *failure.Classification {
	return &classification
}
