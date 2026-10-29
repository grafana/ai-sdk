package execution

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

type testNetError struct{ timeout bool }

func (e testNetError) Error() string   { return "private transport details" }
func (e testNetError) Timeout() bool   { return e.timeout }
func (e testNetError) Temporary() bool { return false }

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type panicError struct{}

func (panicError) Error() string { panic("private") }
func (panicError) Is(error) bool { panic("private") }

func apiError(status int, cause error) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: status, Cause: cause})
}

func TestClassifySelection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		category Category
		reason   Reason
	}{
		{"account access", fmt.Errorf("wrapped: %w", ErrAccountAccess), CategoryPermission, ReasonNone},
		{"byok credentials", ErrInvalidBYOK, CategoryBYOKCredentials, ReasonNone},
		{"byok selector", ErrInvalidBYOKSelector, CategoryBYOKSelector, ReasonNone},
		{"gateway control", ErrUnsupportedGatewayControl, CategoryGatewayControl, ReasonNone},
		{"reserved options", ErrReservedProviderOptions, CategoryInvalidRequest, ReasonReservedProviderOptions},
		{"unknown model", &catalog.UnknownModelError{ModelID: "private"}, CategoryModelNotFound, ReasonNone},
		{"provider fallthrough", apiError(429, nil), CategoryRateLimit, ReasonNone},
		{"deadline", context.DeadlineExceeded, CategoryTimeout, ReasonNone},
		{"other", errors.New("private"), CategoryInternal, ReasonNone},
		{"nil", nil, CategoryInternal, ReasonNone},
		{"panicking error", panicError{}, CategoryInternal, ReasonNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			category, reason := ClassifySelection(tc.err)
			assert.Equal(t, tc.category, category)
			assert.Equal(t, tc.reason, reason)
		})
	}
}

func TestClassifyProvider(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		category Category
	}{
		{"canceled", context.Canceled, CategoryCancellation},
		{"deadline", context.DeadlineExceeded, CategoryTimeout},
		{"unsupported request", fmt.Errorf("wrapped: %w", catalog.ErrUnsupportedRequest), CategoryInvalidRequest},
		{"unsupported functionality", fmt.Errorf("wrapped: %w", provider.ErrUnsupportedFunctionality), CategoryInvalidRequest},
		{"408", apiError(408, nil), CategoryTimeout},
		{"504", apiError(504, nil), CategoryTimeout},
		{"429", apiError(429, nil), CategoryRateLimit},
		{"503", apiError(503, nil), CategoryOverload},
		{"529", apiError(529, nil), CategoryOverload},
		{"400", apiError(400, nil), CategoryFailedDependency},
		{"500", apiError(500, nil), CategoryUpstream},
		{"net timeout", testNetError{timeout: true}, CategoryTimeout},
		{"url error", &url.Error{Op: "Post", URL: "https://private", Err: testNetError{}}, CategoryUpstream},
		{"dial error", &net.OpError{Op: "dial", Net: "tcp", Addr: testAddr("private:443"), Err: errors.New("refused")}, CategoryUpstream},
		{"other", errors.New("private internal"), CategoryInternal},
		{"nil", nil, CategoryInternal},
		{"panicking error", panicError{}, CategoryInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.category, ClassifyProvider(tc.err))
		})
	}
}

func TestClassify_ProviderAndStreamBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		unary  Category
		stream Category
	}{
		{"wrapped cancellation", apiError(401, context.Canceled), CategoryCancellation, CategoryFailedDependency},
		{"wrapped timeout", apiError(401, context.DeadlineExceeded), CategoryTimeout, CategoryFailedDependency},
		{"invalid status", apiError(700, nil), CategoryUpstream, CategoryInternal},
		{"zero status", apiError(0, nil), CategoryUpstream, CategoryProviderStream},
		{"zero status plain cause", apiError(0, errors.New("private")), CategoryUpstream, CategoryProviderStream},
		{"zero status wrapped deadline", apiError(0, fmt.Errorf("read: %w", context.DeadlineExceeded)), CategoryTimeout, CategoryTimeout},
		{"zero status wrapped cancellation", apiError(0, fmt.Errorf("read: %w", context.Canceled)), CategoryCancellation, CategoryCancellation},
		{"zero canceled status", apiError(0, context.Canceled), CategoryCancellation, CategoryCancellation},
		{"zero status transport timeout", apiError(0, testNetError{timeout: true}), CategoryUpstream, CategoryTimeout},
		{"nil API error", (*provider.APICallError)(nil), CategoryInternal, CategoryInternal},
		{"non API error", errors.New("private"), CategoryInternal, CategoryInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.unary, ClassifyProvider(tc.err))
			assert.Equal(t, tc.stream, ClassifyStreamPart(tc.err))
		})
	}
}
