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
		{"inconsistent tool history", fmt.Errorf("wrapped: %w", ErrInvalidToolHistory), CategoryInvalidRequest, ReasonNone},
		{"account access", fmt.Errorf("wrapped: %w", ErrAccountAccess), CategoryPermission, ReasonNone},
		{"byok credentials", ErrInvalidBYOK, CategoryBYOKCredentials, ReasonNone},
		{"byok selector", ErrInvalidBYOKSelector, CategoryBYOKSelector, ReasonNone},
		{"gateway control", ErrUnsupportedGatewayControl, CategoryGatewayControl, ReasonNone},
		{"catalog configuration", fmt.Errorf("wrapped: %w", ErrCatalogConfiguration), CategoryCatalogConfiguration, ReasonNone},
		{"reserved options", ErrReservedProviderOptions, CategoryInvalidRequest, ReasonReservedProviderOptions},
		{"unknown model", &catalog.UnknownModelError{ModelID: "private"}, CategoryModelNotFound, ReasonNone},
		{"routing controls", fmt.Errorf("wrapped: %w", ErrRoutingControls), CategoryInvalidRequest, ReasonRoutingControls},
		{"no candidates", ErrNoRoutingCandidates, CategoryInvalidRequest, ReasonNoCandidates},
		{"attempt bound", ErrRoutingAttemptBound, CategoryInvalidRequest, ReasonAttemptBound},
		{"unknown fallback model", fmt.Errorf("wrapped: %w", ErrUnknownFallbackModel), CategoryModelNotFound, ReasonFallbackModel},
		{"provider fallthrough", apiError(429, nil), CategoryRateLimit, ReasonNone},
		{"egress refusal", fmt.Errorf("wrapped: %w", ErrEgressRefused), CategoryEgressRefused, ReasonNone},
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
		{"egress refusal", ErrEgressRefused, CategoryEgressRefused},
		{"egress connection failure", fmt.Errorf("anthropic: %w", ErrEgressConnection), CategoryUpstream},
		{"egress connection timeout", fmt.Errorf("anthropic: %w", ErrEgressTimeout), CategoryTimeout},
		{"cancellation after a refused attempt", errors.Join(ErrEgressRefused, context.Canceled), CategoryCancellation},
		{"deadline after a refused attempt", errors.Join(ErrEgressRefused, context.DeadlineExceeded), CategoryTimeout},
		{"timed-out attempt after a refused attempt", errors.Join(ErrEgressRefused, ErrEgressTimeout), CategoryTimeout},
		{"failed connection after a refused attempt", errors.Join(ErrEgressRefused, ErrEgressConnection), CategoryEgressRefused},
		{"egress refusal inside a provider transport error", apiError(0, &url.Error{Op: "Post", URL: "https://llm.example", Err: ErrEgressRefused}), CategoryEgressRefused},
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
		{"invalid output", fmt.Errorf("wrapped: %w", ErrInvalidOutput), CategoryInternal},
		{"other", errors.New("private internal"), CategoryInternal},
		{"nil", nil, CategoryInternal},
		{"panicking error", panicError{}, CategoryInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.category, ClassifyProvider(tc.err))
		})
	}
}

func TestClassifyProvider_ChainFailures(t *testing.T) {
	attempt := func(err error) error {
		return fmt.Errorf("fallback: provider %q model %q: %w", "instance", "model", err)
	}
	chain := func(newestFirst ...error) error {
		wrapped := make([]error, len(newestFirst))
		for i, err := range newestFirst {
			wrapped[i] = attempt(err)
		}
		return errors.Join(wrapped...)
	}
	refusal := catalog.ErrUnsupportedRequest
	functionality := fmt.Errorf("anthropic: %w", provider.ErrUnsupportedFunctionality)
	unknown := errors.New("private failure")
	for _, tc := range []struct {
		name     string
		err      error
		category Category
	}{
		{"refusal then 503", chain(apiError(503, nil), refusal), CategoryOverload},
		{"503 then refusal", chain(refusal, apiError(503, nil)), CategoryOverload},
		{"refusal then 429", chain(apiError(429, nil), refusal), CategoryRateLimit},
		{"most recent non-refusal decides", chain(apiError(503, nil), refusal, apiError(429, nil)), CategoryOverload},
		{"refusal then unknown error", chain(unknown, refusal), CategoryInternal},
		{"every attempt refused", chain(refusal, refusal), CategoryInvalidRequest},
		{"single refusal", refusal, CategoryInvalidRequest},
		{"refusal then call cancellation", errors.Join(refusal, context.Canceled), CategoryCancellation},
		{"refusal then call deadline", errors.Join(refusal, context.DeadlineExceeded), CategoryTimeout},
		{"chain then call cancellation", errors.Join(chain(apiError(503, nil), refusal), context.Canceled), CategoryCancellation},
		{"attempt deadline then 503", chain(apiError(503, nil), context.DeadlineExceeded), CategoryOverload},
		{"attempt cancellation then 503", chain(apiError(503, nil), context.Canceled), CategoryOverload},
		{"functionality refusal then 503", chain(apiError(503, nil), functionality), CategoryOverload},
		{"503 then functionality refusal", chain(functionality, apiError(503, nil)), CategoryOverload},
		{"unknown error then functionality refusal", chain(functionality, unknown), CategoryInternal},
		{"functionality refusal then unknown error", chain(unknown, functionality), CategoryInternal},
		{"functionality and request refusals", chain(functionality, refusal), CategoryInvalidRequest},
		{"functionality refusal then call cancellation", errors.Join(functionality, context.Canceled), CategoryCancellation},
		{"API error wrapping the functionality sentinel", chain(apiError(400, provider.ErrUnsupportedFunctionality), refusal), CategoryFailedDependency},
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
