package anthropic

import (
	"encoding/json"
	"errors"
	"net/http"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared"
	"github.com/grafana/ai-sdk/provider"
)

// wrapAPIError wraps an anthropic SDK API error into a *provider.APICallError.
// Non-API errors (network, DNS, etc.) pass through unwrapped, matching the
// existing api-call-error spec contract.
func wrapAPIError(err error, url string, body any) error {
	var apiErr *sdk.Error
	if !errors.As(err, &apiErr) {
		return err
	}

	var requestBody json.RawMessage
	if body != nil {
		if b, mErr := json.Marshal(body); mErr == nil {
			requestBody = b
		}
	}

	var responseHeaders map[string][]string
	if apiErr.Response != nil {
		responseHeaders = apiErr.Response.Header
	}

	var isRetryable *bool
	if apiErr.StatusCode == http.StatusOK {
		if metadata, known := streamErrorMetadata(apiErr.Type()); known {
			isRetryable = &metadata.retryable
		}
	}

	wrapped := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           apiErr.Error(),
		URL:               url,
		RequestBodyValues: requestBody,
		StatusCode:        apiErr.StatusCode,
		ResponseHeaders:   responseHeaders,
		ResponseBody:      apiErr.RawJSON(),
		IsRetryable:       isRetryable,
		Cause:             err,
	})
	wrapped.Data = structuredErrorData(apiErr.RawJSON())
	return wrapped
}

// streamErrorFrameStatus is the status given to an error frame whose type
// upstream does not classify. A first-chunk error becomes a failed call and
// defaults to a server error; a mid-stream error part keeps no status.
const (
	initialStreamErrorDefaultStatus = http.StatusInternalServerError
	midStreamErrorDefaultStatus     = 0
)

// errorTypeRequestTooLarge is not a named constant in the SDK.
const errorTypeRequestTooLarge shared.ErrorType = "request_too_large"

type errorMetadata struct {
	statusCode int
	retryable  bool
}

// streamErrorMetadata mirrors upstream getAnthropicStreamErrorMetadata.
func streamErrorMetadata(errorType shared.ErrorType) (errorMetadata, bool) {
	switch errorType {
	case shared.ErrorTypeAPIError:
		return errorMetadata{statusCode: http.StatusInternalServerError, retryable: true}, true
	case shared.ErrorTypeOverloadedError:
		return errorMetadata{statusCode: 529, retryable: true}, true
	case shared.ErrorTypeRateLimitError:
		return errorMetadata{statusCode: http.StatusTooManyRequests, retryable: true}, true
	case errorTypeRequestTooLarge:
		return errorMetadata{statusCode: http.StatusRequestEntityTooLarge}, true
	case shared.ErrorTypeAuthenticationError:
		return errorMetadata{statusCode: http.StatusUnauthorized}, true
	case shared.ErrorTypePermissionError:
		return errorMetadata{statusCode: http.StatusForbidden}, true
	case shared.ErrorTypeNotFoundError:
		return errorMetadata{statusCode: http.StatusNotFound}, true
	case shared.ErrorTypeBillingError, shared.ErrorTypeInvalidRequestError:
		return errorMetadata{statusCode: http.StatusBadRequest}, true
	}
	return errorMetadata{}, false
}

func wrapInitialStreamError(err error, body any) error {
	return wrapErrorFrame(wrapAPIError(err, "", body), err, initialStreamErrorDefaultStatus)
}

// wrapStreamErrorFrame wraps an error frame received after the stream started.
func wrapStreamErrorFrame(err error) *provider.APICallError {
	wrapped := wrapAsAPICallError(err, "", nil)
	if framed, ok := wrapErrorFrame(wrapped, err, midStreamErrorDefaultStatus).(*provider.APICallError); ok {
		return framed
	}
	return wrapped
}

// wrapErrorFrame gives an Anthropic error frame upstream's classification: the
// inner error message, and the status code and retryability of its type.
func wrapErrorFrame(wrapped error, err error, defaultStatus int) error {
	var callErr *provider.APICallError
	if !errors.As(wrapped, &callErr) {
		return wrapped
	}

	var anthropicErr *sdk.Error
	if !errors.As(err, &anthropicErr) || anthropicErr.StatusCode != http.StatusOK {
		return callErr
	}

	metadata, known := streamErrorMetadata(anthropicErr.Type())
	callErr.StatusCode = defaultStatus
	callErr.IsRetryable = false
	if known {
		callErr.StatusCode = metadata.statusCode
		callErr.IsRetryable = metadata.retryable
	}
	if anthropicErr.Request != nil && anthropicErr.Request.URL != nil {
		callErr.URL = anthropicErr.Request.URL.String()
	}

	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal([]byte(anthropicErr.RawJSON()), &envelope) == nil && len(envelope.Error) > 0 {
		callErr.ResponseBody = string(envelope.Error)
		var details struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(envelope.Error, &details) == nil && details.Message != "" {
			callErr.Message = details.Message
		}
	}
	return callErr
}

// structuredErrorData returns the parsed structured error envelope as
// json.RawMessage when raw is a recognizable Anthropic error envelope
// ({"type":"error","error":{"type":..,"message":..}}), so the provider error
// type is preserved in APICallError.Data for the gateway normalizer. It returns
// nil when raw is empty or does not carry a structured error type, leaving the
// raw body available in ResponseBody.
func structuredErrorData(raw string) json.RawMessage {
	if raw == "" {
		return nil
	}
	var env struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil || env.Error.Type == "" {
		return nil
	}
	return json.RawMessage(raw)
}

// wrapAsAPICallError forces wrapping into *provider.APICallError, even for
// non-API errors. Use this when emitting a [provider.PartError] stream part,
// where the wire requires an APICallError so retryability and HTTP status
// cross the boundary.
func wrapAsAPICallError(err error, url string, body any) *provider.APICallError {
	wrapped := wrapAPIError(err, url, body)
	if apiErr, ok := wrapped.(*provider.APICallError); ok {
		return apiErr
	}

	var requestBody json.RawMessage
	if body != nil {
		if b, mErr := json.Marshal(body); mErr == nil {
			requestBody = b
		}
	}

	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           err.Error(),
		URL:               url,
		RequestBodyValues: requestBody,
		Cause:             err,
	})
}
