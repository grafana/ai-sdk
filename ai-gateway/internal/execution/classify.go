package execution

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
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
// setup.
func ClassifyProvider(err error) (category Category) {
	defer func() {
		if recover() != nil {
			category = CategoryInternal
		}
	}()
	if isNilInterface(err) {
		return CategoryInternal
	}
	switch {
	case errors.Is(err, ErrTranslation), errors.Is(err, ErrInvalidOutput):
		return CategoryInternal
	case errors.Is(err, catalog.ErrUnsupportedRequest):
		return CategoryInvalidRequest
	case errors.Is(err, context.Canceled):
		return CategoryCancellation
	case errors.Is(err, context.DeadlineExceeded):
		return CategoryTimeout
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
			return CategoryUpstream
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
