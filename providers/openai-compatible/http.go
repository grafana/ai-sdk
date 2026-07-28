package openaicompatible

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/grafana/ai-sdk/provider"
)

func (m *model) doGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	warnings, bodyBytes, endpoint, err := m.prepareRequest(params, false)
	if err != nil {
		return nil, err
	}
	metadataKey := resolveMetadataKey(params.ProviderOptions, m.providerName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: building request: %w", err)
	}
	m.setHeaders(req, params.Headers, false)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, transportError(endpoint, bodyBytes, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:           fmt.Sprintf("openai: reading response body: %v", err),
			URL:               endpoint,
			RequestBodyValues: json.RawMessage(append([]byte(nil), bodyBytes...)),
			StatusCode:        resp.StatusCode,
			ResponseHeaders:   cloneHeaders(resp.Header),
			Cause:             err,
		})
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, wrapAPIError(resp.StatusCode, endpoint, bodyBytes, resp.Header, respBody)
	}

	result, err := parseGenerateResponse(respBody, resp.Header, m.providerName, metadataKey, m.generateID)
	if err != nil {
		return nil, err
	}
	result.Warnings = warnings
	result.Request = &provider.RequestMetadata{Body: json.RawMessage(bodyBytes)}
	return result, nil
}

func (m *model) doStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	warnings, bodyBytes, endpoint, err := m.prepareRequest(params, true)
	if err != nil {
		return nil, err
	}
	metadataKey := resolveMetadataKey(params.ProviderOptions, m.providerName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: building request: %w", err)
	}
	m.setHeaders(req, params.Headers, true)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, transportError(endpoint, bodyBytes, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, wrapAPIError(resp.StatusCode, endpoint, bodyBytes, resp.Header, respBody)
	}

	ch := make(chan provider.StreamPart, streamBufferSize)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		m.runStream(ctx, endpoint, bodyBytes, resp.Body, resp.Header, warnings, params.IncludeRawChunks, metadataKey, ch)
	}()

	return &provider.StreamResult{
		Stream:   ch,
		Request:  &provider.RequestMetadata{Body: json.RawMessage(bodyBytes)},
		Response: &provider.ResponseHeaders{Headers: flattenHeaders(resp.Header)},
	}, nil
}

func (m *model) prepareRequest(params provider.CallOptions, streaming bool) ([]provider.Warning, []byte, string, error) {
	body, warnings, err := m.buildRequest(params, streaming)
	if err != nil {
		return warnings, nil, "", err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return warnings, nil, "", fmt.Errorf("openai: marshaling request: %w", err)
	}
	endpoint, err := m.endpoint("/chat/completions")
	if err != nil {
		return warnings, nil, "", fmt.Errorf("openai: building endpoint: %w", err)
	}
	return warnings, bodyBytes, endpoint, nil
}

func (m *model) setHeaders(req *http.Request, callHeaders map[string]string, streaming bool) {
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", userAgent)
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}
	for k, v := range m.headers {
		req.Header.Set(k, v)
	}
	for k, v := range callHeaders {
		req.Header.Set(k, v)
	}
}
