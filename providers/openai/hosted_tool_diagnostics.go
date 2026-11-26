package openai

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

const hostedToolRequestFailed = "openai: request with hosted tool credentials failed"

func hasHostedToolCredentials(body responses.ResponseNewParams) bool {
	for _, tool := range body.Tools {
		if ci := tool.OfCodeInterpreter; ci != nil && ci.Container.OfCodeInterpreterToolAuto != nil {
			if policy := ci.Container.OfCodeInterpreterToolAuto.NetworkPolicy.OfAllowlist; policy != nil && len(policy.DomainSecrets) > 0 {
				return true
			}
		}
		if mcp := tool.OfMcp; mcp != nil && mcp.TunnelID.Valid() && (mcp.Authorization.Valid() || len(mcp.Headers) > 0) {
			return true
		}
	}
	return false
}

func hasWireHostedToolCredentials(raw json.RawMessage) bool {
	var body struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return false
	}
	for _, entry := range body.Tools {
		var tool struct {
			Type          string          `json:"type"`
			Tunnel        json.RawMessage `json:"tunnel_id"`
			Authorization json.RawMessage `json:"authorization"`
			Headers       json.RawMessage `json:"headers"`
			Container     json.RawMessage `json:"container"`
		}
		if json.Unmarshal(entry, &tool) != nil {
			continue
		}
		switch tool.Type {
		case "mcp":
			if tool.Tunnel == nil {
				continue
			}
			if tool.Authorization != nil {
				return true
			}
			if tool.Headers != nil {
				var headers map[string]json.RawMessage
				if json.Unmarshal(tool.Headers, &headers) != nil || len(headers) > 0 {
					return true
				}
			}
		case "code_interpreter":
			var container struct {
				NetworkPolicy struct {
					DomainSecrets []json.RawMessage `json:"domain_secrets"`
				} `json:"network_policy"`
			}
			if json.Unmarshal(tool.Container, &container) == nil && len(container.NetworkPolicy.DomainSecrets) > 0 {
				return true
			}
		}
	}
	return false
}

func safeHostedToolError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		return safeHostedToolAPIError(apiErr)
	}
	if cause := safeHostedToolCause(err); cause != nil {
		return cause
	}
	return errors.New(hostedToolRequestFailed)
}

func safeHostedToolCause(err error) error {
	for _, cause := range []error{streamrecover.ErrPanic, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) {
			return cause
		}
	}
	return nil
}

func safeHostedToolAPIError(err *provider.APICallError) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         hostedToolRequestFailed,
		StatusCode:      err.StatusCode,
		IsRetryable:     &err.IsRetryable,
		ResponseHeaders: safeHostedToolHeaders(err.ResponseHeaders),
		Data:            safeHostedToolErrorCode(err),
		Cause:           safeHostedToolCause(err),
	})
}

func safeHostedToolHeaders(headers http.Header) http.Header {
	var safe http.Header
	for _, key := range []string{"Retry-After-Ms", "Retry-After"} {
		values := headers.Values(key)
		if len(values) == 0 {
			continue
		}
		hint := normalizedRetryHint(key, values[0])
		if hint != "" {
			if safe == nil {
				safe = make(http.Header, 2)
			}
			safe.Set(key, hint)
		}
	}
	return safe
}

func normalizedRetryHint(key, value string) string {
	value = strings.TrimSpace(value)
	if num, err := strconv.ParseFloat(value, 64); err == nil && num >= 0 && num <= float64(math.MaxInt32) {
		return strconv.FormatFloat(num, 'f', -1, 64)
	}
	if key == "Retry-After" {
		if date, err := http.ParseTime(value); err == nil {
			return date.UTC().Format(http.TimeFormat)
		}
	}
	return ""
}

func safeHostedToolErrorCode(err *provider.APICallError) json.RawMessage {
	type errorDetail struct {
		Code string `json:"code"`
	}
	var details struct {
		Type     string       `json:"type"`
		Code     string       `json:"code"`
		Error    *errorDetail `json:"error"`
		Response struct {
			Error errorDetail `json:"error"`
		} `json:"response"`
	}
	data := []byte(err.Data)
	if len(data) == 0 {
		data = []byte(err.ResponseBody)
	}
	_ = json.Unmarshal(data, &details)
	if details.Type == "response.failed" {
		details.Code = details.Response.Error.Code
	} else if details.Error != nil {
		details.Code = details.Error.Code
	}
	if details.Code == "context_length_exceeded" {
		return json.RawMessage(`{"code":"context_length_exceeded"}`)
	}
	return nil
}
