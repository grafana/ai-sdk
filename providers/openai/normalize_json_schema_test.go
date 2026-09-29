package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIJSONSchema_RecursivePropertyNames(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object", "propertyNames":{"type":"string","pattern":"^[A-Z]+$"},
		"properties":{"a":{"propertyNames":{"type":"string"}},"flag":false},
		"patternProperties":{"^x":{"propertyNames":{"type":"string"}}},
		"additionalProperties":{"propertyNames":{"type":"string"}},
		"additionalItems":{"propertyNames":{"type":"string"}},
		"items":[true,{"propertyNames":{"type":"string"}}],
		"contains":{"propertyNames":{"type":"string"}},
		"not":{"propertyNames":{"type":"string"}},
		"allOf":[{"propertyNames":{"type":"string"}}],
		"anyOf":[{"propertyNames":{"type":"string"}}],
		"oneOf":[{"propertyNames":{"type":"string"}}],
		"definitions":{"a":{"propertyNames":{"type":"string"}}},
		"$defs":{"b":{"propertyNames":{"type":"string"}}},
		"dependencies":{"a":["b"],"b":{"propertyNames":{"type":"string"}}},
		"if":{"propertyNames":{"type":"string"}},
		"then":{"propertyNames":{"type":"string"}},
		"else":{"propertyNames":{"type":"string"}},
		"examples":[{"propertyNames":{"type":"number"}}],
		"required":["a"]
	}`)
	original := append(json.RawMessage(nil), schema...)
	got, warnings, err := normalizeOpenAIJSONSchema(schema)
	require.NoError(t, err)
	assert.Equal(t, original, schema)
	assert.Equal(t, []provider.Warning{{
		Type: provider.WarnCompatibility, Feature: "JSON Schema propertyNames",
		Details: "OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints.",
	}}, warnings)
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type":"object", "properties":{"a":{},"flag":false}, "patternProperties":{"^x":{}},
		"additionalProperties":{}, "additionalItems":{}, "items":[true,{}], "contains":{},
		"not":{}, "allOf":[{}], "anyOf":[{}], "oneOf":[{}], "definitions":{"a":{}},
		"$defs":{"b":{}}, "dependencies":{"a":["b"],"b":{}},
		"if":{}, "then":{}, "else":{}, "examples":[{"propertyNames":{"type":"number"}}],
		"required":["a"]
	}`, string(encoded))
}

func TestNormalizeOpenAIJSONSchema_UnaffectedAndNull(t *testing.T) {
	for _, tc := range []struct {
		name, schema, want string
	}{
		{"null", `{"propertyNames":null,"properties":{"inner":{"propertyNames":null}}}`, `{"properties":{"inner":{}}}`},
		{"unaffected", `{"type":"object","additionalProperties":false,"items":true}`, `{"type":"object","additionalProperties":false,"items":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := json.RawMessage(tc.schema)
			got, warnings, err := normalizeOpenAIJSONSchema(original)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.JSONEq(t, tc.schema, string(original))
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestBuildParams_NormalizesResponsesSchemas(t *testing.T) {
	for _, namespace := range []bool{false, true} {
		for _, strict := range []bool{false, true} {
			name := "regular"
			if namespace {
				name = "namespace"
			}
			if strict {
				name += "-strict"
			}
			t.Run(name, func(t *testing.T) {
				input := json.RawMessage(`{"type":"object","propertyNames":{"type":"string"}}`)
				output := json.RawMessage(`{"type":"object","propertyNames":{"type":"string"},"additionalProperties":false}`)
				response := json.RawMessage(`{"type":"object","propertyNames":{"type":"string"}}`)
				toolOptions := OpenAIToolOptions{OutputSchema: output}
				if namespace {
					toolOptions.Namespace = &OpenAIToolNamespaceOptions{Name: "tools", Description: "group"}
				}
				opts := provider.CallOptions{
					Prompt:          []provider.Message{provider.UserText("hi")},
					ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Name: "result", Schema: response},
					ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{StrictJSONSchema: &strict}),
					Tools:           []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: input, ProviderOptions: provider.BuildProviderOptions(toolOptions)}},
				}
				body, warnings := buildBody(t, "gpt-4o", opts)
				require.Len(t, warnings, 3)
				for _, w := range warnings {
					assert.Equal(t, provider.WarnCompatibility, w.Type)
					assert.Equal(t, "JSON Schema propertyNames", w.Feature)
				}
				format := body["text"].(map[string]any)["format"].(map[string]any)
				assert.Equal(t, strict, format["strict"])
				assert.Equal(t, "result", format["name"])
				assert.Equal(t, map[string]any{"type": "object"}, format["schema"])
				fn := toolsArray(t, body)[0]
				if namespace {
					fn = fn["tools"].([]any)[0].(map[string]any)
				}
				assert.Equal(t, map[string]any{"type": "object"}, fn["parameters"])
				assert.Equal(t, map[string]any{"type": "object", "additionalProperties": false}, fn["output_schema"])
				assert.JSONEq(t, `{"type":"object","propertyNames":{"type":"string"}}`, string(input))
				assert.JSONEq(t, `{"type":"object","propertyNames":{"type":"string"},"additionalProperties":false}`, string(output))
				assert.JSONEq(t, `{"type":"object","propertyNames":{"type":"string"}}`, string(response))
			})
		}
	}
}

func TestResponsesSchemaNormalization_GenerateAndStream(t *testing.T) {
	response := json.RawMessage(`{"type":"object","propertyNames":{"type":"string"}}`)
	input := json.RawMessage(`{"type":"object","propertyNames":{"type":"string"}}`)
	output := json.RawMessage(`{"type":"object","propertyNames":{"type":"string"}}`)
	strict := false
	opts := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: response},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{StrictJSONSchema: &strict}),
		Tools:           []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: input, ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{OutputSchema: output})}},
	}
	var bodies []map[string]any
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(data, &body))
		bodies = append(bodies, body)
		contentType, payload := "application/json", `{"id":"resp_1","status":"completed","output":[]}`
		if body["stream"] == true {
			contentType = "text/event-stream"
			payload = "event: response.completed\n" + `data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_1","status":"completed","output":[]}}` + "\n\n"
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
	})}
	model := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
	generated, err := model.DoGenerate(context.Background(), opts)
	require.NoError(t, err)
	streamed, err := model.DoStream(context.Background(), opts)
	require.NoError(t, err)
	var streamWarnings []provider.Warning
	for part := range streamed.Stream {
		if part.Type == provider.PartStreamStart {
			streamWarnings = part.Warnings
		}
	}
	require.Len(t, bodies, 2)
	require.Len(t, generated.Warnings, 3)
	assert.Equal(t, generated.Warnings, streamWarnings)
	assert.Equal(t, bodies[0]["text"], bodies[1]["text"])
	assert.Equal(t, bodies[0]["tools"], bodies[1]["tools"])
	assert.Equal(t, false, bodies[0]["text"].(map[string]any)["format"].(map[string]any)["strict"])
	assert.JSONEq(t, `{"type":"object","propertyNames":{"type":"string"}}`, string(response))
	assert.JSONEq(t, `{"type":"object","propertyNames":{"type":"string"}}`, string(input))
	assert.JSONEq(t, `{"type":"object","propertyNames":{"type":"string"}}`, string(output))
}

