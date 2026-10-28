package v4

import "github.com/grafana/ai-sdk/ai-gateway/internal/execution"

type safeErrorCategory uint8

const (
	safeInvalidRequest safeErrorCategory = iota + 1
	safeBYOKCredentials
	safeBYOKSelector
	safeGatewayControl
	safeModelNotFound
	safeRateLimit
	safeOverload
	safeFailedDependency
	safeUpstream
	safeTimeout
	safeCancellation
	safeInternal
	safePermission
	safeBYOKDiscovery
	safeRoutingControls
	safeProviderTimeouts
	safeNoCandidates
	safeAttemptBound
	safeFallbackModelNotFound
)

type safeError struct {
	category safeErrorCategory
	reason   requestFailureReason
}

var safeCategories = [...]safeErrorCategory{
	execution.CategoryInvalidRequest:   safeInvalidRequest,
	execution.CategoryBYOKCredentials:  safeBYOKCredentials,
	execution.CategoryBYOKSelector:     safeBYOKSelector,
	execution.CategoryGatewayControl:   safeGatewayControl,
	execution.CategoryModelNotFound:    safeModelNotFound,
	execution.CategoryPermission:       safePermission,
	execution.CategoryRateLimit:        safeRateLimit,
	execution.CategoryOverload:         safeOverload,
	execution.CategoryFailedDependency: safeFailedDependency,
	execution.CategoryUpstream:         safeUpstream,
	execution.CategoryTimeout:          safeTimeout,
	execution.CategoryCancellation:     safeCancellation,
	execution.CategoryInternal:         safeInternal,
}

var routingErrors = map[execution.Reason]safeErrorCategory{
	execution.ReasonRoutingControls:  safeRoutingControls,
	execution.ReasonProviderTimeouts: safeProviderTimeouts,
	execution.ReasonNoCandidates:     safeNoCandidates,
	execution.ReasonAttemptBound:     safeAttemptBound,
}

func safeErrorFor(category execution.Category, reason execution.Reason) safeError {
	switch {
	case category == execution.CategoryInvalidRouting:
		routing, ok := routingErrors[reason]
		if !ok {
			return safeError{category: safeInternal}
		}
		return safeError{category: routing}
	case category == execution.CategoryModelNotFound && reason == execution.ReasonFallbackModel:
		return safeError{category: safeFallbackModelNotFound}
	}
	if int(category) >= len(safeCategories) || safeCategories[category] == 0 {
		return safeError{category: safeInternal}
	}
	value := safeError{category: safeCategories[category]}
	switch reason {
	case execution.ReasonReservedProviderOptions:
		value.reason = policyReservedProviderOptions
	case execution.ReasonProtectedCallHeader:
		value.reason = policyProtectedCallHeader
	}
	return value
}

func safeErrorFromResolution(err error) safeError {
	return safeErrorFor(execution.ClassifySelection(err))
}

func safeErrorFromProvider(err error) safeError {
	return safeErrorFor(execution.ClassifyProvider(err), execution.ReasonNone)
}

func safeErrorFromStreamProvider(err error) safeError {
	return safeErrorFor(execution.ClassifyStreamPart(err), execution.ReasonNone)
}
