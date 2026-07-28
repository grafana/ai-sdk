package providerwire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"

	"github.com/grafana/ai-sdk/provider"
)

// EncodeAPICallError serializes an [provider.APICallError] into the upstream
// Vercel AI SDK gateway error envelope: `{"error": {<APICallError fields>}}`.
// The upstream gateway error parser (`createGatewayErrorFromResponse`) requires
// `error.message` (present here) to surface the real message instead of
// "Invalid error response format"; the remaining APICallError fields
// (statusCode, isRetryable, ...) are ignored by the upstream client but
// preserved for Go-to-Go round-trips.
//
// A synthetic `type` is intentionally NOT injected: the upstream gateway treats
// a missing `type` as an internal_server_error while still surfacing the
// message, and injecting one would pollute the response body that the Go client
// parses for provider-structured error categorization.
//
// The unexported cause field does not survive the wire by design; consumers
// should use Message, StatusCode, ResponseBody, and IsRetryable for cross-
// process error attribution. See openspec change
// provider-wire-upstream-full-compat.
func EncodeAPICallError(err *provider.APICallError) ([]byte, error) {
	if err == nil {
		return nil, fmt.Errorf("wire: nil APICallError")
	}
	inner, jerr := json.Marshal(err)
	if jerr != nil {
		return nil, fmt.Errorf("wire: encoding APICallError: %w", jerr)
	}
	envelope, jerr := json.Marshal(map[string]json.RawMessage{"error": inner})
	if jerr != nil {
		return nil, fmt.Errorf("wire: encoding error envelope: %w", jerr)
	}
	return envelope, nil
}

// DecodeAPICallError deserializes a [provider.APICallError] from the wire. It
// accepts both the upstream wrapped envelope (`{"error": {...}}`) and the
// legacy bare `APICallError` object. The decoded value's Unwrap() returns nil
// because the cause field is not transmitted; IsRetryable, StatusCode, and
// Message are preserved. See openspec change provider-wire-upstream-full-compat.
func DecodeAPICallError(data []byte) (*provider.APICallError, error) {
	// Prefer the wrapped envelope when an `error` object is present.
	var wrapped struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Error) > 0 {
		if trimmed := bytes.TrimSpace(wrapped.Error); len(trimmed) > 0 && trimmed[0] == '{' {
			var apiErr provider.APICallError
			if err := json.Unmarshal(wrapped.Error, &apiErr); err != nil {
				return nil, fmt.Errorf("wire: decoding APICallError: %w", err)
			}
			return &apiErr, nil
		}
	}
	var apiErr provider.APICallError
	if err := json.Unmarshal(data, &apiErr); err != nil {
		return nil, fmt.Errorf("wire: decoding APICallError: %w", err)
	}
	return &apiErr, nil
}

// WriteErrorResponse writes err as a non-2xx JSON HTTP response body. The
// error is encoded before headers are committed; if the underlying writer fails
// after WriteHeader, callers may still observe a partially written response.
func WriteErrorResponse(w http.ResponseWriter, err *provider.APICallError) error {
	if err == nil {
		return fmt.Errorf("wire: nil APICallError")
	}
	statusCode, statusErr := errorResponseStatusCode(err)
	if statusErr != nil {
		return statusErr
	}
	data, jerr := EncodeAPICallError(err)
	if jerr != nil {
		return jerr
	}
	w.Header().Set("Content-Type", MIMEJSON)
	w.WriteHeader(statusCode)
	if _, werr := w.Write(data); werr != nil {
		return fmt.Errorf("wire: writing error response: %w", werr)
	}
	return nil
}

func errorResponseStatusCode(err *provider.APICallError) (int, error) {
	statusCode := err.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	if statusCode < http.StatusMultipleChoices || statusCode == http.StatusNotModified || statusCode > 999 {
		return 0, fmt.Errorf("wire: invalid API call error HTTP status %d", statusCode)
	}
	return statusCode, nil
}

// DecodeErrorResponse reads a non-2xx JSON HTTP response into an APICallError.
func DecodeErrorResponse(resp *http.Response) (*provider.APICallError, error) {
	if resp == nil {
		return nil, fmt.Errorf("wire: nil HTTP response")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wire: reading error response body: %w", err)
	}
	apiErr, err := DecodeAPICallError(body)
	if err != nil {
		return nil, err
	}
	if apiErr.StatusCode == 0 {
		apiErr.StatusCode = resp.StatusCode
	}
	if apiErr.URL == "" && resp.Request != nil && resp.Request.URL != nil {
		apiErr.URL = resp.Request.URL.String()
	}
	if apiErr.ResponseHeaders == nil && len(resp.Header) > 0 {
		apiErr.ResponseHeaders = cloneHTTPHeader(resp.Header)
	}
	if apiErr.ResponseBody == "" {
		apiErr.ResponseBody = string(body)
	}
	return apiErr, nil
}

func cloneHTTPHeader(h http.Header) map[string][]string {
	clone := maps.Clone(map[string][]string(h))
	for k := range clone {
		clone[k] = slices.Clone(clone[k])
	}
	return clone
}