func TestResponsesSchemaNormalization_NullOptionalSchemas(t *testing.T) {
	for _, tc := range []struct {
		name, schemaPosition string
		namespace            bool
	}{
		{name: "response", schemaPosition: "response"},
		{name: "regular tool output", schemaPosition: "output"},
		{name: "namespaced tool output", schemaPosition: "output", namespace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies []map[string]any
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				data, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, json.Unmarshal(data, &body))
				bodies = append(bodies, body)
				contentType, payload := "application/json", `{"id":"resp_1","status":"completed","output":[]}`
				if body["stream"] == true {
					contentType = "text/event-stream"
					payload = "event: response.completed\n" + `data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_1","status":"completed","output":[]}}` + "\n\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
			})}
			model := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}
			if tc.schemaPosition == "response" {
				opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`null`)}
			} else {
				toolOptions := OpenAIToolOptions{OutputSchema: json.RawMessage(`null`)}
				if tc.namespace {
					toolOptions.Namespace = &OpenAIToolNamespaceOptions{Name: "tools"}
				}
				opts.Tools = []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`), ProviderOptions: provider.BuildProviderOptions(toolOptions)}}
			}
			generated, err := model.DoGenerate(t.Context(), opts)
			require.NoError(t, err)
			assert.Empty(t, generated.Warnings)
			streamed, err := model.DoStream(t.Context(), opts)
			require.NoError(t, err)
			for part := range streamed.Stream {
				if part.Type == provider.PartStreamStart {
					assert.Empty(t, part.Warnings)
				}
			}
			require.Len(t, bodies, 2)
			for _, body := range bodies {
				if tc.schemaPosition == "response" {
					assert.Equal(t, "json_object", body["text"].(map[string]any)["format"].(map[string]any)["type"])
				} else {
					fn := toolsArray(t, body)[0]
					if tc.namespace {
						fn = fn["tools"].([]any)[0].(map[string]any)
					}
					assert.NotContains(t, fn, "output_schema")
				}
			}
		})
	}
}

func TestResponsesSchemaNormalization_RejectsBeforeHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, position string
		schema         json.RawMessage
		namespace      bool
	}{
		{"response boolean", "response", json.RawMessage(`{"type":"object","propertyNames":false}`), false},
		{"response nested number", "response", json.RawMessage(`{"properties":{"x":{"propertyNames":{"type":"number"}}}}`), false},
		{"tool input boolean", "input", json.RawMessage(`{"propertyNames":true}`), false},
		{"namespaced input nested number", "input", json.RawMessage(`{"additionalProperties":{"propertyNames":{"type":"number"}}}`), true},
		{"tool output boolean", "output", json.RawMessage(`{"propertyNames":false}`), false},
		{"namespaced output nested number", "output", json.RawMessage(`{"$defs":{"a":{"propertyNames":{"type":"number"}}}}`), true},
		{"malformed response", "response", json.RawMessage(`{"type":`), false},
		{"malformed input", "input", json.RawMessage(`{"type":`), false},
		{"malformed output", "output", json.RawMessage(`{"type":`), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("unexpected HTTP request")
			})}
			model := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}
			if tc.position == "response" {
				opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: tc.schema}
			}
			if tc.position != "response" {
				tool := provider.Tool{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}
				toolOptions := OpenAIToolOptions{}
				if tc.position == "input" {
					tool.InputSchema = tc.schema
				} else {
					toolOptions.OutputSchema = tc.schema
				}
				if tc.namespace {
					toolOptions.Namespace = &OpenAIToolNamespaceOptions{Name: "tools"}
				}
				tool.ProviderOptions = provider.BuildProviderOptions(toolOptions)
				opts.Tools = []provider.Tool{tool}
			}
			_, err := model.DoGenerate(context.Background(), opts)
			require.Error(t, err)
			if !strings.HasPrefix(tc.name, "malformed") {
				assert.Contains(t, err.Error(), "propertyNames")
			}
			_, err = model.DoStream(context.Background(), opts)
			require.Error(t, err)
			assert.Zero(t, requests)
		})
	}
}
