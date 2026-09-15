package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

func (m *model) doRequest(ctx context.Context, opts provider.CallOptions, streaming bool) (*http.Response, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	body, err := encodeRequest(opts)
	if err != nil {
		return nil, nil, err
	}
	req, err := m.provider.request(ctx, http.MethodPost, "/language-model", opts.Headers)
	if err != nil {
		return nil, nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}
	req.Header.Set("ai-language-model-id", m.id)
	req.Header.Set("ai-language-model-specification-version", "4")
	req.Header.Set("ai-language-model-streaming", strconv.FormatBool(streaming))
	resp, err := m.provider.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		retryable := true
		return nil, nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "grafana: model transport failed", Cause: err, IsRetryable: &retryable})
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		return nil, nil, readGatewayError(ctx, resp, m.provider.limits.ErrorBytes)
	}
	return resp, body, nil
}

func (m *model) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	resp, requestBody, err := m.doRequest(ctx, opts, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := readJSON(ctx, resp, m.provider.limits.UnaryBytes)
	if err != nil {
		return nil, protocolError("grafana: invalid unary response", resp.StatusCode, err)
	}
	result, err := decodeGenerate(body)
	if err != nil {
		return nil, protocolError("grafana: invalid unary result", resp.StatusCode, err)
	}
	result.Request = &provider.RequestMetadata{Body: requestBody}
	result.Response = &provider.GenerateResponse{Headers: responseHeaders(resp.Header), Body: body}
	result.Warnings = []provider.Warning{}
	return result, nil
}

func (m *model) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	resp, requestBody, err := m.doRequest(ctx, opts, true)
	if err != nil {
		return nil, err
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "text/event-stream" {
		_ = resp.Body.Close()
		return nil, protocolError("grafana: expected SSE media type", resp.StatusCode, nil)
	}
	parts := make(chan provider.StreamPart, 64)
	go consumeStream(ctx, resp.Body, parts, m.provider.limits, opts.IncludeRawChunks)
	return &provider.StreamResult{Stream: parts, Request: &provider.RequestMetadata{Body: requestBody}, Response: &provider.ResponseHeaders{Headers: responseHeaders(resp.Header)}}, nil
}

func responseHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for name, values := range headers {
		if len(values) > 0 {
			result[name] = strings.Join(values, ", ")
		}
	}
	return result
}

type wireFinish struct {
	Unified provider.UnifiedFinishReason `json:"unified"`
	Raw     *string                      `json:"raw"`
}

func (w *wireFinish) UnmarshalJSON(data []byte) error {
	type plain wireFinish
	return decodeFields(data, (*plain)(w), "unified", "raw")
}

type wireUsage struct {
	InputTokens *struct {
		Total      *int `json:"total"`
		NoCache    *int `json:"noCache"`
		CacheRead  *int `json:"cacheRead"`
		CacheWrite *int `json:"cacheWrite"`
	} `json:"inputTokens"`
	OutputTokens *struct {
		Total     *int `json:"total"`
		Text      *int `json:"text"`
		Reasoning *int `json:"reasoning"`
	} `json:"outputTokens"`
}

func (w *wireUsage) UnmarshalJSON(data []byte) error {
	type plain wireUsage
	var groups map[string]json.RawMessage
	if json.Unmarshal(data, &groups) != nil || groups == nil {
		return errors.New("grafana: invalid usage object")
	}
	filtered := make(map[string]map[string]json.RawMessage, 2)
	for name, keys := range map[string][]string{"inputTokens": {"total", "noCache", "cacheRead", "cacheWrite"}, "outputTokens": {"total", "text", "reasoning"}} {
		var counts map[string]json.RawMessage
		if json.Unmarshal(groups[name], &counts) != nil || counts == nil {
			return errors.New("grafana: invalid token usage object")
		}
		filtered[name] = make(map[string]json.RawMessage, len(keys))
		for _, key := range keys {
			if raw, ok := counts[key]; ok {
				var count *int
				if json.Unmarshal(raw, &count) != nil || count == nil {
					return errors.New("grafana: invalid token count")
				}
				filtered[name][key] = raw
			}
		}
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, (*plain)(w))
}

type wireText struct {
	Type provider.GenerateContentType `json:"type"`
	Text *string                      `json:"text"`
}

func (w *wireText) UnmarshalJSON(data []byte) error {
	type plain wireText
	return decodeFields(data, (*plain)(w), "type", "text")
}

func decodeFinish(value *wireFinish) (provider.FinishReason, error) {
	if value == nil {
		return provider.FinishReason{}, errors.New("grafana: missing finish reason")
	}
	switch value.Unified {
	case provider.FinishReasonStop, provider.FinishReasonLength, provider.FinishReasonContentFilter, provider.FinishReasonToolCalls, provider.FinishReasonError, provider.FinishReasonOther:
	default:
		return provider.FinishReason{}, errors.New("grafana: invalid finish reason")
	}
	result := provider.FinishReason{Unified: value.Unified}
	if value.Raw != nil {
		result.Raw = *value.Raw
	}
	return result, nil
}

func decodeUsage(value *wireUsage) (provider.Usage, error) {
	if value == nil || value.InputTokens == nil || value.OutputTokens == nil {
		return provider.Usage{}, errors.New("grafana: missing usage")
	}
	i, o := value.InputTokens, value.OutputTokens
	for _, count := range []*int{i.Total, i.NoCache, i.CacheRead, i.CacheWrite, o.Total, o.Text, o.Reasoning} {
		if count != nil && (*count < 0 || int64(*count) > 9007199254740991) {
			return provider.Usage{}, errors.New("grafana: invalid token count")
		}
	}
	return provider.Usage{InputTokens: provider.InputTokenUsage{Total: i.Total, NoCache: i.NoCache, CacheRead: i.CacheRead, CacheWrite: i.CacheWrite}, OutputTokens: provider.OutputTokenUsage{Total: o.Total, Text: o.Text, Reasoning: o.Reasoning}}, nil
}

func decodeGenerate(body []byte) (*provider.GenerateResult, error) {
	var value struct {
		Content      *[]wireText `json:"content"`
		FinishReason *wireFinish `json:"finishReason"`
		Usage        *wireUsage  `json:"usage"`
	}
	if err := decodeFields(body, &value, "content", "finishReason", "usage"); err != nil {
		return nil, errors.New("grafana: malformed unary result")
	}
	if value.Content == nil {
		return nil, errors.New("grafana: missing content")
	}
	finish, err := decodeFinish(value.FinishReason)
	if err != nil {
		return nil, err
	}
	usage, err := decodeUsage(value.Usage)
	if err != nil {
		return nil, err
	}
	content := make([]provider.GenerateContentPart, 0, len(*value.Content))
	for _, part := range *value.Content {
		if part.Type != provider.ContentText || part.Text == nil {
			return nil, errors.New("grafana: unsupported unary content")
		}
		content = append(content, provider.GenerateContentPart{Type: provider.ContentText, Text: *part.Text})
	}
	return &provider.GenerateResult{Content: content, FinishReason: finish, Usage: usage}, nil
}
