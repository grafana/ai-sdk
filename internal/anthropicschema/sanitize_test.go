package anthropicschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitize_RefNodeRetainsBothDefinitionMaps(t *testing.T) {
	input := decodeSchema(t, `{
		"type":"object",
		"properties":{"value":{
			"$ref":"#/properties/value/$defs/Value",
			"definitions":{"Legacy":{"type":"integer","minimum":0}},
			"$defs":{"Value":{"type":"string","minLength":2}},
			"title":"discarded"
		}}
	}`)
	want := decodeSchema(t, `{
		"type":"object",
		"additionalProperties":false,
		"properties":{"value":{
			"$ref":"#/properties/value/$defs/Value",
			"definitions":{"Legacy":{"type":"integer","description":"minimum: 0."}},
			"$defs":{"Value":{"type":"string","description":"min length: 2."}}
		}}
	}`)
	assert.Equal(t, want, Sanitize(input))
}
