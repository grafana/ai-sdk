package v4

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

const (
	maxErrorResponseBytes     = 64 << 10
	statusClientClosedRequest = 499
)

type errorType string
type errorCode string

const (
	errorInvalidRequest errorType = "invalid_request_error"
	errorAuthentication errorType = "authentication_error"
	errorForbidden      errorType = "forbidden"
	errorModelNotFound  errorType = "model_not_found"
	errorRateLimit      errorType = "rate_limit_exceeded"
	errorInternal       errorType = "internal_server_error"
	errorDependency     errorType = "failed_dependency"

	codeInvalidRequest errorCode = "invalid_request"
	codeAuthentication errorCode = "authentication_error"
	codeForbidden      errorCode = "forbidden"
	codeModelNotFound  errorCode = "model_not_found"
	codeRateLimit      errorCode = "rate_limit_exceeded"
	codeOverload       errorCode = "overloaded"
	codeDependency     errorCode = "failed_dependency"
	codeUpstream       errorCode = "upstream_error"
	codeTimeout        errorCode = "timeout"
	codeCancellation   errorCode = "canceled"
	codeInternal       errorCode = "internal_error"
)

type errorDefinition struct {
	status    int
	message   string
	kind      errorType
	code      errorCode
	retryable bool
}

var publicErrors = [...]errorDefinition{
	safeInvalidRequest:   {http.StatusBadRequest, "invalid request", errorInvalidRequest, codeInvalidRequest, false},
	safeModelNotFound:    {http.StatusNotFound, "model not found", errorModelNotFound, codeModelNotFound, false},
	safeRateLimit:        {http.StatusTooManyRequests, "rate limit exceeded", errorRateLimit, codeRateLimit, true},
	safeOverload:         {http.StatusServiceUnavailable, "service overloaded", errorInternal, codeOverload, true},
	safeFailedDependency: {http.StatusFailedDependency, "failed dependency", errorDependency, codeDependency, false},
	safeUpstream:         {http.StatusBadGateway, "upstream failure", errorInternal, codeUpstream, true},
	safeTimeout:          {http.StatusGatewayTimeout, "request timed out", errorInternal, codeTimeout, true},
	safeCancellation:     {statusClientClosedRequest, "request canceled", errorInternal, codeCancellation, false},
	safeInternal:         {http.StatusInternalServerError, "internal error", errorInternal, codeInternal, true},
	safeAuthentication:   {http.StatusUnauthorized, "authentication failed", errorAuthentication, codeAuthentication, false},
	safePermission:       {http.StatusForbidden, "forbidden", errorForbidden, codeForbidden, false},
}

type errorResponse struct {
	Type     provider.StreamPartType   `json:"type,omitempty"`
	Error    publicError               `json:"error"`
	Metadata provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

type publicError struct {
	Message    string     `json:"message"`
	Type       errorType  `json:"type"`
	Param      *string    `json:"param"`
	Code       errorCode  `json:"code"`
	StatusCode int        `json:"statusCode,omitempty"`
	Retryable  *bool      `json:"retryable,omitempty"`
	Data       *errorData `json:"data,omitempty"`
}

type errorData struct {
	Metadata    provider.ProviderMetadata `json:"providerMetadata,omitzero"`
	NativeError *execution.Failure        `json:"nativeError,omitempty"`
}

func definitionForError(value safeError) errorDefinition {
	if value.category == 0 || int(value.category) >= len(publicErrors) {
		return publicErrors[safeInternal]
	}
	definition := publicErrors[value.category]
	if value.category == safeInvalidRequest {
		if message := unsupportedCapabilityMessage(value.capability); message != "" {
			definition.message = message
		}
	}
	return definition
}

func unsupportedCapabilityMessage(capability unsupportedCapability) string {
	switch capability {
	case capabilityReasoningContent:
		return "unsupported capability: reasoning-content"
	case capabilityCustomContent:
		return "unsupported capability: custom-content"
	case capabilityTools:
		return "unsupported capability: tools"
	case capabilityToolApprovals:
		return "unsupported capability: tool-approvals"
	case capabilityStructuredOutput:
		return "unsupported capability: structured-output"
	case capabilityRawOutput:
		return "unsupported capability: raw-output"
	case capabilityProviderOptions:
		return "unsupported capability: provider-options"
	case capabilityReservedProviderOptions:
		return "reserved provider option namespace"
	case capabilityProtectedCallHeader:
		return "protected call header"
	default:
		return ""
	}
}

func (definition errorDefinition) fields() publicError {
	return publicError{Message: definition.message, Type: definition.kind, Code: definition.code}
}

func encodeHTTPError(value safeError, metadata provider.ProviderMetadata, limit int64) (int, []byte) {
	definition := definitionForError(value)
	response := errorResponse{Error: definition.fields()}
	original, _ := json.Marshal(response)
	if len(metadata) == 0 || int64(len(original)) > limit {
		return definition.status, original
	}
	response.Metadata = metadata
	enriched, err := json.Marshal(response)
	if err != nil || int64(len(enriched)) > limit {
		return definition.status, original
	}
	return definition.status, enriched
}

func streamErrorResponse(value safeError) errorResponse {
	switch value.category {
	case safeRateLimit, safeOverload, safeFailedDependency, safeUpstream, safeTimeout, safeCancellation:
	default:
		value = safeError{category: safeInternal}
	}
	definition := definitionForError(value)
	fields := definition.fields()
	fields.StatusCode = definition.status
	fields.Retryable = &definition.retryable
	return errorResponse{Type: provider.PartError, Error: fields}
}

func encodeErrorFrame(response errorResponse) ([]byte, error) {
	body, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return append(append([]byte("data: "), body...), '\n', '\n'), nil
}

func streamErrorFrameForSafeError(value safeError) []byte {
	frame, _ := encodeErrorFrame(streamErrorResponse(value))
	return frame
}

func encodeStreamError(value safeError, metadata provider.ProviderMetadata, current *execution.Failure, limit int64) ([]byte, bool) {
	response := streamErrorResponse(value)
	original, _ := encodeErrorFrame(response)
	if int64(len(original)) > limit {
		return nil, false
	}
	accepted := original
	if current != nil {
		response.Error.Data = &errorData{NativeError: current}
		frame, err := encodeErrorFrame(response)
		if err != nil || int64(len(frame)) > limit {
			return original, true
		}
		accepted = frame
	}
	if len(metadata) != 0 {
		response.Error.Data = &errorData{Metadata: metadata, NativeError: current}
		if frame, err := encodeErrorFrame(response); err == nil && int64(len(frame)) <= limit {
			accepted = frame
		}
	}
	return accepted, true
}

func (h *handler) writeSafeError(w http.ResponseWriter, value safeError, metadata provider.ProviderMetadata) {
	status, body := encodeHTTPError(value, metadata, min(h.limits.UnaryResponseBytes, maxErrorResponseBytes))
	writeErrorResponse(w, status, body)
}

func writeErrorResponse(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
