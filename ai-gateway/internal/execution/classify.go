package execution

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

// ErrEgressRefused reports a provider connection that the custom-endpoint
// egress policy refused, or whose host did not resolve. It carries no
// endpoint, host or address.
var ErrEgressRefused = errors.New("execution: provider endpoint refused by the egress policy or not resolvable")

var (
	// ErrEgressConnection reports an egress connection, TLS handshake or
	// response read that failed after the egress policy permitted the address.
	// It names no host, address or certificate, and classifies as upstream.
	ErrEgressConnection = errors.New("execution: provider connection failed")
	// ErrEgressTimeout is ErrEgressConnection for a timeout. It wraps
	// [context.DeadlineExceeded], so it classifies as a timeout.
	ErrEgressTimeout = fmt.Errorf("execution: provider connection timed out: %w", context.DeadlineExceeded)
)

// Category is the closed failure class of a call. Adapters map each category
// to their own fixed error document.
type Category uint8

const (
	CategoryInvalidRequest Category = iota + 1
	CategoryBYOKCredentials
	CategoryBYOKSelector
	CategoryGatewayControl
	CategoryModelNotFound
	CategoryPermission
	CategoryRateLimit
	CategoryOverload
	CategoryFailedDependency
	CategoryUpstream
	CategoryTimeout
	CategoryCancellation
	CategoryInternal
	// CategoryProviderStream is an in-band provider stream error without an
	// HTTP status or a cancellation, deadline or transport cause. Only
	// [ClassifyStreamPart] returns it.
	CategoryProviderStream
	// CategoryCatalogConfiguration is a stored stack catalog that fails its
	// checks or lacks a provider key.
	CategoryCatalogConfiguration
	// CategoryEgressRefused is a provider connection the custom-endpoint egress
	// policy refused, or whose host did not resolve.
	CategoryEgressRefused
)

// Reason refines [CategoryInvalidRequest] for host policy and routing refusals
// and [CategoryModelNotFound] for unknown fallback models.
type Reason uint8

const (
	// ReasonNone carries no refinement.
	ReasonNone Reason = iota
	// ReasonReservedProviderOptions reports gateway controls a selector refused.
	ReasonReservedProviderOptions
	// ReasonProtectedCallHeader reports a credential-bearing call header.
	ReasonProtectedCallHeader
	// ReasonRoutingControls reports malformed models, order or only controls.
	ReasonRoutingControls
	// ReasonNoCandidates reports an only filter that excludes every candidate.
	ReasonNoCandidates
	// ReasonAttemptBound reports a composed chain above the attempt bound.
	ReasonAttemptBound
	// ReasonFallbackModel reports a models entry that is not an available model.
	ReasonFallbackModel
)

// ClassifySelection classifies an error returned by [Runtime.Call]: invalid
// tool history, a policy refusal, a host sentinel, a catalog error, or
// otherwise as [ClassifyProvider] does.
func ClassifySelection(err error) (category Category, reason Reason) {
	defer func() {
		if recover() != nil {
			category, reason = CategoryInternal, ReasonNone
		}
	}()
	if isNilInterface(err) {
		return CategoryInternal, ReasonNone
	}
	switch {
	case errors.Is(err, ErrInvalidToolHistory):
		return CategoryInvalidRequest, ReasonNone
	case errors.Is(err, ErrAccountAccess):
		return CategoryPermission, ReasonNone
	case errors.Is(err, ErrInvalidBYOK):
		return CategoryBYOKCredentials, ReasonNone
	case errors.Is(err, ErrInvalidBYOKSelector):
		return CategoryBYOKSelector, ReasonNone
	case errors.Is(err, ErrUnsupportedGatewayControl):
		return CategoryGatewayControl, ReasonNone
	case errors.Is(err, ErrCatalogConfiguration):
		return CategoryCatalogConfiguration, ReasonNone
	case errors.Is(err, ErrReservedProviderOptions):
		return CategoryInvalidRequest, ReasonReservedProviderOptions
	case errors.Is(err, ErrProtectedCallHeader):
		return CategoryInvalidRequest, ReasonProtectedCallHeader
	case errors.Is(err, ErrRoutingControls):
		return CategoryInvalidRequest, ReasonRoutingControls
	case errors.Is(err, ErrNoRoutingCandidates):
		return CategoryInvalidRequest, ReasonNoCandidates
	case errors.Is(err, ErrRoutingAttemptBound):
		return CategoryInvalidRequest, ReasonAttemptBound
	case errors.Is(err, ErrUnknownFallbackModel):
		return CategoryModelNotFound, ReasonFallbackModel
	case errors.Is(err, catalog.ErrUnknownModel):
		return CategoryModelNotFound, ReasonNone
	}
	return ClassifyProvider(err), ReasonNone
}

