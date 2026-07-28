package bedrock

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// wrapAPIError builds an `*provider.APICallError` from a non-2xx Bedrock
// response. The body is parsed as `{message, type}` (Converse's error
// envelope) when possible. Retryability is inferred from the status code
// and known exception type names.
func wrapAPIError(statusCode int, url, requestBody string, responseHeaders map[string][]string, responseBody []byte) *provider.APICallError {
	var berr converseError
	_ = json.Unmarshal(responseBody, &berr) // best-effort

	retryable := isRetryableStatus(statusCode) || isRetryableBedrockErrorType(berr.Type)

	msg := berr.Message
	if msg == "" {
		msg = strings.TrimSpace(string(responseBody))
	}
	if msg == "" {
		msg = fmt.Sprintf("bedrock: HTTP %d", statusCode)
	}

	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           msg,
		StatusCode:        statusCode,
		URL:               url,
		RequestBodyValues: []byte(requestBody),
		ResponseHeaders:   responseHeaders,
		ResponseBody:      string(responseBody),
		IsRetryable:       &retryable,
	})
}

// isRetryableStatus mirrors upstream's auto-retry heuristic: 429 and 5xx
// (except 501) are retryable. 408 conflict and 409 lock contention are also
// retryable.
func isRetryableStatus(code int) bool {
	switch code {
	case 408, 409, 429:
		return true
	}
	if code >= 500 && code != 501 {
		return true
	}
	return false
}

// isRetryableBedrockErrorType returns true for Bedrock exception names that
// represent transient failures (throttling, capacity, model errors). These
// override status-code heuristics for cases where Bedrock returns a 400
// with a throttling-style message.
func isRetryableBedrockErrorType(typ string) bool {
	switch typ {
	case "ThrottlingException",
		"throttlingException",
		"InternalServerException",
		"internalServerException",
		"ModelStreamErrorException",
		"modelStreamErrorException",
		"ServiceUnavailableException",
		"serviceUnavailableException":
		return true
	}
	return false
}
