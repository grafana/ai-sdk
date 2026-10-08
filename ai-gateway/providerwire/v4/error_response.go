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
	safeBYOKCredentials: {
		status:  http.StatusBadRequest,
		message: "providerOptions.gateway.byok requires supported provider arrays with valid accounts",
		kind:    errorInvalidRequest,
		code:    codeInvalidRequest,
	},
	safeBYOKSelector: {
		status:  http.StatusBadRequest,
		message: "BYOK requires a supported provider/model selector",
		kind:    errorInvalidRequest,
		code:    codeInvalidRequest,
	},
	safeGatewayControl: {
		status:  http.StatusBadRequest,
		message: "unsupported gateway control; only gateway.byok is supported",
		kind:    errorInvalidRequest,
		code:    codeInvalidRequest,
	},
	safeInvalidRequest: {
		status:    http.StatusBadRequest,
		message:   "invalid request",
		kind:      errorInvalidRequest,
		code:      codeInvalidRequest,
		retryable: false,
	},
	safeModelNotFound: {
		status:    http.StatusNotFound,
		message:   "model not found",
		kind:      errorModelNotFound,
		code:      codeModelNotFound,
		retryable: false,
	},
	safeRateLimit: {
		status:    http.StatusTooManyRequests,
		message:   "rate limit exceeded",
		kind:      errorRateLimit,
		code:      codeRateLimit,
		retryable: true,
	},
	safeOverload: {
		status:    http.StatusServiceUnavailable,
		message:   "service overloaded",
		kind:      errorInternal,
		code:      codeOverload,
		retryable: true,
	},
	safeFailedDependency: {
		status:    http.StatusFailedDependency,
		message:   "failed dependency",
		kind:      errorDependency,
		code:      codeDependency,
		retryable: false,
	},
	safeUpstream: {
		status:    http.StatusBadGateway,
		message:   "upstream failure",
		kind:      errorInternal,
		code:      codeUpstream,
		retryable: true,
	},
	safeTimeout: {
		status:    http.StatusGatewayTimeout,
		message:   "request timed out",
		kind:      errorInternal,
		code:      codeTimeout,
		retryable: true,
	},
	safeCancellation: {
		status:    statusClientClosedRequest,
		message:   "request canceled",
		kind:      errorInternal,
		code:      codeCancellation,
		retryable: false,
	},
	safeInternal: {
		status:    http.StatusInternalServerError,
		message:   "internal error",
		kind:      errorInternal,
		code:      codeInternal,
		retryable: true,
	},
	safeAuthentication: {
		status:    http.StatusUnauthorized,
		message:   "authentication failed",
		kind:      errorAuthentication,
		code:      codeAuthentication,
		retryable: false,
	},
	safePermission: {
		status:    http.StatusForbidden,
		message:   "forbidden",
		kind:      errorForbidden,
		code:      codeForbidden,
		retryable: false,
	},
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
		if message := requestFailureMessage(value.reason); message != "" {
			definition.message = message
		}
	}
	return definition
}

func requestFailureMessage(reason requestFailureReason) string {
	switch reason {
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
	case policyReservedProviderOptions:
		return "reserved provider option namespace"
	case policyProtectedCallHeader:
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