// ClassifyProvider classifies an error returned by unary invocation or stream
// setup. A fallback chain failure that joins several attempt failures, or
// attempt failures with the call's context error, is classified as one
// failure: a call-level cancellation or expired deadline wins, otherwise the
// most recent attempt failure that is not a refusal decides, and the chain is
// an invalid request only when every attempt refused.
func ClassifyProvider(err error) (category Category) {
	defer func() {
		if recover() != nil {
			category = CategoryInternal
		}
	}()
	if isNilInterface(err) {
		return CategoryInternal
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		if category, ok := classifyChain(joined.Unwrap()); ok {
			return category
		}
	}
	return classifySingle(err)
}

// classifyChain applies the aggregate rule to the members of a top-level join,
// as the fallback module produces them: attempt failures newest first,
// optionally followed by the call-level context error. It reports false for a
// join that is not an aggregate.
func classifyChain(members []error) (Category, bool) {
	if len(members) < 2 {
		return CategoryInternal, false
	}
	switch members[len(members)-1] {
	case context.Canceled:
		return CategoryCancellation, true
	case context.DeadlineExceeded:
		return CategoryTimeout, true
	}
	for _, attempt := range members {
		if category := classifySingle(attempt); !isRefusal(attempt, category) {
			return category, true
		}
	}
	return CategoryInvalidRequest, true
}

// isRefusal reports whether an attempt failed before native I/O because its
// candidate cannot take the request.
func isRefusal(err error, category Category) bool {
	return category == CategoryInvalidRequest && (errors.Is(err, catalog.ErrUnsupportedRequest) || errors.Is(err, provider.ErrUnsupportedFunctionality))
}

func classifySingle(err error) Category {
	switch {
	case errors.Is(err, ErrTranslation), errors.Is(err, ErrInvalidOutput):
		return CategoryInternal
	case errors.Is(err, catalog.ErrUnsupportedRequest):
		return CategoryInvalidRequest
	case errors.Is(err, context.Canceled):
		return CategoryCancellation
	case errors.Is(err, context.DeadlineExceeded):
		return CategoryTimeout
	case errors.Is(err, ErrEgressRefused):
		return CategoryEgressRefused
	}

	var apiError *provider.APICallError
	if errors.As(err, &apiError) {
		return categoryFromStatus(apiError.StatusCode)
	}
	if category, ok := classifyTransport(err); ok {
		return category
	}
	if errors.Is(err, provider.ErrUnsupportedFunctionality) {
		return CategoryInvalidRequest
	}
	return CategoryInternal
}

// ClassifyStreamPart classifies the error carried by an in-band stream error
// part.
func ClassifyStreamPart(err error) (category Category) {
	defer func() {
		if recover() != nil {
			category = CategoryInternal
		}
	}()
	if isNilInterface(err) {
		return CategoryInternal
	}

	var apiError *provider.APICallError
	if errors.As(err, &apiError) {
		if isNilInterface(apiError) {
			return CategoryInternal
		}
		if apiError.StatusCode == 0 {
			switch {
			case errors.Is(err, context.Canceled):
				return CategoryCancellation
			case errors.Is(err, context.DeadlineExceeded):
				return CategoryTimeout
			}
			if category, ok := classifyTransport(err); ok {
				return category
			}
			return CategoryProviderStream
		}
		if apiError.StatusCode < 100 || apiError.StatusCode > 599 {
			return CategoryInternal
		}
		return categoryFromStatus(apiError.StatusCode)
	}
	return ClassifyProvider(err)
}

const statusOverloaded = 529

func categoryFromStatus(status int) Category {
	switch status {
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return CategoryTimeout
	case http.StatusTooManyRequests:
		return CategoryRateLimit
	case http.StatusServiceUnavailable, statusOverloaded:
		return CategoryOverload
	}
	if status >= 400 && status < 500 {
		return CategoryFailedDependency
	}
	return CategoryUpstream
}

func classifyTransport(err error) (Category, bool) {
	if errors.Is(err, ErrEgressConnection) {
		return CategoryUpstream, true
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		if urlError.Timeout() {
			return CategoryTimeout, true
		}
		return CategoryUpstream, true
	}
	var netError net.Error
	if errors.As(err, &netError) {
		if netError.Timeout() {
			return CategoryTimeout, true
		}
		return CategoryUpstream, true
	}
	return 0, false
}
