// Package failure classifies gateway execution failures without choosing a
// transport status or public protocol envelope.
package failure

import (
	"context"
	"errors"
	"reflect"

	"github.com/grafana/ai-sdk/provider"
)

// Kind identifies a protocol-neutral gateway failure category.
type Kind string

const (
	// KindUnauthenticated identifies missing or invalid caller authentication.
	KindUnauthenticated Kind = "unauthenticated"
	// KindInvalidCall identifies invalid caller-owned input.
	KindInvalidCall Kind = "invalid_call"
	// KindUnknownModel identifies an unknown public model ID.
	KindUnknownModel Kind = "unknown_model"
	// KindForbidden identifies a policy-denied call.
	KindForbidden Kind = "forbidden"
	// KindRateLimited identifies provider or gateway rate limiting.
	KindRateLimited Kind = "rate_limited"
	// KindTimeout identifies an invocation deadline expiry.
	KindTimeout Kind = "timeout"
	// KindCanceled identifies caller or adapter cancellation.
	KindCanceled Kind = "canceled"
	// KindFailedDependency identifies a provider-bound failure.
	KindFailedDependency Kind = "failed_dependency"
	// KindInternal identifies an unclassified runtime defect.
	KindInternal Kind = "internal"
)

const (
	providerStatusRequestTimeout      = 408
	providerStatusTooManyRequests     = 429
	providerStatusInternalServerError = 500
)

var (
	// ErrUnauthenticated marks missing or invalid caller authentication.
	ErrUnauthenticated = errors.New("gateway: unauthenticated")
	// ErrInvalidCall marks invalid caller-owned input.
	ErrInvalidCall = errors.New("gateway: invalid call")
	// ErrUnknownModel marks an unknown public model ID.
	ErrUnknownModel = errors.New("gateway: unknown model")
	// ErrForbidden marks a policy-denied call.
	ErrForbidden = errors.New("gateway: forbidden")
	// ErrRateLimited marks provider or gateway rate limiting.
	ErrRateLimited = errors.New("gateway: rate limited")
	// ErrTimeout marks invocation deadline expiry.
	ErrTimeout = errors.New("gateway: timeout")
	// ErrCanceled marks caller or adapter cancellation.
	ErrCanceled = errors.New("gateway: canceled")
	// ErrFailedDependency marks a provider-bound failure.
	ErrFailedDependency = errors.New("gateway: failed dependency")
	// ErrInternal marks an unclassified runtime defect.
	ErrInternal = errors.New("gateway: internal failure")
)

// SafeParameters contains the request-owned values that a protocol adapter may
// expose publicly.
type SafeParameters struct {
	// RequestedModelID is the caller-owned public model ID approved for projection.
	RequestedModelID string
	// PolicyID is an explicitly allowlisted public policy identifier.
	PolicyID string
}

// Classification is a derived gateway failure decision. Cause is private
// execution detail and must not be serialized by protocol adapters.
type Classification struct {
	// Kind is the transport-neutral failure category selected at this boundary.
	Kind Kind
	// Retryable is freshly derived for this boundary and is not inherited metadata.
	Retryable bool
	// Cause retains private error-chain detail and must never be serialized publicly.
	Cause error
	// SafeParameters contains only typed, allowlisted public projection values.
	SafeParameters SafeParameters
}

// Wrap makes cause match category while preserving cause traversal. It does
// not attach retryability to the error chain.
func Wrap(category, cause error) error {
	if isNilError(category) {
		return normalizeError(cause)
	}
	if isNilError(cause) {
		return category
	}
	return errors.Join(category, cause)
}

// ClassifyOption configures a classification decision.
type ClassifyOption func(*classifyOptions)

type classifyOptions struct {
	retryable      *bool
	safeParameters SafeParameters
}

// WithRetryable overrides retryability at the active classification boundary.
func WithRetryable(retryable bool) ClassifyOption {
	return func(options *classifyOptions) {
		options.retryable = &retryable
	}
}

// WithRequestedModelID adds the caller-owned public model identifier.
func WithRequestedModelID(modelID string) ClassifyOption {
	return func(options *classifyOptions) {
		options.safeParameters.RequestedModelID = modelID
	}
}

// WithPolicyID adds an allowlisted public policy identifier.
func WithPolicyID(policyID string) ClassifyOption {
	return func(options *classifyOptions) {
		options.safeParameters.PolicyID = policyID
	}
}

// Classify derives one deterministic classification from err.
func Classify(err error, options ...ClassifyOption) Classification {
	err = normalizeError(err)
	config := classifyOptions{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}

	kind := classifyKind(err)
	retryable := deriveRetryability(kind, err)
	if config.retryable != nil {
		retryable = *config.retryable
	}
	return Classification{
		Kind:           kind,
		Retryable:      retryable,
		Cause:          err,
		SafeParameters: config.safeParameters,
	}
}

func classifyKind(err error) Kind {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		return KindUnauthenticated
	case errors.Is(err, ErrInvalidCall):
		return KindInvalidCall
	case errors.Is(err, ErrUnknownModel):
		return KindUnknownModel
	case errors.Is(err, ErrForbidden):
		return KindForbidden
	case errors.Is(err, ErrRateLimited):
		return KindRateLimited
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return KindTimeout
	case errors.Is(err, ErrCanceled), errors.Is(err, context.Canceled):
		return KindCanceled
	case errors.Is(err, ErrFailedDependency):
		return KindFailedDependency
	case errors.Is(err, ErrInternal):
		return KindInternal
	}

	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) && apiErr != nil {
		if apiErr.StatusCode == providerStatusTooManyRequests {
			return KindRateLimited
		}
		return KindFailedDependency
	}
	return KindInternal
}

func deriveRetryability(kind Kind, err error) bool {
	switch kind {
	case KindRateLimited, KindTimeout:
		return true
	case KindFailedDependency:
		var apiErr *provider.APICallError
		if !errors.As(err, &apiErr) || apiErr == nil {
			return false
		}
		return apiErr.StatusCode == providerStatusRequestTimeout ||
			apiErr.StatusCode >= providerStatusInternalServerError ||
			(apiErr.StatusCode == 0 && apiErr.IsRetryable)
	default:
		return false
	}
}

func normalizeError(err error) error {
	if isNilError(err) {
		return nil
	}
	return err
}

func isNilError(err error) bool {
	if err == nil {
		return true
	}
	value := reflect.ValueOf(err)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
