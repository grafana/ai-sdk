package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hostedToolResponse = `{"id":"resp_test","created_at":1,"model":"gpt-4o","object":"response","status":"completed","output":[]}`

func TestResponses_HostedToolControls(t *testing.T) {
	for _, tc := range []struct {
		name, id, args, wire, errorPath string
	}{
		{name: "default container", id: toolIDCodeInterpreter, args: `{}`, wire: `{"type":"code_interpreter","container":{"type":"auto"}}`},
		{name: "reference", id: toolIDCodeInterpreter, args: `{"container":"cn_example"}`, wire: `{"type":"code_interpreter","container":"cn_example"}`},
		{name: "empty files", id: toolIDCodeInterpreter, args: `{"container":{"fileIds":[]}}`, wire: `{"type":"code_interpreter","container":{"type":"auto","file_ids":[]}}`},
		{name: "memory only", id: toolIDCodeInterpreter, args: `{"container":{"memoryLimit":"1g"}}`, wire: `{"type":"code_interpreter","container":{"type":"auto","memory_limit":"1g"}}`},
		{name: "disabled", id: toolIDCodeInterpreter, args: `{"container":{"fileIds":["file_test"],"memoryLimit":"4g","networkPolicy":{"type":"disabled","allowedDomains":["ignored.test"]}}}`, wire: `{"type":"code_interpreter","container":{"type":"auto","file_ids":["file_test"],"memory_limit":"4g","network_policy":{"type":"disabled"}}}`},
		{name: "allowlist omits optional secrets", id: toolIDCodeInterpreter, args: `{"container":{"memoryLimit":"16g","networkPolicy":{"type":"allowlist","allowedDomains":["api.example.test"]}}}`, wire: `{"type":"code_interpreter","container":{"type":"auto","memory_limit":"16g","network_policy":{"type":"allowlist","allowed_domains":["api.example.test"]}}}`},
		{name: "allowlist preserves empty arrays", id: toolIDCodeInterpreter, args: `{"container":{"memoryLimit":"64g","networkPolicy":{"type":"allowlist","allowedDomains":[],"domainSecrets":[]}}}`, wire: `{"type":"code_interpreter","container":{"type":"auto","memory_limit":"64g","network_policy":{"type":"allowlist","allowed_domains":[],"domain_secrets":[]}}}`},
		{name: "null container", id: toolIDCodeInterpreter, args: `{"container":null}`, errorPath: "container"},
		{name: "malformed container", id: toolIDCodeInterpreter, args: `{"container":true}`, errorPath: "container"},
		{name: "malformed file ids", id: toolIDCodeInterpreter, args: `{"container":{"fileIds":[null]}}`, errorPath: "fileIds"},
		{name: "invalid memory", id: toolIDCodeInterpreter, args: `{"container":{"memoryLimit":"8g"}}`, errorPath: "memoryLimit"},
		{name: "invalid memory type", id: toolIDCodeInterpreter, args: `{"container":{"memoryLimit":4}}`, errorPath: "memoryLimit"},
		{name: "null policy", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":null}}`, errorPath: "networkPolicy"},
		{name: "unknown policy", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":{"type":"open"}}}`, errorPath: "networkPolicy.type"},
		{name: "missing allowlist domains", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":{"type":"allowlist"}}}`, errorPath: "allowedDomains"},
		{name: "invalid allowlist domains", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":{"type":"allowlist","allowedDomains":"api.example.test"}}}`, errorPath: "allowedDomains"},
		{name: "malformed secret", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":{"type":"allowlist","allowedDomains":[],"domainSecrets":[{"domain":"api.example.test","value":"fake-domain-credential"}]}}}`, errorPath: "domainSecrets[0].name"},
		{name: "malformed secret value", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":{"type":"allowlist","allowedDomains":[],"domainSecrets":[{"domain":"api.example.test","name":"KEY","value":{"echo":"fake-domain-credential"}}]}}}`, errorPath: "domainSecrets[0].value"},
		{name: "tunnel only", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","allowedTools":{"readOnly":true,"toolNames":["read"]},"serverDescription":"test tool","requireApproval":"always"}`, wire: `{"type":"mcp","server_label":"test","tunnel_id":"tunnel_test","allowed_tools":{"read_only":true,"tool_names":["read"]},"server_description":"test tool","require_approval":"always"}`},
		{name: "empty tunnel credentials", id: toolIDMCP, args: `{"serverLabel":"","tunnelId":"tunnel_test","authorization":"","headers":{}}`, wire: `{"type":"mcp","server_label":"","tunnel_id":"tunnel_test","authorization":"","headers":{},"require_approval":"never"}`},
		{name: "missing tunnel label", id: toolIDMCP, args: `{"tunnelId":"tunnel_test"}`, errorPath: "serverLabel"},
		{name: "invalid tunnel authorization", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","authorization":3}`, errorPath: "authorization"},
		{name: "null tunnel header value", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","headers":{"X-Custom":null}}`, errorPath: "headers"},
		{name: "URL only", id: toolIDMCP, args: `{"serverLabel":"test","serverUrl":"https://mcp.example.test"}`, wire: `{"type":"mcp","server_label":"test","server_url":"https://mcp.example.test","require_approval":"never"}`},
		{name: "connector only", id: toolIDMCP, args: `{"serverLabel":"test","connectorId":"connector_test"}`, wire: `{"type":"mcp","server_label":"test","connector_id":"connector_test","require_approval":"never"}`},
		{name: "legacy pair", id: toolIDMCP, args: `{"serverLabel":"test","serverUrl":"https://mcp.example.test","connectorId":"connector_test"}`, wire: `{"type":"mcp","server_label":"test","server_url":"https://mcp.example.test","connector_id":"connector_test","require_approval":"never"}`},
		{name: "empty target is a supplied string", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":""}`, wire: `{"type":"mcp","server_label":"test","tunnel_id":"","require_approval":"never"}`},
		{name: "missing target", id: toolIDMCP, args: `{"serverLabel":"test"}`, errorPath: "one of serverUrl, connectorId, or tunnelId"},
		{name: "tunnel plus URL", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","serverUrl":"https://mcp.example.test"}`, errorPath: "tunnelId cannot be combined"},
		{name: "tunnel plus connector", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","connectorId":"connector_test"}`, errorPath: "tunnelId cannot be combined"},
		{name: "tunnel plus both including empty URL", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","serverUrl":"","connectorId":"connector_test"}`, errorPath: "tunnelId cannot be combined"},
		{name: "null target", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":null}`, errorPath: "tunnelId"},
		{name: "non-string target", id: toolIDMCP, args: `{"serverLabel":"test","serverUrl":42}`, errorPath: "serverUrl"},
	} {
		for _, mode := range []string{"generate", "stream"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				var calls int
				model := hostedToolModel(func(req *http.Request) (*http.Response, error) {
					calls++
					if tc.errorPath == "" {
						var body struct {
							Tools   []json.RawMessage `json:"tools"`
							Include []string          `json:"include"`
						}
						require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
						require.Len(t, body.Tools, 1)
						assert.JSONEq(t, tc.wire, string(body.Tools[0]))
						if tc.id == toolIDCodeInterpreter {
							assert.Contains(t, body.Include, "code_interpreter_call.outputs")
						}
					}
					return hostedToolHTTPResponse(req, mode, hostedToolResponse), nil
				})
				wantCaptures := tc.name == "empty tunnel credentials"
				call := provider.CallOptions{
					Tools:            []provider.Tool{hostedTool(t, tc.id, tc.args)},
					IncludeRawChunks: wantCaptures,
				}
				request, response, err := callHostedTool(t, model, call, mode)
				if tc.errorPath != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tc.errorPath)
					assert.False(t, strings.Contains(err.Error(), "fake-domain-credential"), "validation exposed a credential")
					assert.Zero(t, calls)
					return
				}
				require.NoError(t, err)
				if wantCaptures {
					assert.Contains(t, string(request), `"authorization":""`)
					assert.Contains(t, string(response), `"id":"resp_test"`)
				}
				assert.Equal(t, 1, calls)
			})
		}
	}
}

func TestResponses_HostedToolCredentialDiagnostics(t *testing.T) {
	const domainSecret = "fake-domain-credential"
	const auth = "fake-MCP-auth"
	const header = "fake-MCP-header"
	tools := []provider.Tool{
		hostedTool(t, toolIDCodeInterpreter, `{"container":{"networkPolicy":{"type":"allowlist","allowedDomains":["api.example.test"],"domainSecrets":[{"domain":"api.example.test","name":"KEY","value":"`+domainSecret+`"}]}}}`),
		hostedTool(t, toolIDMCP, `{"serverLabel":"docs","tunnelId":"tunnel_fake","authorization":"`+auth+`","headers":{"X-Custom":"`+header+`"}}`),
	}
	check := func(t *testing.T, diagnostic string, containsCredentials bool) {
		t.Helper()
		for i, secret := range []string{domainSecret, auth, header} {
			assert.Equal(t, containsCredentials, strings.Contains(diagnostic, secret), "credential %d visibility", i)
		}
	}
	echo := fmt.Sprintf(`{"message":"rejected %s %s %s","code":"rate_limit_exceeded","type":"rate_limit_error"}`, domainSecret, auth, header)
	const firstDelta = "data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"delta\":\"hi\"}\n\n"
	const retryDate = "Sun, 06 Nov 1994 08:49:37 GMT"
	contextError := `{"code":"context_length_exceeded","message":"` + domainSecret + `"}`
	failed := func(details string) string {
		return fmt.Sprintf(`data: {"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":%s,"service_tier":"%s","usage":{"input_tokens":2,"echo":"%s"},"incomplete_details":{"reason":"%s"}}}`, details, domainSecret, header, auth) + "\n\n"
	}
	for _, tc := range []struct {
		name, mode   string
		status       int
		response     string
		errorStatus  int
		errorRetried bool
		contextLimit bool
	}{
		{name: "unary success", mode: "generate"},
		{name: "stream success", mode: "stream"},
		{name: "HTTP failure", mode: "generate", status: 429, response: `{"error":` + echo + `}`, errorStatus: 429, errorRetried: true},
		{name: "stream HTTP failure", mode: "stream", status: 429, response: `{"error":` + echo + `}`, errorStatus: 429, errorRetried: true},
		{name: "body failure", mode: "generate", status: 200, response: `{"error":` + echo + `}`, errorStatus: 400},
		{name: "context limit", mode: "generate", status: 400, response: `{"error":` + contextError + `}`, errorStatus: 400, contextLimit: true},
		{name: "SSE preflight failure", mode: "stream", response: `data: {"type":"error","error":` + echo + "}\n\n", errorStatus: 429, errorRetried: true},
		{name: "SSE late error", mode: "stream", response: firstDelta + `data: {"type":"error","error":` + echo + "}\n\n", errorStatus: 429, errorRetried: true},
		{name: "SSE late failed finish", mode: "stream", response: firstDelta + failed(echo), errorStatus: 429, errorRetried: true},
		{name: "SSE preflight failed context limit", mode: "stream", response: failed(contextError), errorStatus: 400, contextLimit: true},
		{name: "SSE late failed context limit", mode: "stream", response: firstDelta + failed(contextError), errorStatus: 400, contextLimit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := hostedToolModel(func(req *http.Request) (*http.Response, error) {
				var body struct {
					Tools json.RawMessage `json:"tools"`
				}
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				check(t, string(body.Tools), true)
				if tc.errorStatus == 0 {
					echoedTools := strings.TrimSuffix(hostedToolResponse, "}") + fmt.Sprintf(`,"tools":[{"type":"mcp","authorization":"%s"}],"service_tier":"%s","usage":{"input_tokens":1,"echo":"%s"},"incomplete_details":{"reason":"%s"}}`, auth, domainSecret, header, auth)
					resp := hostedToolHTTPResponse(req, tc.mode, echoedTools)
					resp.Header.Set("Retry-After", "Sun Nov  6 08:49:37 1994")
					resp.Header.Set("X-Echo", auth)
					return resp, nil
				}
				status, contentType := tc.status, "application/json"
				if status == 0 {
					status, contentType = http.StatusOK, "text/event-stream"
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}, "Retry-After": {"2"}, "Retry-After-Ms": {header}, "X-Echo": {auth}}, Body: io.NopCloser(strings.NewReader(tc.response)), Request: req}, nil
			})
			call := provider.CallOptions{Tools: tools, IncludeRawChunks: true}
			var err error
			if tc.mode == "generate" {
				var result *provider.GenerateResult
				result, err = model.DoGenerate(t.Context(), call)
				if tc.errorStatus == 0 {
					require.NoError(t, err)
					assert.Empty(t, result.Request.Body)
					assert.Empty(t, result.Response.Body)
					assert.Equal(t, map[string]string{"Retry-After": retryDate}, result.Response.Headers)
					assert.Empty(t, result.ProviderMetadata)
					assert.Empty(t, result.FinishReason.Raw)
					assert.Empty(t, result.Usage.Raw)
					encoded, marshalErr := json.Marshal(result)
					require.NoError(t, marshalErr)
					check(t, string(encoded), false)
				}
			} else {
				var result *provider.StreamResult
				result, err = model.DoStream(t.Context(), call)
				if result != nil {
					assert.Empty(t, result.Request.Body)
					assert.Len(t, result.Response.Headers, 1)
					if tc.errorStatus == 0 {
						assert.Equal(t, map[string]string{"Retry-After": retryDate}, result.Response.Headers)
						result.Response.Headers["Retry-After"] = "5"
					}
					var finish, responseMeta bool
					for part := range result.Stream {
						assert.NotEqual(t, provider.PartRaw, part.Type)
						encoded, marshalErr := json.Marshal(part)
						require.NoError(t, marshalErr)
						check(t, string(encoded), false)
						if part.Type == provider.PartResponseMeta {
							responseMeta = true
							assert.Equal(t, map[string]string{"Retry-After": retryDate}, part.ResponseHeaders)
						}
						if part.Type == provider.PartFinish {
							finish = true
							assert.Empty(t, part.ProviderMetadata)
							assert.Empty(t, part.Usage.Raw)
							assert.Empty(t, part.FinishReason.Raw)
						}
						if part.Type == provider.PartError {
							err = part.APICallError
						}
					}
					assert.True(t, finish)
					assert.Equal(t, tc.errorStatus == 0, responseMeta)
				}
			}
			if tc.errorStatus == 0 {
				require.NoError(t, err)
			} else {
				var apiErr *provider.APICallError
				require.ErrorAs(t, err, &apiErr)
				encoded, marshalErr := json.Marshal(apiErr)
				require.NoError(t, marshalErr)
				check(t, string(encoded), false)
				check(t, apiErr.Error(), false)
				assert.Equal(t, tc.errorStatus, apiErr.StatusCode)
				assert.Equal(t, tc.errorRetried, apiErr.IsRetryable)
				assert.Nil(t, apiErr.Unwrap(), "unsafe underlying error must not be retained")
				if tc.contextLimit {
					assert.JSONEq(t, `{"code":"context_length_exceeded"}`, string(apiErr.Data))
				}
				assert.Empty(t, apiErr.RequestBodyValues)
				assert.Empty(t, apiErr.ResponseBody)
				assert.Equal(t, []string{"2"}, apiErr.ResponseHeaders["Retry-After"])
				assert.Len(t, apiErr.ResponseHeaders, 1)
			}
		})
	}
}

func TestResponses_HostedToolCredentialsFromRequestOptions(t *testing.T) {
	const fake = "fake-option-credential"
	for _, tc := range []struct {
		name, headers string
		protected     bool
	}{
		{name: "with credential", headers: `{"X-Custom":"` + fake + `"}`, protected: true},
		{name: "empty headers", headers: `{}`},
	} {
		for _, mode := range []string{"generate", "stream"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				var tools []any
				require.NoError(t, json.Unmarshal([]byte(`[{"type":"mcp","server_label":"test","tunnel_id":"tunnel_test","headers":`+tc.headers+`}]`), &tools))
				model := NewResponses("fake-api-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithJSONSet("tools", tools), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					assert.True(t, strings.Contains(string(body), tc.headers), "tool headers did not reach the transport")
					return hostedToolHTTPResponse(req, mode, hostedToolResponse), nil
				})})))
				request, rawResponse, err := callHostedTool(t, model, provider.CallOptions{IncludeRawChunks: true}, mode)
				require.NoError(t, err)
				if tc.protected {
					assert.Empty(t, request)
					assert.Empty(t, rawResponse)
				} else {
					assert.Contains(t, string(request), `"headers":{}`)
					assert.Contains(t, string(rawResponse), `"id":"resp_test"`)
				}
			})
		}
	}
}

