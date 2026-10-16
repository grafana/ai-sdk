package service

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_NativeRequests(t *testing.T) {
	for _, backend := range []string{"openai", "anthropic", "openai-compatible"} {
		for _, streaming := range []bool{false, true} {
			for _, fallback := range []bool{false, true} {
				for _, name := range []string{"", `,"name":""`, `,"name":"answer"`} {
					t.Run(fmt.Sprintf("%s/stream=%t/fallback=%t/name=%s", backend, streaming, fallback, name), func(t *testing.T) {
						body := `{"prompt":[],"responseFormat":{"type":"json","schema":{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}` + name + `,"description":"description"},"providerOptions":{"openai":{"strictJsonSchema":false},"anthropic":{"structuredOutputMode":"outputFormat"}}}`
						response, requests := nativeOptionsRequest(t, backend, body, streaming, fallback)
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						count := 1
						if fallback {
							count = 2
						}
						require.Len(t, requests, count)
						for _, request := range requests {
							switch backend {
							case "openai":
								format := request["text"].(map[string]any)["format"].(map[string]any)
								expectedName := "response"
								if name == `,"name":""` {
									expectedName = ""
								}
								if name == `,"name":"answer"` {
									expectedName = "answer"
								}
								assert.Equal(t, expectedName, format["name"])
								assert.Equal(t, "description", format["description"])
								assert.Equal(t, false, format["strict"])
								assert.Equal(t, "object", format["schema"].(map[string]any)["type"])
							case "anthropic":
								format := request["output_config"].(map[string]any)["format"].(map[string]any)
								assert.Equal(t, "json_schema", format["type"])
								assert.Equal(t, "object", format["schema"].(map[string]any)["type"])
							case "openai-compatible":
								assert.Equal(t, map[string]any{"type": "json_object"}, request["response_format"])
								assert.Contains(t, response.Body.String(), "WithStructuredOutputs")
							}
						}
					})
				}
			}
		}
	}
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
