package openai

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"

	"github.com/grafana/ai-sdk/provider"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// wrapAPIError converts an OpenAI SDK error into a *provider.APICallError.
// Non-API errors (network/DNS) are returned unwrapped so callers can inspect
// them directly. The requestBody is marshaled into RequestBodyValues for
// debugging.
func wrapAPIError(err error, requestBody responses.ResponseNewParams) error {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		if !isRetryableNetworkError(err) {
			return err
		}
		requestJSON, _ := json.Marshal(requestBody)
		retryable := true
		var requestURL string
		var urlError *url.Error
		if errors.As(err, &urlError) {
			requestURL = urlError.URL
		}
		return provider.NewAPICallError(provider.APICallErrorOptions{
			Message:           err.Error(),
			URL:               requestURL,
			RequestBodyValues: requestJSON,
			IsRetryable:       &retryable,
			Cause:             err,
		})
	}
	return buildAPICallError(apiErr, requestBody, err)
}

// wrapAsAPICallError forces wrapping of any error into a *provider.APICallError,
// used when emitting PartError stream parts (the wire requires an APICallError
// so retryability and status cross the boundary).
func wrapAsAPICallError(err error) *provider.APICallError {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return buildAPICallError(apiErr, responses.ResponseNewParams{}, err)
	}
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message: err.Error(),
		Cause:   err,
	})
}

func buildAPICallError(apiErr *openai.Error, requestBody responses.ResponseNewParams, cause error) *provider.APICallError {
	var reqBytes json.RawMessage
	if b, mErr := json.Marshal(requestBody); mErr == nil {
		reqBytes = b
	}

	var url string
	var headers map[string][]string
	if apiErr.Request != nil {
		url = apiErr.Request.URL.String()
	}
	if apiErr.Response != nil {
		headers = apiErr.Response.Header
	}

	data := structuredErrorData(apiErr)
	var details struct {
		Type string `json:"type"`
		Code any    `json:"code"`
	}
	_ = json.Unmarshal(data, &details)
	var retryable *bool
	if details.Type == "insufficient_quota" || details.Code == "insufficient_quota" {
		value := false
		retryable = &value
	}

	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           apiErr.Message,
		Type:              details.Type,
		Code:              details.Code,
		URL:               url,
		RequestBodyValues: reqBytes,
		StatusCode:        apiErr.StatusCode,
		ResponseHeaders:   headers,
		ResponseBody:      apiErr.RawJSON(),
		IsRetryable:       retryable,
		Data:              data,
		Cause:             cause,
	})
}

// structuredErrorData returns the structured error object from the SDK error so
// the error type/code survives for downstream gateway normalization. The
// openai-go error's RawJSON is the inner error object
// ({"message":...,"type":...,"code":...}); if it is instead wrapped in an
// {"error":{...}} envelope, the inner object is extracted.
func isRetryableNetworkError(err error) bool {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError)
}

func structuredErrorData(apiErr *openai.Error) json.RawMessage {
	raw := apiErr.RawJSON()
	if raw == "" {
		return nil
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil && len(envelope.Error) > 0 {
		return envelope.Error
	}
	return json.RawMessage(raw)
}
