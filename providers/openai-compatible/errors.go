package openaicompatible

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/provider"
)

func wrapAPIError(status int, endpoint string, requestBody []byte, headers http.Header, responseBody []byte) *provider.APICallError {
	message := fmt.Sprintf("openai: API request failed with status %d", status)
	var data json.RawMessage
	var errorType string
	var errorCode any

	var parsed openAIErrorResponse
	if err := json.Unmarshal(responseBody, &parsed); err == nil && parsed.Error.Message != "" {
		message = "openai: " + parsed.Error.Message
		if b, err := json.Marshal(parsed.Error); err == nil {
			data = b
		}
		errorType = parsed.Error.Type
		errorCode = parsed.Error.Code
	}

	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           message,
		Type:              errorType,
		Code:              errorCode,
		URL:               endpoint,
		RequestBodyValues: json.RawMessage(append([]byte(nil), requestBody...)),
		StatusCode:        status,
		ResponseHeaders:   cloneHeaders(headers),
		ResponseBody:      string(responseBody),
		Data:              data,
	})
}

func transportError(endpoint string, requestBody []byte, err error) *provider.APICallError {
	retryable := true
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           fmt.Sprintf("openai: HTTP request failed: %v", err),
		URL:               endpoint,
		RequestBodyValues: json.RawMessage(append([]byte(nil), requestBody...)),
		IsRetryable:       &retryable,
		Cause:             err,
	})
}

func streamDecodeError(endpoint string, err error) *provider.APICallError {
	retryable := true
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     fmt.Sprintf("openai: stream decode failure: %v", err),
		URL:         endpoint,
		IsRetryable: &retryable,
		Cause:       err,
	})
}

func cloneHeaders(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string][]string, len(headers))
	for k, values := range headers {
		out[k] = append([]string(nil), values...)
	}
	return out
}
