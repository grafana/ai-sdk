package v4

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

type safeErrorCategory uint8

const (
	safeInvalidRequest safeErrorCategory = iota + 1
	safeModelNotFound
	safeRateLimit
	safeOverload
	safeFailedDependency
	safeUpstream
	safeTimeout
	safeCancellation
	safeInternal
)

type safeError struct {
	category   safeErrorCategory
	capability unsupportedCapability
}

type safeErrorDocument struct {
	status int
	body   []byte
}

var (
	canonicalInvalidRequestError = []byte(`{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	canonicalModelNotFoundError  = []byte(`{"error":{"message":"model not found","type":"model_not_found","param":null,"code":"model_not_found"}}`)
	canonicalRateLimitError      = []byte(`{"error":{"message":"rate limit exceeded","type":"rate_limit_exceeded","param":null,"code":"rate_limit_exceeded"}}`)
	canonicalOverloadError       = []byte(`{"error":{"message":"service overloaded","type":"internal_server_error","param":null,"code":"overloaded"}}`)
	canonicalDependencyError     = []byte(`{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`)
	canonicalUpstreamError       = []byte(`{"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error"}}`)
	canonicalTimeoutError        = []byte(`{"error":{"message":"request timed out","type":"internal_server_error","param":null,"code":"timeout"}}`)
	canonicalCancellationError   = []byte(`{"error":{"message":"request canceled","type":"internal_server_error","param":null,"code":"canceled"}}`)
	canonicalInternalError       = []byte(`{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`)

	unsupportedFilesError            = []byte(`{"error":{"message":"unsupported capability: files","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedReasoningContentError = []byte(`{"error":{"message":"unsupported capability: reasoning-content","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedCustomContentError    = []byte(`{"error":{"message":"unsupported capability: custom-content","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedToolsError            = []byte(`{"error":{"message":"unsupported capability: tools","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedToolApprovalsError    = []byte(`{"error":{"message":"unsupported capability: tool-approvals","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedStructuredOutputError = []byte(`{"error":{"message":"unsupported capability: structured-output","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedProviderOptionsError  = []byte(`{"error":{"message":"unsupported capability: provider-options","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedBodyHeadersError      = []byte(`{"error":{"message":"unsupported capability: body-headers","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedRawOutputError        = []byte(`{"error":{"message":"unsupported capability: raw-output","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
)

func documentForSafeError(value safeError) safeErrorDocument {
	switch value.category {
	case safeInvalidRequest:
		body := unsupportedCapabilityDocument(value.capability)
		if body == nil {
			body = canonicalInvalidRequestError
		}
		return safeErrorDocument{status: http.StatusBadRequest, body: body}
	case safeModelNotFound:
		return safeErrorDocument{status: http.StatusNotFound, body: canonicalModelNotFoundError}
	case safeRateLimit:
		return safeErrorDocument{status: http.StatusTooManyRequests, body: canonicalRateLimitError}
	case safeOverload:
		return safeErrorDocument{status: http.StatusServiceUnavailable, body: canonicalOverloadError}
	case safeFailedDependency:
		return safeErrorDocument{status: http.StatusFailedDependency, body: canonicalDependencyError}
	case safeUpstream:
		return safeErrorDocument{status: http.StatusBadGateway, body: canonicalUpstreamError}
	case safeTimeout:
		return safeErrorDocument{status: http.StatusGatewayTimeout, body: canonicalTimeoutError}
	case safeCancellation:
		return safeErrorDocument{status: 499, body: canonicalCancellationError}
	default:
		return safeErrorDocument{status: http.StatusInternalServerError, body: canonicalInternalError}
	}
}

func unsupportedCapabilityDocument(capability unsupportedCapability) []byte {
	switch capability {
	case capabilityFiles:
		return unsupportedFilesError
	case capabilityReasoningContent:
		return unsupportedReasoningContentError
	case capabilityCustomContent:
		return unsupportedCustomContentError
	case capabilityTools:
		return unsupportedToolsError
	case capabilityToolApprovals:
		return unsupportedToolApprovalsError
	case capabilityStructuredOutput:
		return unsupportedStructuredOutputError
	case capabilityProviderOptions:
		return unsupportedProviderOptionsError
	case capabilityBodyHeaders:
		return unsupportedBodyHeadersError
	case capabilityRawOutput:
		return unsupportedRawOutputError
	default:
		return nil
	}
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

	var urlError *url.Error
	if errors.As(err, &urlError) {
		if urlError.Timeout() {
			return safeError{category: safeTimeout}
		}
		return safeError{category: safeUpstream}
	}
	var netError net.Error
	if errors.As(err, &netError) {
		if netError.Timeout() {
			return safeError{category: safeTimeout}
		}
		return safeError{category: safeUpstream}
	}
	return safeError{category: safeInternal}
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

func (h *handler) writeSafeError(w http.ResponseWriter, value safeError) {
	document := documentForSafeError(value)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(document.status)
	_, _ = w.Write(document.body)
}
