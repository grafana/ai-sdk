package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareTools_StrictSchemaCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema string
		closed bool
	}{
		{"open", `{"type":"object"}`, false},
		{"explicit open", `{"type":"object","additionalProperties":true}`, false},
		{"schema additional properties", `{"type":"object","additionalProperties":{"type":"string"}}`, false},
		{"union open", `{"type":["null","object"]}`, false},
		{"closed", `{"type":"object","additionalProperties":false}`, true},
		{"union closed", `{"type":["null","object"],"additionalProperties":false}`, true},
		{"boolean true", `true`, true},
		{"boolean false", `false`, true},
		{"empty", `{}`, true},
		{"nested open", `{"type":"object","additionalProperties":false,"properties":{"nested":{"type":"object"}}}`, false},
		{"nested closed", `{"type":"object","additionalProperties":false,"properties":{"nested":{"type":"object","additionalProperties":false}}}`, true},
		{"literal objects", `{"type":"object","additionalProperties":false,"default":{"type":"object"},"examples":[{"type":"object"}],"enum":[{"type":"object"}]}`, true},
		{"reference", `{"$ref":"#/$defs/item","$defs":{"item":{"type":"object"}}}`, false},
		{"unresolved external reference", `{"$ref":"https://example.com/schema"}`, true},
		{"property dependency", `{"dependencies":{"item":["other"]}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkStrictSchemaRequest(t, tc.schema, tc.closed)
		})
	}
	for _, keyword := range []string{"properties", "patternProperties", "definitions", "$defs", "dependencies"} {
		t.Run(keyword, func(t *testing.T) {
			checkStrictSchemaRequest(t, `{"`+keyword+`":{"nested":{"type":"object"}}}`, false)
			checkStrictSchemaRequest(t, `{"`+keyword+`":{"nested":{"type":"object","additionalProperties":false}}}`, true)
		})
	}
	for _, keyword := range []string{"propertyNames", "contains", "not", "if", "then", "else", "items"} {
		t.Run(keyword, func(t *testing.T) {
			checkStrictSchemaRequest(t, `{"`+keyword+`":{"type":"object"}}`, false)
			checkStrictSchemaRequest(t, `{"`+keyword+`":{"type":"object","additionalProperties":false}}`, true)
		})
	}
	for _, keyword := range []string{"items", "anyOf", "allOf", "oneOf"} {
		t.Run(keyword+" array", func(t *testing.T) {
			checkStrictSchemaRequest(t, `{"`+keyword+`":[true,{"type":"object"}]}`, false)
			checkStrictSchemaRequest(t, `{"`+keyword+`":[false,{"type":"object","additionalProperties":false}]}`, true)
		})
	}
}

func checkStrictSchemaRequest(t *testing.T, schema string, compatible bool) {
	t.Helper()
	strictTrue, strictFalse := true, false
	for _, strict := range []*bool{nil, &strictFalse, &strictTrue} {
		for _, modelID := range []string{testAnthropicModel, "anthropic.claude-opus-4-8"} {
			tool := provider.Tool{Type: provider.ToolTypeFunction, Name: "lookup", Strict: strict, InputSchema: json.RawMessage(schema)}
			before, err := json.Marshal(tool)
			require.NoError(t, err)
			req, warnings, _ := mustBuildRequest(t, modelID, provider.CallOptions{Tools: []provider.Tool{tool}})
			require.NotNil(t, req.ToolConfig)
			spec := req.ToolConfig.Tools[0].ToolSpec
			require.NotNil(t, spec)
			assert.JSONEq(t, schema, string(spec.InputSchema.JSON))
			if strict == nil {
				assert.Nil(t, spec.Strict)
				assert.Empty(t, warnings)
			} else if modelID != testAnthropicModel || (*strict && !compatible) {
				assert.Nil(t, spec.Strict)
				require.Len(t, warnings, 1)
				assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
				assert.Equal(t, "strict", warnings[0].Feature)
				if modelID != testAnthropicModel {
					assert.Contains(t, warnings[0].Details, "not supported by this model")
				} else {
					assert.Equal(t, "Tool 'lookup' has strict: true, but Amazon Bedrock requires every object in a strict tool schema to set additionalProperties: false. The strict property will be ignored.", warnings[0].Details)
				}
			} else {
				assert.Equal(t, strict, spec.Strict)
				assert.Empty(t, warnings)
			}
			after, err := json.Marshal(tool)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		}
	}
}
