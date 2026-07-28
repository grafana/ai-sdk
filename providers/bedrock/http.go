package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// encodeModelIDPathSegment percent-encodes a Bedrock model ID for use in the
// request path using encodeURIComponent-compatible escaping.
func encodeModelIDPathSegment(modelID string) string {
	const upperhex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(modelID); i++ {
		c := modelID[i]
		// encodeURIComponent leaves unescaped: A-Z a-z 0-9 - _ . ! ~ * ' ( )
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperhex[c>>4])
		b.WriteByte(upperhex[c&0x0F])
	}
	return b.String()
}

// doGenerate performs a non-streaming Converse call.
//
//  1. Build the request body from CallOptions.
//  2. POST to <baseURL>/model/<modelID>/converse, signed via signRequest.
//  3. Decode the JSON body with parseResponse on success; otherwise wrap
//     the response into a `*provider.APICallError`.
func (m *model) doGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	body, warnings, meta, err := buildRequest(m.modelID, params)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("bedrock: marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/model/%s/converse", m.endpoint(), encodeModelIDPathSegment(m.modelID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("bedrock: building request: %w", err)
	}
	// Match upstream: only Content-Type is set explicitly. Bedrock ignores
	// Accept for converse/converse-stream and the upstream SDK relies on the
	// runtime default, so we omit it for wire and conformance parity.
	req.Header.Set("Content-Type", "application/json")
	for k, v := range params.Headers {
		req.Header.Set(k, v)
	}

	if err := m.signRequest(ctx, req); err != nil {
		var apiErr *provider.APICallError
		if errors.As(err, &apiErr) {
			return nil, apiErr
		}
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		retryable := true
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:     fmt.Sprintf("bedrock: HTTP request failed: %v", err),
			URL:         req.URL.String(),
			IsRetryable: &retryable,
			Cause:       err,
		})
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		retryable := true
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:     fmt.Sprintf("bedrock: reading response body: %v", err),
			URL:         req.URL.String(),
			StatusCode:  resp.StatusCode,
			IsRetryable: &retryable,
			Cause:       err,
		})
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, wrapAPIError(resp.StatusCode, req.URL.String(), string(bodyBytes), resp.Header, respBody)
	}

	result, err := parseResponse(respBody, resp.Header, m.modelID, meta, m.generateID)
	if err != nil {
		return nil, fmt.Errorf("bedrock: parsing response: %w", err)
	}
	result.Warnings = append(result.Warnings, warnings...)
	result.Request = &provider.RequestMetadata{Body: bodyBytes}
	return result, nil
}

// doStream performs a streaming Converse call. The HTTP response carries an
// AWS Smithy event-stream body whose frames we decode into provider stream
// parts.
func (m *model) doStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	body, warnings, meta, err := buildRequest(m.modelID, params)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("bedrock: marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/model/%s/converse-stream", m.endpoint(), encodeModelIDPathSegment(m.modelID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("bedrock: building request: %w", err)
	}
	// Match upstream: only Content-Type is set explicitly. The Bedrock
	// streaming endpoint returns an event-stream body regardless of Accept,
	// and the upstream SDK does not set it, so we omit it for parity.
	req.Header.Set("Content-Type", "application/json")
	for k, v := range params.Headers {
		req.Header.Set(k, v)
	}

	if err := m.signRequest(ctx, req); err != nil {
		var apiErr *provider.APICallError
		if errors.As(err, &apiErr) {
			return nil, apiErr
		}
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		retryable := true
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:     fmt.Sprintf("bedrock: HTTP request failed: %v", err),
			URL:         req.URL.String(),
			IsRetryable: &retryable,
			Cause:       err,
		})
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, wrapAPIError(resp.StatusCode, req.URL.String(), string(bodyBytes), resp.Header, respBody)
	}

	// At this point we own the response body. The runStream goroutine reads
	// it; we close it in the goroutine's defer.
	ch := make(chan provider.StreamPart, streamBufferSize)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		m.runStream(ctx, resp.Body, resp.Header, meta, warnings, params.IncludeRawChunks, ch)
	}()

	return &provider.StreamResult{
		Stream:   ch,
		Request:  &provider.RequestMetadata{Body: bodyBytes},
		Response: &provider.ResponseHeaders{Headers: flattenHeaders(resp.Header)},
	}, nil
}

// flattenHeaders collapses multi-valued headers into a flat string map.
func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = strings.Join(v, ", ")
		}
	}
	return out
}
