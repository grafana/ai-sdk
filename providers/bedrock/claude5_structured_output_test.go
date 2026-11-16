package bedrock

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/provider"
)

const claude5ClosedSchema = `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`

func TestBuildRequest_Claude5NativeStructuredOutputWithUserTools(t *testing.T) {
	for _, modelID := range []string{
		"anthropic.claude-opus-5",
		"anthropic.claude-opus-5-5",
		"anthropic.claude-sonnet-5",
		"anthropic.claude-sonnet-5-5",
		"anthropic.claude-haiku-5-5",
	} {
		t.Run(modelID, func(t *testing.T) {
			req, _, meta := mustBuildRequest(t, modelID, provider.CallOptions{
				Prompt:         []provider.Message{provider.UserText("Hello")},
				ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(claude5ClosedSchema)},
				Tools:          []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(claude5ClosedSchema)}},
			})
			assert.False(t, meta.usesJSONResponseTool)
			assert.False(t, meta.usesJSONInstruction)
			outputConfig, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
			require.True(t, ok)
			format, err := json.Marshal(outputConfig["format"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"json_schema","schema":`+claude5ClosedSchema+`}`, string(format))
			require.NotNil(t, req.ToolConfig)
			require.Len(t, req.ToolConfig.Tools, 1)
			assert.Equal(t, "lookup", req.ToolConfig.Tools[0].ToolSpec.Name)
		})
	}
}

func TestBuildRequest_Claude5StructuredOutputSchemaFallback(t *testing.T) {
	optional := make([]string, 11)
	for i := range optional {
		optional[i] = fmt.Sprintf(`"f%d":{"type":"string"}`, i)
	}
	tests := []struct{ name, schema string }{
		{"open object", `{"type":"object","additionalProperties":true}`},
		{"recursive reference", `{"type":"object","properties":{"child":{"$ref":"#"}},"required":["child"],"additionalProperties":false}`},
		{"too many optional properties", `{"type":"object","properties":{` + strings.Join(optional, ",") + `},"additionalProperties":false}`},
	}
	for _, tc := range tests {
		name, schema := tc.name, tc.schema
		opts := provider.CallOptions{
			Prompt:         []provider.Message{provider.UserText("Hello")},
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(schema)},
		}

		t.Run(name+" keeps the JSON tool on models that accept forced tool use", func(t *testing.T) {
			req, _, meta := mustBuildRequest(t, "anthropic.claude-opus-5", opts)
			assert.True(t, meta.usesJSONResponseTool)
			assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")
			require.NotNil(t, req.ToolConfig)
			assert.Equal(t, &toolChoiceUnion{Any: &struct{}{}}, req.ToolConfig.ToolChoice)
			spec := req.ToolConfig.Tools[0].ToolSpec
			assert.Equal(t, jsonResponseToolName, spec.Name)
			assert.JSONEq(t, schema, string(spec.InputSchema.JSON))
		})

		t.Run(name+" keeps JSON instructions on models that reject forced tool use", func(t *testing.T) {
			req, _, meta := mustBuildRequest(t, "anthropic.claude-sonnet-5-5", opts)
			assert.True(t, meta.usesJSONInstruction)
			assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")
			assert.Nil(t, req.ToolConfig)
			require.NotEmpty(t, req.System)
			assert.Contains(t, req.System[0].Text, schema)
		})
	}
}
