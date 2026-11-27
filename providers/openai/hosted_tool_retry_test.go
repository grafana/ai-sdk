package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponses_HostedToolCredentialsPersistAcrossRetry(t *testing.T) {
	const first = `{"tools":[{"type":"mcp","server_label":"test","tunnel_id":"tunnel_test","authorization":"27182"}]}`
	const last = `{"input":[]}`
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			attempt := 0
			var sent []string
			client := openaisdk.NewClient(option.WithAPIKey("fake-key"), option.WithMaxRetries(1),
				option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
					attempt++
					if err := req.Body.Close(); err != nil {
						return nil, err
					}
					payload := last
					if attempt == 1 {
						payload = first
					}
					req.Body = io.NopCloser(strings.NewReader(payload))
					req.ContentLength = int64(len(payload))
					return next(req)
				}), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					data, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					sent = append(sent, string(data))
					if attempt == 1 {
						resp := hostedToolHTTPResponse(req, "generate", `{"error":{"message":"retry"}}`)
						resp.StatusCode = http.StatusServiceUnavailable
						resp.Header.Set("Retry-After", "0")
						return resp, nil
					}
					resp := hostedToolHTTPResponse(req, mode, hostedToolResponse)
					resp.Header.Set("Retry-After", "27182")
					resp.Header.Set("Retry-After-Ms", "250")
					return resp, nil
				})}))
			model := NewResponsesWithClient(client, "gpt-4o")
			call := provider.CallOptions{IncludeRawChunks: true}
			if mode == "generate" {
				result, err := model.DoGenerate(t.Context(), call)
				require.NoError(t, err)
				assert.Empty(t, result.Request.Body)
				assert.Empty(t, result.Response.Body)
				assert.Equal(t, map[string]string{"Retry-After-Ms": "250"}, result.Response.Headers)
			} else {
				result, err := model.DoStream(t.Context(), call)
				require.NoError(t, err)
				assert.Empty(t, result.Request.Body)
				assert.Equal(t, map[string]string{"Retry-After-Ms": "250"}, result.Response.Headers)
				for part := range result.Stream {
					assert.NotEqual(t, provider.PartRaw, part.Type)
					if part.Type == provider.PartResponseMeta {
						assert.Equal(t, result.Response.Headers, part.ResponseHeaders)
					}
				}
			}
			assert.True(t, len(sent) == 2 && sent[0] == first && sent[1] == last, "outbound attempt bodies must be unchanged")
		})
	}
}

func TestHostedToolRetryHintProjection(t *testing.T) {
	diagnostics := &hostedToolDiagnostics{}
	diagnostics.addCredential("27182")
	for _, echoed := range []string{"27182", "2.7182e4"} {
		apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
			StatusCode:      http.StatusTooManyRequests,
			ResponseHeaders: http.Header{"Retry-After": {echoed}, "Retry-After-Ms": {"250"}},
		})
		safe := diagnostics.safeAPIError(apiErr)
		assert.Equal(t, map[string][]string{"Retry-After-Ms": {"250"}}, safe.ResponseHeaders)
		assert.Equal(t, http.StatusTooManyRequests, safe.StatusCode)
		assert.True(t, safe.IsRetryable)
	}
}

func TestResponses_HostedToolTransportErrorTraits(t *testing.T) {
	const fake = "fake-transport-diagnostic"
	for _, tc := range []struct {
		name  string
		cause error
	}{
		{name: "network", cause: errors.New(fake)},
		{name: "timeout", cause: &net.OpError{Op: fake, Err: os.ErrDeadlineExceeded}},
		{name: "canceled", cause: &net.OpError{Op: fake, Err: context.Canceled}},
	} {
		for _, mode := range []string{"generate", "stream"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				model := hostedToolModel(func(*http.Request) (*http.Response, error) { return nil, tc.cause })
				call := provider.CallOptions{Tools: []provider.Tool{hostedTool(t, toolIDMCP, `{"serverLabel":"test","tunnelId":"tunnel_test","authorization":"fake-auth"}`)}}
				_, _, err := callHostedTool(t, model, call, mode)
				require.Error(t, err)
				assert.False(t, strings.Contains(err.Error(), fake), "unsafe transport diagnostic")
				assert.False(t, errors.Is(err, tc.cause), "unsafe transport cause was retained")
				if tc.name == "canceled" {
					assert.ErrorIs(t, err, context.Canceled)
					return
				}
				var network net.Error
				require.ErrorAs(t, err, &network)
				assert.Equal(t, tc.name == "timeout", network.Timeout())
				if mode == "stream" {
					var apiErr *provider.APICallError
					require.ErrorAs(t, err, &apiErr)
					assert.True(t, apiErr.IsRetryable)
					data, marshalErr := json.Marshal(apiErr)
					require.NoError(t, marshalErr)
					assert.False(t, strings.Contains(string(data), fake), "unsafe serialized transport diagnostic")
				}
			})
		}
	}
}
