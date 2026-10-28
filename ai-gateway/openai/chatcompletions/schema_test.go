package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatSchemas(t *testing.T) {
	for name, raw := range map[string]string{
		"nullable union":    `{"type":"object","properties":{"value":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["value"],"additionalProperties":false}`,
		"local reference":   `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","$defs":{"value":{"type":["string","null"]}},"properties":{"value":{"$ref":"#/$defs/value"}},"required":["value"],"additionalProperties":false}`,
		"schema properties": `{"type":"object","additionalProperties":{"type":["string","null"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			v, err := compileSchema(json.RawMessage(raw))
			require.NoError(t, err)
			require.NoError(t, v.Validate(json.RawMessage(`{"value":null}`)))
			require.Error(t, v.Validate(json.RawMessage(`{"value":17}`)))
			r := requestWith(t, `,"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":`+raw+`}}`)
			assert.JSONEq(t, raw, string(r.options.ResponseFormat.Schema))
		})
	}
	t.Run("omitted function parameters", func(t *testing.T) {
		r := requestWith(t, `,"tools":[{"type":"function","function":{"name":"ping"}}]`)
		assert.JSONEq(t, `{"type":"object","properties":{},"additionalProperties":false}`, string(r.options.Tools[0].InputSchema))
	})
	for _, raw := range []string{`{"$ref":"file:///etc/passwd"}`, `{"$ref":"https://example.com/schema"}`, `{"type":"object","properties":{"x":{"$ref":"relative.json"}}}`} {
		_, err := compileSchema(json.RawMessage(raw))
		require.Error(t, err)
	}
}

func TestChatSchema_NumberPrecision(t *testing.T) {
	compiled, err := compileSchema(json.RawMessage(`{"const":9007199254740993}`))
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(json.RawMessage(`9007199254740993`)))
	require.Error(t, compiled.Validate(json.RawMessage(`9007199254740992`)))
	_, err = compileSchema(json.RawMessage(`true`))
	require.ErrorIs(t, err, errRequest)
}
