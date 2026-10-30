package openaicompatible

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_DisabledStructuredOutputs(t *testing.T) {
	want := []provider.Warning{{
		Type:    provider.WarnUnsupported,
		Feature: "responseFormat",
		Details: "JSON response format schema is only supported with structuredOutputs",
	}}
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: [DONE]\n\n"))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"value\":\"Spark\"}"},"finish_reason":"stop"}]}`))
			}))
			defer server.Close()
			model := New("gpt-4o-2024-08-06", WithBaseURL(server.URL), WithStructuredOutputs(false))
			opts := provider.CallOptions{
				Prompt:         []provider.Message{provider.UserText("Hello")},
				ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false,"$schema":"http://json-schema.org/draft-07/schema#"}`)},
			}
			var warnings []provider.Warning
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				parts := collectStreamParts(result)
				require.NotEmpty(t, parts)
				require.Equal(t, provider.PartStreamStart, parts[0].Type)
				warnings = parts[0].Warnings
			} else {
				result, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
				warnings = result.Warnings
			}
			assert.Equal(t, map[string]any{"type": "json_object"}, body["response_format"])
			assert.Equal(t, want, warnings)
		})
	}
}

func TestStructuredOutput_NamePresence(t *testing.T) {
	for _, name := range []*string{nil, new(""), new("answer")} {
		for _, streaming := range []bool{false, true} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Format struct {
						Schema struct {
							Name   *string         `json:"name"`
							Schema json.RawMessage `json:"schema"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				expected := "response"
				if name != nil {
					expected = *name
				}
				require.NotNil(t, body.Format.Schema.Name)
				assert.Equal(t, expected, *body.Format.Schema.Name)
				assert.Contains(t, string(body.Format.Schema.Schema), "9007199254740993")
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: [DONE]\n\n"))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`))
				}
			}))
			model := New("model", WithBaseURL(server.URL), WithStructuredOutputs(true))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("json")}, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Name: name, Schema: json.RawMessage(`{"type":"object","x-number":9007199254740993}`)}}
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
			}
			server.Close()
		}
	}
}
