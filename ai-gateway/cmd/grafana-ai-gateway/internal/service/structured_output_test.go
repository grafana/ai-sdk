package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	nativeStructuredOutputOptions = `{"openai":{"strictJsonSchema":false},"anthropic":{"structuredOutputMode":"outputFormat"}}`
	responseFormatFeature         = "responseFormat"
)

var (
	structuredOutputNames      = []string{"", `,"name":""`, `,"name":"answer"`}
	compatibleJSONObjectFormat = map[string]any{"type": "json_object"}
	compatibleSchemaWarning    = provider.Warning{Type: provider.WarnUnsupported, Feature: responseFormatFeature, Details: "JSON response format schema is only supported with structuredOutputs"}
)

func TestStructuredOutput_NativeRequests(t *testing.T) {
	for _, backend := range []string{"openai", "anthropic", "openai-compatible"} {
		for _, streaming := range []bool{false, true} {
			for _, fallback := range []bool{false, true} {
				for _, name := range structuredOutputNames {
					t.Run(fmt.Sprintf("%s/stream=%t/fallback=%t/name=%s", backend, streaming, fallback, name), func(t *testing.T) {
						response, requests := nativeOptionsRequest(t, backend, structuredOutputBody(name, nativeStructuredOutputOptions), streaming, fallback)
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						require.Len(t, requests, structuredOutputRequestCount(fallback))
						for _, request := range requests {
							switch backend {
							case "openai":
								format := request["text"].(map[string]any)["format"].(map[string]any)
								assert.Equal(t, expectedStructuredOutputName(name), format["name"])
								assert.Equal(t, "description", format["description"])
								assert.Equal(t, false, format["strict"])
								assert.Equal(t, "object", format["schema"].(map[string]any)["type"])
							case "anthropic":
								format := request["output_config"].(map[string]any)["format"].(map[string]any)
								assert.Equal(t, "json_schema", format["type"])
								assert.Equal(t, "object", format["schema"].(map[string]any)["type"])
							case "openai-compatible":
								assert.Equal(t, compatibleJSONObjectFormat, request["response_format"])
							}
						}
						if backend == "openai-compatible" {
							assert.Equal(t, []provider.Warning{compatibleSchemaWarning}, responseFormatWarnings(t, response.Body.String(), streaming))
						}
					})
				}
			}
		}
	}
}

func TestStructuredOutput_CompatibleStructuredOutputs(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		for _, streaming := range []bool{false, true} {
			for _, fallback := range []bool{false, true} {
				for _, name := range structuredOutputNames {
					t.Run(fmt.Sprintf("stream=%t/fallback=%t/name=%s", streaming, fallback, name), func(t *testing.T) {
						response, requests := nativeOptionsRequestWithProviders(t, "openai-compatible", structuredOutputBody(name, nativeStructuredOutputOptions), streaming, fallback, withStructuredOutputs(true, true))
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						require.Len(t, requests, structuredOutputRequestCount(fallback))
						for _, request := range requests {
							format := compatibleJSONSchemaFormat(t, request)
							assert.Equal(t, expectedStructuredOutputName(name), format["name"])
							assert.Equal(t, "description", format["description"])
							assert.Equal(t, true, format["strict"])
							assert.Equal(t, "object", format["schema"].(map[string]any)["type"])
						}
						assert.Empty(t, responseFormatWarnings(t, response.Body.String(), streaming))
					})
				}
			}
		}
	})

	t.Run("per-call strict override", func(t *testing.T) {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
				response, requests := nativeOptionsRequestWithProviders(t, "openai-compatible", structuredOutputBody("", `{"myVllm":{"strictJsonSchema":false}}`), streaming, false, withStructuredOutputs(true, true))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Len(t, requests, 1)
				assert.Equal(t, false, compatibleJSONSchemaFormat(t, requests[0])["strict"])
				assert.Empty(t, responseWarnings(t, response.Body.String(), streaming))
			})
		}
	})

	t.Run("mixed fallback", func(t *testing.T) {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name     string
				primary  bool
				fallback bool
			}{
				{name: "primary enabled", primary: true, fallback: false},
				{name: "fallback enabled", primary: false, fallback: true},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
					response, requests := nativeOptionsRequestWithProviders(t, "openai-compatible", structuredOutputBody("", nativeStructuredOutputOptions), streaming, true, withStructuredOutputs(tc.fallback, tc.primary))
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					require.Len(t, requests, 2)
					assertCompatibleResponseFormat(t, tc.primary, requests[0])
					assertCompatibleResponseFormat(t, tc.fallback, requests[1])
					warnings := responseFormatWarnings(t, response.Body.String(), streaming)
					if tc.fallback {
						assert.Empty(t, warnings)
					} else {
						assert.Equal(t, []provider.Warning{compatibleSchemaWarning}, warnings)
					}
				})
			}
		}
	})
}

func TestStructuredOutput_NativeSchemaRejection(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		response, requests := nativeOptionsRequest(t, "openai", `{"prompt":[],"responseFormat":{"type":"json","schema":{"propertyNames":{"type":"number"}}}}`, streaming, false)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Empty(t, requests)
		assert.Contains(t, response.Body.String(), "propertyNames")
		assert.Contains(t, response.Body.String(), `"outcome":"failed"`)
	}
}

func structuredOutputBody(name, providerOptions string) string {
	return `{"prompt":[],"responseFormat":{"type":"json","schema":{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}` + name + `,"description":"description"},"providerOptions":` + providerOptions + `}`
}

func structuredOutputRequestCount(fallback bool) int {
	if fallback {
		return 2
	}
	return 1
}

func expectedStructuredOutputName(name string) string {
	switch name {
	case `,"name":""`:
		return ""
	case `,"name":"answer"`:
		return "answer"
	default:
		return "response"
	}
}

func withStructuredOutputs(native, primary bool) func(map[string]config.ResolvedProvider) {
	return func(providers map[string]config.ResolvedProvider) {
		for name, enabled := range map[string]bool{"native": native, "primary": primary} {
			resolved := providers[name]
			resolved.SupportsStructuredOutputs = enabled
			providers[name] = resolved
		}
	}
}

func compatibleJSONSchemaFormat(t *testing.T, request map[string]any) map[string]any {
	t.Helper()
	format, ok := request["response_format"].(map[string]any)
	require.True(t, ok, "request has no response_format")
	require.Equal(t, "json_schema", format["type"])
	schema, ok := format["json_schema"].(map[string]any)
	require.True(t, ok, "response_format has no json_schema")
	return schema
}

func assertCompatibleResponseFormat(t *testing.T, structuredOutputs bool, request map[string]any) {
	t.Helper()
	if structuredOutputs {
		compatibleJSONSchemaFormat(t, request)
		return
	}
	assert.Equal(t, compatibleJSONObjectFormat, request["response_format"])
}

func responseFormatWarnings(t *testing.T, body string, streaming bool) []provider.Warning {
	t.Helper()
	var warnings []provider.Warning
	for _, warning := range responseWarnings(t, body, streaming) {
		if warning.Type == provider.WarnUnsupported && warning.Feature == responseFormatFeature {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func responseWarnings(t *testing.T, body string, streaming bool) []provider.Warning {
	t.Helper()
	if !streaming {
		var result struct {
			Warnings []provider.Warning `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &result))
		return result.Warnings
	}
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var part struct {
			Type     provider.StreamPartType `json:"type"`
			Warnings []provider.Warning      `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &part))
		if part.Type == provider.PartStreamStart {
			return part.Warnings
		}
	}
	require.Fail(t, "stream has no stream-start part")
	return nil
}
