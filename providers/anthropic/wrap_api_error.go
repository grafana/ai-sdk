package anthropic

import (
	"encoding/json"
	"errors"
	"net/http"

	sdk "github.com/anthropics/anthropic-sdk-go"
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

	statusCode := apiErr.StatusCode
	var isRetryable *bool
	if statusCode == http.StatusOK {
		inferredStatus, inferredRetryable := anthropicStreamErrorMetadata(string(apiErr.Type()))
		if inferredStatus != 0 {
			statusCode = inferredStatus
			isRetryable = &inferredRetryable
		}
	}

	wrapped := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           apiErr.Error(),
		Type:              string(apiErr.Type()),
		URL:               url,
		RequestBodyValues: requestBody,
		StatusCode:        statusCode,
		ResponseHeaders:   responseHeaders,
		ResponseBody:      apiErr.RawJSON(),
		IsRetryable:       isRetryable,
		Cause:             err,
	})
	wrapped.Data = structuredErrorData(apiErr.RawJSON())
	return wrapped
}

func wrapInitialStreamError(err error, body any) error {
	wrapped := wrapAPIError(err, "", body)

	var callErr *provider.APICallError
	if !errors.As(wrapped, &callErr) {
		return wrapped
	}

	var anthropicErr *sdk.Error
	if !errors.As(err, &anthropicErr) || anthropicErr.StatusCode != 200 {
		return callErr
	}

	callErr.StatusCode, callErr.IsRetryable = anthropicStreamErrorMetadata(string(anthropicErr.Type()))
	if callErr.StatusCode == 0 {
		callErr.StatusCode = http.StatusInternalServerError
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

func anthropicStreamErrorMetadata(errorType string) (int, bool) {
	switch errorType {
	case "api_error":
		return http.StatusInternalServerError, true
	case "overloaded_error":
		return 529, true
	case "rate_limit_error":
		return http.StatusTooManyRequests, true
	case "request_too_large":
		return http.StatusRequestEntityTooLarge, false
	case "authentication_error":
		return http.StatusUnauthorized, false
	case "permission_error":
		return http.StatusForbidden, false
	case "not_found_error":
		return http.StatusNotFound, false
	case "billing_error", "invalid_request_error":
		return http.StatusBadRequest, false
	default:
		return 0, false
	}
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
