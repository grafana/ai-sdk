package openai

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

const hostedToolRequestFailed = "openai: request with hosted tool credentials failed"

type hostedToolDiagnostics struct {
	credentials    map[string]struct{}
	omitRetryHints bool
}

func (d *hostedToolDiagnostics) protected() bool {
	return len(d.credentials) > 0 || d.omitRetryHints
}

func (d *hostedToolDiagnostics) addCredential(value string) {
	if value == "" {
		return
	}
	if d.credentials == nil {
		d.credentials = make(map[string]struct{})
	}
	d.credentials[value] = struct{}{}
}

func newHostedToolDiagnostics(body responses.ResponseNewParams) *hostedToolDiagnostics {
	d := &hostedToolDiagnostics{}
	for _, tool := range body.Tools {
		if ci := tool.OfCodeInterpreter; ci != nil && ci.Container.OfCodeInterpreterToolAuto != nil {
			if policy := ci.Container.OfCodeInterpreterToolAuto.NetworkPolicy.OfAllowlist; policy != nil {
				for _, secret := range policy.DomainSecrets {
					d.addCredential(secret.Value)
				}
			}
		}
		if mcp := tool.OfMcp; mcp != nil && mcp.TunnelID.Valid() {
			if mcp.Authorization.Valid() {
				d.addCredential(mcp.Authorization.Value)
			}
			for _, value := range mcp.Headers {
				d.addCredential(value)
			}
		}
	}
	return d
}

func (d *hostedToolDiagnostics) observeBody(raw json.RawMessage) {
	var body struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return
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
			d.addWireCredential(tool.Authorization)
			if tool.Headers != nil {
				var headers map[string]json.RawMessage
				if json.Unmarshal(tool.Headers, &headers) != nil {
					d.omitRetryHints = true
				}
				for _, value := range headers {
					d.addWireCredential(value)
				}
			}
		case "code_interpreter":
			var container struct {
				NetworkPolicy struct {
					DomainSecrets []json.RawMessage `json:"domain_secrets"`
				} `json:"network_policy"`
			}
			if json.Unmarshal(tool.Container, &container) == nil {
				for _, entry := range container.NetworkPolicy.DomainSecrets {
					var secret struct {
						Value json.RawMessage `json:"value"`
					}
					if json.Unmarshal(entry, &secret) != nil {
						d.omitRetryHints = true
					}
					d.addWireCredential(secret.Value)
				}
			}
		}
	}
}

func (d *hostedToolDiagnostics) addWireCredential(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		d.omitRetryHints = true
		return
	}
	d.addCredential(value)
}

func (d *hostedToolDiagnostics) safeError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		return d.safeAPIError(apiErr)
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
	var network net.Error
	if errors.As(err, &network) {
		cause := errors.New(hostedToolRequestFailed)
		if network.Timeout() {
			cause = os.ErrDeadlineExceeded
		}
		return &url.Error{Op: "openai", Err: cause}
	}
	return nil
}

func (d *hostedToolDiagnostics) safeAPIError(err *provider.APICallError) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         hostedToolRequestFailed,
		StatusCode:      err.StatusCode,
		IsRetryable:     &err.IsRetryable,
		ResponseHeaders: d.safeHeaders(err.ResponseHeaders),
		Data:            safeHostedToolErrorCode(err),
		Cause:           safeHostedToolCause(err),
	})
}

func (d *hostedToolDiagnostics) safeHeaders(headers http.Header) http.Header {
	if d.omitRetryHints {
		return nil
	}
	var safe http.Header
	for _, key := range []string{"Retry-After-Ms", "Retry-After"} {
		values := headers.Values(key)
		if len(values) == 0 {
			continue
		}
		hint := normalizedRetryHint(key, values[0])
		if hint != "" && !d.containsCredential(values[0]) && !d.containsCredential(hint) {
			if safe == nil {
				safe = make(http.Header, 2)
			}
			safe.Set(key, hint)
		}
	}
	return safe
}

func (d *hostedToolDiagnostics) containsCredential(hint string) bool {
	for value := range d.credentials {
		if strings.Contains(hint, value) {
			return true
		}
	}
	return false
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
