package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var requestSchemaSites = []string{"response", "input", "output", "namespace input", "namespace output"}

func schemaCallOptions(schema json.RawMessage, site string) provider.CallOptions {
	opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hi")}}
	if site == "response" {
		opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema}
		return opts
	}
	tool := provider.Tool{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}
	toolOptions := OpenAIToolOptions{}
	if strings.HasSuffix(site, "output") {
		toolOptions.OutputSchema = schema
	} else {
		tool.InputSchema = schema
	}
	if strings.HasPrefix(site, "namespace") {
		toolOptions.Namespace = &OpenAIToolNamespaceOptions{Name: "ns", Description: "tools"}
	}
	tool.ProviderOptions = provider.BuildProviderOptions(toolOptions)
	opts.Tools = []provider.Tool{tool}
	return opts
}

func TestBuildParams_NormalizeSchema(t *testing.T) {
	cases := []struct {
		name, schema, want string
		warn               bool
	}{
		{"string", `{"type":"object","propertyNames":{"type":"string","pattern":"^[A-Z]+$"}}`, `{"type":"object"}`, true},
		{"null", `{"type":"object","propertyNames":null}`, `{"type":"object"}`, false},
		{"nested", `{"type":"object","properties":{"x":{"propertyNames":{"type":"string"},"additionalProperties":{"propertyNames":{"type":"string"}}}},"$defs":{"x":{"if":{"propertyNames":{"type":"string"}}}}}`, `{"type":"object","properties":{"x":{"additionalProperties":{}}},"$defs":{"x":{"if":{}}}}`, true},
		{"literals", `{"type":"object","default":{"propertyNames":false},"examples":[{"propertyNames":{}}],"enum":[{"propertyNames":{"type":"number"}}],"dependencies":{"x":["propertyNames"]}}`, `{"type":"object","default":{"propertyNames":false},"examples":[{"propertyNames":{}}],"enum":[{"propertyNames":{"type":"number"}}],"dependencies":{"x":["propertyNames"]}}`, false},
		{"valid", `{"type":"object","properties":{"x":{"type":"string"},"no":false},"additionalProperties":false}`, `{"type":"object","properties":{"x":{"type":"string"},"no":false},"additionalProperties":false}`, false},
	}
	for _, keyword := range []string{"properties", "patternProperties", "definitions", "$defs", "dependencies"} {
		cases = append(cases, struct {
			name, schema, want string
			warn               bool
		}{keyword, `{"` + keyword + `":{"x":{"propertyNames":{"type":"string"}},"boolean":true}}`, `{"` + keyword + `":{"x":{},"boolean":true}}`, true})
	}
	for _, keyword := range []string{"additionalProperties", "additionalItems", "items", "contains", "not", "if", "then", "else"} {
		cases = append(cases, struct {
			name, schema, want string
			warn               bool
		}{keyword, `{"` + keyword + `":{"propertyNames":{"type":"string"}}}`, `{"` + keyword + `":{}}`, true})
	}
	for _, keyword := range []string{"items", "allOf", "anyOf", "oneOf"} {
		cases = append(cases, struct {
			name, schema, want string
			warn               bool
		}{keyword + " array", `{"` + keyword + `":[true,{"propertyNames":{"type":"string"}}]}`, `{"` + keyword + `":[true,{}]}`, true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, site := range requestSchemaSites {
				t.Run(site, func(t *testing.T) {
					opts := schemaCallOptions(json.RawMessage(tc.schema), site)
					before, err := json.Marshal(opts)
					require.NoError(t, err)
					body, warnings := buildBody(t, "gpt-4o", opts)
					var schema any
					if site == "response" {
						schema = body["text"].(map[string]any)["format"].(map[string]any)["schema"]
					} else {
						tool := body["tools"].([]any)[0].(map[string]any)
						if strings.HasPrefix(site, "namespace") {
							tool = tool["tools"].([]any)[0].(map[string]any)
						}
						if strings.HasSuffix(site, "output") {
							schema = tool["output_schema"]
						} else {
							schema = tool["parameters"]
						}
					}
					raw, err := json.Marshal(schema)
					require.NoError(t, err)
					assert.JSONEq(t, tc.want, string(raw))
					if tc.warn {
						require.Len(t, warnings, 1)
						assert.Equal(t, provider.Warning{Type: provider.WarnCompatibility, Feature: "JSON Schema propertyNames", Details: "OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints."}, warnings[0])
					} else {
						assert.Empty(t, warnings)
					}
					after, err := json.Marshal(opts)
					require.NoError(t, err)
					assert.JSONEq(t, string(before), string(after))
				})
			}
		})
	}
}

func TestBuildParams_NormalizeSchemaWarningsPerBoundary(t *testing.T) {
	schema := json.RawMessage(`{"propertyNames":{"type":"string"},"properties":{"x":{"propertyNames":{"type":"string"}}}}`)
	opts := schemaCallOptions(schema, "response")
	for _, site := range []string{"input", "namespace input"} {
		tool := schemaCallOptions(schema, site).Tools[0]
		toolOptions := OpenAIToolOptions{OutputSchema: schema}
		if site == "namespace input" {
			toolOptions.Namespace = &OpenAIToolNamespaceOptions{Name: "ns", Description: "tools"}
		}
		tool.ProviderOptions = provider.BuildProviderOptions(toolOptions)
		opts.Tools = append(opts.Tools, tool)
	}
	_, warnings := buildBody(t, "gpt-4o", opts)
	require.Len(t, warnings, 5)
	for _, warning := range warnings {
		assert.Equal(t, provider.WarnCompatibility, warning.Type)
		assert.Equal(t, "JSON Schema propertyNames", warning.Feature)
	}
}

func TestModel_NormalizeSchemaRejectsBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	m := NewResponses("test", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
	for _, propertyNames := range []string{`true`, `false`, `{}`, `{"type":"number"}`, `{"type":["string"]}`} {
		for _, site := range requestSchemaSites {
			t.Run(propertyNames+"/"+site, func(t *testing.T) {
				opts := schemaCallOptions(json.RawMessage(`{"properties":{"x":{"propertyNames":`+propertyNames+`}}}`), site)
				_, err := m.DoGenerate(context.Background(), opts)
				require.ErrorContains(t, err, "propertyNames that does not use a string schema")
				_, err = m.DoStream(context.Background(), opts)
				require.ErrorContains(t, err, "propertyNames that does not use a string schema")
			})
		}
	}
	assert.Zero(t, requests.Load())
}
