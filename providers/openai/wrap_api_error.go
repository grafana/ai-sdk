package openai

import (
	"encoding/json"
	"errors"

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
		return err
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

	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           apiErr.Message,
		URL:               url,
		RequestBodyValues: reqBytes,
		StatusCode:        apiErr.StatusCode,
		ResponseHeaders:   headers,
		ResponseBody:      apiErr.RawJSON(),
		Data:              structuredErrorData(apiErr),
		Cause:             cause,
	})
}

// structuredErrorData returns the structured error object from the SDK error so
// the error type/code survives for downstream gateway normalization. The
// openai-go error's RawJSON is the inner error object
// ({"message":...,"type":...,"code":...}); if it is instead wrapped in an
// {"error":{...}} envelope, the inner object is extracted.
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