func TestSafeHostedToolError_PreservesPanicCause(t *testing.T) {
	diagnostics := &hostedToolDiagnostics{}
	assert.ErrorIs(t, diagnostics.safeError(streamrecover.ErrPanic), streamrecover.ErrPanic)
	wrapped := diagnostics.safeAPIError(streamrecover.Error())
	assert.ErrorIs(t, wrapped, streamrecover.ErrPanic)
	assert.False(t, wrapped.IsRetryable)
}

func TestHostedToolCredentialDetection_IsScoped(t *testing.T) {
	for _, tc := range []struct {
		name, id, args string
		want           bool
	}{
		{name: "domain secret", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":{"type":"allowlist","allowedDomains":[],"domainSecrets":[{"domain":"api.example.test","name":"KEY","value":"fake-domain"}]}}}`, want: true},
		{name: "empty secrets", id: toolIDCodeInterpreter, args: `{"container":{"networkPolicy":{"type":"allowlist","allowedDomains":[],"domainSecrets":[]}}}`},
		{name: "tunnel authorization", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","authorization":"fake-auth"}`, want: true},
		{name: "tunnel headers", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test","headers":{"X-Custom":"fake-header"}}`, want: true},
		{name: "tunnel without credentials", id: toolIDMCP, args: `{"serverLabel":"test","tunnelId":"tunnel_test"}`},
		{name: "legacy MCP credentials", id: toolIDMCP, args: `{"connectorId":"connector_test","authorization":"fake-legacy"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _, _, err := buildParams("gpt-4o", provider.CallOptions{Tools: []provider.Tool{hostedTool(t, tc.id, tc.args)}})
			require.NoError(t, err)
			assert.Equal(t, tc.want, newHostedToolDiagnostics(body).protected())
		})
	}
}

func hostedTool(t *testing.T, id, args string) provider.Tool {
	t.Helper()
	tool := provider.Tool{Type: provider.ToolTypeProvider, ID: id, Name: strings.TrimPrefix(id, "openai.")}
	require.NoError(t, json.Unmarshal([]byte(args), &tool.Args))
	return tool
}

func hostedToolModel(transport roundTripFunc) provider.LanguageModel {
	return NewResponses("fake-api-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transport})))
}

func hostedToolHTTPResponse(req *http.Request, mode, body string) *http.Response {
	contentType := "application/json"
	if mode == "stream" {
		contentType = "text/event-stream"
		body = `data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_test","status":"in_progress"}}` + "\n\n" +
			`data: {"type":"response.completed","sequence_number":1,"response":` + body + "}\n\n"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func callHostedTool(t *testing.T, model provider.LanguageModel, call provider.CallOptions, mode string) (request, response json.RawMessage, err error) {
	t.Helper()
	if mode == "generate" {
		result, err := model.DoGenerate(t.Context(), call)
		if err != nil {
			return nil, nil, err
		}
		assert.Empty(t, result.Warnings)
		return result.Request.Body, result.Response.Body, nil
	}
	result, err := model.DoStream(t.Context(), call)
	if err != nil {
		return nil, nil, err
	}
	for part := range result.Stream {
		assert.Empty(t, part.Warnings)
		if part.Type == provider.PartError {
			err = part.APICallError
		}
		if part.Type == provider.PartRaw {
			response = part.RawValue
		}
	}
	return result.Request.Body, response, err
}
