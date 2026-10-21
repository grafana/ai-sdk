package jsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeObject_NumberFidelityAndSingleValue(t *testing.T) {
	raw := json.RawMessage(`{"const":9007199254740993,"enum":[-9007199254740993,1.234567890123456789,1e100],"nested":{"minimum":9007199254740993},"false":false,"null":null}`)
	object, err := DecodeObject(raw)
	require.NoError(t, err)
	encoded, err := json.Marshal(object)
	require.NoError(t, err)
	for _, token := range []string{"9007199254740993", "-9007199254740993", "1.234567890123456789", "1e100"} {
		assert.Contains(t, string(encoded), token)
	}
	assert.NotContains(t, string(encoded), `"9007199254740993"`)
	for _, invalid := range []string{`{} {}`, `{} true`, `{} garbage`, `{`, `[]`, `true`, `42`, `null`} {
		_, err := DecodeObject(json.RawMessage(invalid))
		require.Error(t, err, invalid)
	}
}
