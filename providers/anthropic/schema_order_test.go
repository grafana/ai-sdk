package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/internal/jsonschema"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaOrder_HTTPRequests(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"zebra":{"type":"object","properties":{"reason":{"type":"string","minLength":1},"answer":{"type":"integer","const":9007199254740993}}},"arrange":{"type":"array","items":{"oneOf":[{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}}}]}},"acts":{"$ref":"#/$defs/Zebra"}},"$defs":{"Zebra":{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}}},"Alpha":false},"definitions":{"Zebra":true,"Alpha":{"type":"string"}},"required":["zebra","arrange","acts"]}`)
	original := bytes.Clone(schema)
	for _, mode := range []string{"native", "jsonTool", "function"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", mode, stream), func(t *testing.T) {
				var requestBody []byte
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requestBody, _ = io.ReadAll(r.Body)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"type":"error","error":{"message":"captured","type":"invalid_request_error"}}`))
				}))
				defer server.Close()
				model := New("test", "claude-sonnet-4-5", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
				opts := provider.CallOptions{MaxOutputTokens: new(64)}
				switch mode {
				case "native", "jsonTool":
					opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema}
					outputMode := StructuredOutputFormat
					if mode == "jsonTool" {
						outputMode = StructuredOutputJSONTool
					}
					opts.ProviderOptions = provider.BuildProviderOptions(AnthropicOptions{StructuredOutputMode: outputMode})
				case "function":
					opts.Tools = []provider.Tool{{Type: provider.ToolTypeFunction, Name: "probe", InputSchema: schema}}
				}
				var err error
				if stream {
					_, err = model.DoStream(context.Background(), opts)
				} else {
					_, err = model.DoGenerate(context.Background(), opts)
				}
				var apiError *provider.APICallError
				require.ErrorAs(t, err, &apiError)
				require.Equal(t, http.StatusBadRequest, apiError.StatusCode)
				var body struct {
					OutputConfig struct {
						Format struct {
							Schema json.RawMessage `json:"schema"`
						} `json:"format"`
					} `json:"output_config"`
					Tools []struct {
						InputSchema json.RawMessage `json:"input_schema"`
					} `json:"tools"`
				}
				require.NoError(t, json.Unmarshal(requestBody, &body))
				outbound := body.OutputConfig.Format.Schema
				if mode != "native" {
					require.Len(t, body.Tools, 1)
					outbound = body.Tools[0].InputSchema
				}
				var root map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(outbound, &root))
				assert.Equal(t, []string{"zebra", "arrange", "acts"}, schemaObjectKeys(t, root["properties"]))
				assert.Equal(t, []string{"Zebra", "Alpha"}, schemaObjectKeys(t, root["$defs"]))
				assert.Equal(t, []string{"Zebra", "Alpha"}, schemaObjectKeys(t, root["definitions"]))
				var properties map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(root["properties"], &properties))
				var zebra map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(properties["zebra"], &zebra))
				assert.Equal(t, []string{"reason", "answer"}, schemaObjectKeys(t, zebra["properties"]))
				var arrange struct {
					Items map[string]json.RawMessage `json:"items"`
				}
				require.NoError(t, json.Unmarshal(properties["arrange"], &arrange))
				unionKey := "oneOf"
				if mode == "native" {
					unionKey = "anyOf"
				}
				var branches []map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(arrange.Items[unionKey], &branches))
				require.Len(t, branches, 1)
				assert.Equal(t, []string{"z", "a"}, schemaObjectKeys(t, branches[0]["properties"]))
				var defs map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(root["$defs"], &defs))
				var definition map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(defs["Zebra"], &definition))
				assert.Equal(t, []string{"z", "a"}, schemaObjectKeys(t, definition["properties"]))
				assert.Contains(t, string(outbound), `9007199254740993`)
				if mode == "native" {
					assert.NotContains(t, string(outbound), `"minLength"`)
					assert.Contains(t, string(outbound), `"min length: 1."`)
					assert.Contains(t, string(outbound), `"anyOf"`)
				} else {
					assert.Contains(t, string(outbound), `"minLength":1`)
					assert.Contains(t, string(outbound), `"oneOf"`)
				}
				assert.Equal(t, original, []byte(schema))
			})
		}
	}
}

func schemaObjectKeys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), token)
	var keys []string
	for decoder.More() {
		token, err = decoder.Token()
		require.NoError(t, err)
		keys = append(keys, token.(string))
		var value json.RawMessage
		require.NoError(t, decoder.Decode(&value))
	}
	return keys
}

func TestOrderedSchema_Sanitization(t *testing.T) {
	for _, raw := range []string{
		`{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}}}`,
		`{"oneOf":[{"properties":{"z":true,"a":false}}]}`,
		`{"anyOf":null,"oneOf":[{"properties":{"z":true,"a":false}}]}`,
		`{"oneOf":[{"properties":{"first":true}}],"oneOf":[{"properties":{"z":true,"a":false}}]}`,
		`{"allOf":[{"items":[{"properties":{"z":true,"a":false}}]}]}`,
		`{"$ref":"#/definitions/z","properties":{"unused":false}}`,
		`{"type":"object","properties":{"quote\"":true,"slash\\":false,"日本語":true},"default":{"z":1,"a":2}}`,
		`{"definitions":true,"$defs":[],"anyOf":false,"allOf":true}`,
		`{"properties":{"z":{"const":9007199254740993,"default":1.234567890123456789,"minimum":9007199254740993}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			schema, err := jsonschema.DecodeObject(json.RawMessage(raw))
			require.NoError(t, err)
			sanitized := sanitizeJSONSchema(schema)
			expected, err := json.Marshal(sanitized)
			require.NoError(t, err)
			actual, err := json.Marshal(orderedSchema{value: sanitized, original: json.RawMessage(raw)})
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(actual))
		})
	}
}
