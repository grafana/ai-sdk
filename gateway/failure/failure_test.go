package failure

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrap_PreservesCategoryAndCause(t *testing.T) {
	cause := provider.NewAPICallError(provider.APICallErrorOptions{Message: "private", StatusCode: http.StatusBadGateway})
	err := Wrap(ErrFailedDependency, cause)

	assert.ErrorIs(t, err, ErrFailedDependency)
	var got *provider.APICallError
	require.ErrorAs(t, err, &got)
	assert.Same(t, cause, got)
	assert.ErrorIs(t, Wrap(ErrForbidden, nil), ErrForbidden)
	assert.Same(t, cause, Wrap(nil, cause))
}

func TestClassify_DeterministicKindAndRetryability(t *testing.T) {
	retryable := true
	cases := []struct {
		name      string
		err       error
		kind      Kind
		retryable bool
	}{
		{name: "unauthenticated", err: ErrUnauthenticated, kind: KindUnauthenticated},
		{name: "invalid call", err: ErrInvalidCall, kind: KindInvalidCall},
		{name: "unknown model", err: ErrUnknownModel, kind: KindUnknownModel},
		{name: "forbidden", err: ErrForbidden, kind: KindForbidden},
		{name: "rate limited category", err: ErrRateLimited, kind: KindRateLimited, retryable: true},
		{name: "timeout category", err: ErrTimeout, kind: KindTimeout, retryable: true},
		{name: "deadline", err: context.DeadlineExceeded, kind: KindTimeout, retryable: true},
		{name: "canceled", err: context.Canceled, kind: KindCanceled},
		{name: "permanent dependency", err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusUnauthorized}), kind: KindFailedDependency},
		{name: "unattributed bad request", err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusBadRequest}), kind: KindFailedDependency},
		{name: "conflict is not inherited", err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusConflict}), kind: KindFailedDependency},
		{name: "transient dependency", err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusBadGateway}), kind: KindFailedDependency, retryable: true},
		{name: "transport dependency", err: provider.NewAPICallError(provider.APICallErrorOptions{IsRetryable: &retryable}), kind: KindFailedDependency, retryable: true},
		{name: "provider rate limit", err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusTooManyRequests}), kind: KindRateLimited, retryable: true},
		{name: "internal", err: errors.New("defect"), kind: KindInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			classification := Classify(tc.err)
			assert.Equal(t, tc.kind, classification.Kind)
			assert.Equal(t, tc.retryable, classification.Retryable)
			assert.Equal(t, tc.err, classification.Cause)
		})
	}
}

func TestClassify_TypedNilErrorsFailClosed(t *testing.T) {
	var apiErr *provider.APICallError
	classification := Classify(apiErr)
	assert.Equal(t, KindInternal, classification.Kind)
	assert.False(t, classification.Retryable)
	assert.Nil(t, classification.Cause)

	wrapped := fmt.Errorf("wrapped typed nil: %w", apiErr)
	classification = Classify(wrapped)
	assert.Equal(t, KindInternal, classification.Kind)
	assert.False(t, classification.Retryable)
	assert.Same(t, wrapped, classification.Cause)

	classification = Classify(Wrap(ErrFailedDependency, apiErr))
	assert.Equal(t, KindFailedDependency, classification.Kind)
	assert.False(t, classification.Retryable)
}

func TestClassify_PrecedenceAndBoundaryOverride(t *testing.T) {
	private := errors.New("private")
	classification := Classify(errors.Join(ErrFailedDependency, ErrTimeout, private))
	assert.Equal(t, KindTimeout, classification.Kind)
	assert.True(t, classification.Retryable)
	assert.ErrorIs(t, classification.Cause, private)

	classification = Classify(errors.Join(ErrInternal, ErrForbidden, private))
	assert.Equal(t, KindForbidden, classification.Kind)
	assert.False(t, classification.Retryable)

	classification = Classify(errors.Join(ErrForbidden, context.Canceled, private))
	assert.Equal(t, KindForbidden, classification.Kind)
	assert.False(t, classification.Retryable)

	classification = Classify(
		provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusServiceUnavailable}),
		WithRetryable(false),
	)
	assert.Equal(t, KindFailedDependency, classification.Kind)
	assert.False(t, classification.Retryable)
}

func TestClassify_SafeParametersAreTyped(t *testing.T) {
	classification := Classify(
		ErrUnknownModel,
		WithRequestedModelID("alias-a"),
		WithPolicyID("public-policy"),
	)
	assert.Equal(t, SafeParameters{RequestedModelID: "alias-a", PolicyID: "public-policy"}, classification.SafeParameters)
}

func TestClassification_DoesNotImplementError(t *testing.T) {
	classification := Classify(ErrInternal)
	_, implementsError := any(classification).(error)
	assert.False(t, implementsError)
}
