package v4

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

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
	safeAuthentication
	safePermission
)

type safeError struct {
	category safeErrorCategory
	reason   requestFailureReason
}

func safeErrorFromResolution(err error) (result safeError) {
	result = safeError{category: safeInternal}
	defer func() {
		if recover() != nil {
			result = safeError{category: safeInternal}
		}
	}()
	if isNilInterface(err) {
		return result
	}
	switch {
	case errors.Is(err, ErrInvalidBYOK):
		return safeError{category: safeBYOKCredentials}
	case errors.Is(err, ErrInvalidBYOKSelector):
		return safeError{category: safeBYOKSelector}
	case errors.Is(err, ErrUnsupportedGatewayControl):
		return safeError{category: safeGatewayControl}
	}
	if errors.Is(err, ErrReservedProviderOptions) {
		return safeError{category: safeInvalidRequest, reason: policyReservedProviderOptions}
	}
	if errors.Is(err, catalog.ErrUnknownModel) {
		return safeError{category: safeModelNotFound}
	}
	return safeErrorFromProvider(err)
}

func safeErrorFromProvider(err error) (result safeError) {
	result = safeError{category: safeInternal}
	defer func() {
		if recover() != nil {
			result = safeError{category: safeInternal}
		}
	}()
	if isNilInterface(err) {
		return result
	}
	switch {
	case errors.Is(err, catalog.ErrUnsupportedRequest):
		return safeError{category: safeInvalidRequest}
	case errors.Is(err, context.Canceled):
		return safeError{category: safeCancellation}
	case errors.Is(err, context.DeadlineExceeded):
		return safeError{category: safeTimeout}
	}

	var apiError *provider.APICallError
	if errors.As(err, &apiError) {
		switch apiError.StatusCode {
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return safeError{category: safeTimeout}
		case http.StatusTooManyRequests:
			return safeError{category: safeRateLimit}
		case http.StatusServiceUnavailable, 529:
			return safeError{category: safeOverload}
		}
		if apiError.StatusCode >= 400 && apiError.StatusCode < 500 {
			return safeError{category: safeFailedDependency}
		}
		return safeError{category: safeUpstream}
	}

	if transportError, ok := safeErrorFromTransport(err); ok {
		return transportError
	}
	if onlyUnsupportedFunctionality(err) {
		return safeError{category: safeInvalidRequest}
	}
	return safeError{category: safeInternal}
}

// onlyUnsupportedFunctionality reports whether every failure in the error tree
// is the provider unsupported-functionality rejection. errors.Is would match a
// join with any such branch, which would hide a retryable operational failure
// behind a non-retryable invalid-request response.
func onlyUnsupportedFunctionality(err error) bool {
	if err == nil {
		return false
	}
	if err == provider.ErrUnsupportedFunctionality {
		return true
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		branches := wrapped.Unwrap()
		for _, branch := range branches {
			if !onlyUnsupportedFunctionality(branch) {
				return false
			}
		}
		return len(branches) > 0
	case interface{ Unwrap() error }:
		return onlyUnsupportedFunctionality(wrapped.Unwrap())
	default:
		return false
	}
}

func safeErrorFromTransport(err error) (safeError, bool) {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		if urlError.Timeout() {
			return safeError{category: safeTimeout}, true
		}
		return safeError{category: safeUpstream}, true
	}
	var netError net.Error
	if errors.As(err, &netError) {
		if netError.Timeout() {
			return safeError{category: safeTimeout}, true
		}
		return safeError{category: safeUpstream}, true
	}
	return safeError{}, false
}
