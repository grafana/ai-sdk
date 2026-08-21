// Package failure defines protocol-neutral, privacy-safe gateway failures.
package failure

import (
	"errors"
)

// Category identifies a safe public failure class.
type Category string

const (
	CategoryInvalidRequest   Category = "invalid_request"
	CategoryAuthentication   Category = "authentication"
	CategoryPermission       Category = "permission"
	CategoryNotFound         Category = "not_found"
	CategoryRateLimit        Category = "rate_limit"
	CategoryOverload         Category = "overload"
	CategoryFailedDependency Category = "failed_dependency"
	CategoryUpstreamFailure  Category = "upstream_failure"
	CategoryTimeout          Category = "timeout"
	CategoryCancellation     Category = "cancellation"
	CategoryInternalFailure  Category = "internal_failure"
)

// Failure contains only information approved for a public protocol boundary.
type Failure struct {
	category  Category
	message   string
	retryable bool
}

// New constructs a safe failure. Retryability is fixed by category.
func New(category Category, message string) (Failure, error) {
	retryable, ok := categoryRetryable(category)
	if !ok {
		return Failure{}, errors.New("failure: unsupported category")
	}
	if message == "" {
		return Failure{}, errors.New("failure: message must not be empty")
	}
	return Failure{category: category, message: message, retryable: retryable}, nil
}

// Category returns the safe failure category.
func (f Failure) Category() Category { return f.category }

// Message returns the approved public message.
func (f Failure) Message() string { return f.message }

// Retryable reports the fixed retryability for the category.
func (f Failure) Retryable() bool { return f.retryable }

// Valid reports whether the value could have been produced by New.
func (f Failure) Valid() bool {
	retryable, ok := categoryRetryable(f.category)
	return ok && f.message != "" && retryable == f.retryable
}

func categoryRetryable(category Category) (bool, bool) {
	switch category {
	case CategoryInvalidRequest,
		CategoryAuthentication,
		CategoryPermission,
		CategoryNotFound,
		CategoryFailedDependency,
		CategoryCancellation:
		return false, true
	case CategoryRateLimit,
		CategoryOverload,
		CategoryUpstreamFailure,
		CategoryTimeout,
		CategoryInternalFailure:
		return true, true
	default:
		return false, false
	}
}
