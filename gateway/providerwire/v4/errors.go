package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

var (
	// ErrPolicyAuthentication reports that host policy could not authenticate the caller.
	ErrPolicyAuthentication = errors.New("providerwire v4: policy authentication failed")
	// ErrPolicyPermission reports that host policy denied the caller.
	ErrPolicyPermission = errors.New("providerwire v4: policy permission denied")
	// ErrPolicyRateLimit reports that host policy rate-limited the caller.
	ErrPolicyRateLimit = errors.New("providerwire v4: policy rate limit exceeded")
	// ErrPolicyOverload reports that host policy is temporarily overloaded.
	ErrPolicyOverload = errors.New("providerwire v4: policy overloaded")
)

type safeErrorCategory uint8

const (
	safeInvalidRequest safeErrorCategory = iota + 1
	safeAuthentication
	safePermission
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

type safeErrorType string

type safeErrorCode string

const (
	errorTypeInvalidRequest safeErrorType = "invalid_request_error"
	errorTypeAuthentication safeErrorType = "authentication_error"
	errorTypeForbidden      safeErrorType = "forbidden"
	errorTypeModelNotFound  safeErrorType = "model_not_found"
	errorTypeRateLimit      safeErrorType = "rate_limit_exceeded"
	errorTypeInternal       safeErrorType = "internal_server_error"
	errorTypeDependency     safeErrorType = "failed_dependency"

	errorCodeInvalidRequest safeErrorCode = "invalid_request"
	errorCodeAuthentication safeErrorCode = "authentication_error"
	errorCodeForbidden      safeErrorCode = "forbidden"
	errorCodeModelNotFound  safeErrorCode = "model_not_found"
	errorCodeRateLimit      safeErrorCode = "rate_limit_exceeded"
	errorCodeOverloaded     safeErrorCode = "overloaded"
	errorCodeDependency     safeErrorCode = "failed_dependency"
	errorCodeUpstream       safeErrorCode = "upstream_error"
	errorCodeTimeout        safeErrorCode = "timeout"
	errorCodeCanceled       safeErrorCode = "canceled"
	errorCodeInternal       safeErrorCode = "internal_error"
)

type safeErrorDefinition struct {
	status   int
	message  string
	typeName safeErrorType
	code     safeErrorCode
}

func definitionForSafeError(value safeError) (safeErrorDefinition, bool) {
	switch value.category {
	case safeInvalidRequest:
		message := "invalid request"
		if value.capability != "" {
			if !validUnsupportedCapability(value.capability) {
				return safeErrorDefinition{}, false
			}
			message = "unsupported capability: " + string(value.capability)
		}
		return safeErrorDefinition{status: http.StatusBadRequest, message: message, typeName: errorTypeInvalidRequest, code: errorCodeInvalidRequest}, true
	case safeAuthentication:
		return safeErrorDefinition{status: http.StatusUnauthorized, message: "authentication failed", typeName: errorTypeAuthentication, code: errorCodeAuthentication}, true
	case safePermission:
		return safeErrorDefinition{status: http.StatusForbidden, message: "forbidden", typeName: errorTypeForbidden, code: errorCodeForbidden}, true
	case safeModelNotFound:
		return safeErrorDefinition{status: http.StatusNotFound, message: "model not found", typeName: errorTypeModelNotFound, code: errorCodeModelNotFound}, true
	case safeRateLimit:
		return safeErrorDefinition{status: http.StatusTooManyRequests, message: "rate limit exceeded", typeName: errorTypeRateLimit, code: errorCodeRateLimit}, true
	case safeOverload:
		return safeErrorDefinition{status: http.StatusServiceUnavailable, message: "service overloaded", typeName: errorTypeInternal, code: errorCodeOverloaded}, true
	case safeFailedDependency:
		return safeErrorDefinition{status: http.StatusFailedDependency, message: "failed dependency", typeName: errorTypeDependency, code: errorCodeDependency}, true
	case safeUpstream:
		return safeErrorDefinition{status: http.StatusBadGateway, message: "upstream failure", typeName: errorTypeInternal, code: errorCodeUpstream}, true
	case safeTimeout:
		return safeErrorDefinition{status: http.StatusGatewayTimeout, message: "request timed out", typeName: errorTypeInternal, code: errorCodeTimeout}, true
	case safeCancellation:
		return safeErrorDefinition{status: 499, message: "request canceled", typeName: errorTypeInternal, code: errorCodeCanceled}, true
	case safeInternal:
		return safeErrorDefinition{status: http.StatusInternalServerError, message: "internal error", typeName: errorTypeInternal, code: errorCodeInternal}, true
	default:
		return safeErrorDefinition{}, false
	}
}

func safeErrorFromPolicy(err error) (result safeError) {
	result = safeError{category: safeInternal}
	defer func() {
		if recover() != nil {
			result = safeError{category: safeInternal}
		}
	}()
	if isNil(err) {
		return safeError{category: safeInternal}
	}
	switch {
	case errors.Is(err, context.Canceled):
		return safeError{category: safeCancellation}
	case errors.Is(err, context.DeadlineExceeded):
		return safeError{category: safeTimeout}
	case errors.Is(err, ErrPolicyAuthentication):
		return safeError{category: safeAuthentication}
	case errors.Is(err, ErrPolicyPermission):
		return safeError{category: safePermission}
	case errors.Is(err, ErrPolicyRateLimit):
		return safeError{category: safeRateLimit}
	case errors.Is(err, ErrPolicyOverload):
		return safeError{category: safeOverload}
	default:
		return safeError{category: safeInternal}
	}
}

func safeErrorFromResolution(err error) (result safeError) {
	result = safeError{category: safeInternal}
	defer func() {
		if recover() != nil {
			result = safeError{category: safeInternal}
		}
	}()
	if isNil(err) {
		return safeError{category: safeInternal}
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
	if isNil(err) {
		return safeError{category: safeInternal}
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

type boundedDocument struct {
	data     []byte
	limit    int64
	overflow bool
	invalid  bool
}

func newBoundedDocument(limit int64) boundedDocument {
	capacity := 256
	if limit < int64(capacity) {
		capacity = int(limit)
	}
	return boundedDocument{data: make([]byte, 0, capacity), limit: limit}
}

func (b *boundedDocument) append(value string) {
	remaining := b.limit - int64(len(b.data))
	if b.overflow || b.invalid || remaining < 0 || int64(len(value)) > remaining {
		b.overflow = true
		return
	}
	b.data = append(b.data, value...)
}

func (b *boundedDocument) appendBytes(value []byte) {
	remaining := b.limit - int64(len(b.data))
	if b.overflow || b.invalid || remaining < 0 || int64(len(value)) > remaining {
		b.overflow = true
		return
	}
	b.data = append(b.data, value...)
}

func (b *boundedDocument) appendJSONString(value string) {
	if b.overflow || b.invalid {
		return
	}
	if !utf8.ValidString(value) {
		b.invalid = true
		return
	}
	b.append(`"`)
	const hex = "0123456789abcdef"
	for i := 0; i < len(value) && !b.overflow; {
		c := value[i]
		switch c {
		case '"', '\\':
			b.appendBytes([]byte{'\\', c})
			i++
		case '\b':
			b.append(`\b`)
			i++
		case '\f':
			b.append(`\f`)
			i++
		case '\n':
			b.append(`\n`)
			i++
		case '\r':
			b.append(`\r`)
			i++
		case '\t':
			b.append(`\t`)
			i++
		default:
			if c < 0x20 {
				b.appendBytes([]byte{'\\', 'u', '0', '0', hex[c>>4], hex[c&0x0f]})
				i++
				continue
			}
			_, size := utf8.DecodeRuneInString(value[i:])
			b.append(value[i : i+size])
			i += size
		}
	}
	b.append(`"`)
}

func encodeSafeError(value safeError, limit int64) ([]byte, int, bool) {
	definition, ok := definitionForSafeError(value)
	if !ok {
		return nil, 0, false
	}
	buffer := newBoundedDocument(limit)
	buffer.append(`{"error":{"message":`)
	buffer.appendJSONString(definition.message)
	buffer.append(`,"type":`)
	buffer.appendJSONString(string(definition.typeName))
	buffer.append(`,"param":null,"code":`)
	buffer.appendJSONString(string(definition.code))
	buffer.append(`}}`)
	if buffer.overflow || buffer.invalid {
		return nil, 0, false
	}
	return buffer.data, definition.status, true
}

func (h *handler) writeSafeError(w http.ResponseWriter, value safeError) {
	body, status, ok := encodeSafeError(value, h.limits.ErrorResponseBytes)
	if !ok || h.errorSchema.Validate(json.RawMessage(body)) != nil {
		body = canonicalInternalError
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
