package v4

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanJSONRejectsInvalidInput(t *testing.T) {
	invalidUTF8 := append([]byte(`{"prompt":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "empty", body: nil},
		{name: "duplicate root", body: []byte(`{"a":1,"a":2}`)},
		{name: "duplicate nested", body: []byte(`{"a":{"b":1,"b":2}}`)},
		{name: "duplicate escaped equivalent", body: []byte(`{"a":1,"\u0061":2}`)},
		{name: "invalid utf8", body: invalidUTF8},
		{name: "invalid escape", body: []byte(`{"a":"\x20"}`)},
		{name: "invalid hex escape", body: []byte(`{"a":"\uZZZZ"}`)},
		{name: "lone high surrogate", body: []byte(`{"a":"\uD800"}`)},
		{name: "high followed by non-low", body: []byte(`{"a":"\uD800\u0041"}`)},
		{name: "lone low surrogate", body: []byte(`{"a":"\uDC00"}`)},
		{name: "raw control", body: []byte("{\"a\":\"\x01\"}")},
		{name: "unterminated string", body: []byte(`{"a":"value}`)},
		{name: "unterminated object", body: []byte(`{"a":1`)},
		{name: "trailing comma object", body: []byte(`{"a":1,}`)},
		{name: "trailing comma array", body: []byte(`[1,]`)},
		{name: "missing colon", body: []byte(`{"a" 1}`)},
		{name: "missing comma", body: []byte(`[1 2]`)},
		{name: "leading zero", body: []byte(`01`)},
		{name: "fraction missing digits", body: []byte(`1.`)},
		{name: "exponent missing digits", body: []byte(`1e+`)},
		{name: "unknown literal", body: []byte(`undefined`)},
		{name: "trailing value", body: []byte(`{} []`)},
		{name: "trailing bytes", body: []byte(`{}x`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, scanJSON(tc.body, 64, 10_000, 128))
		})
	}
}

func TestScanJSONAcceptsValidInput(t *testing.T) {
	tests := []string{
		`null`,
		`true`,
		`-1.25e+3`,
		`"escaped \" \\ \/ \b \f \n \r \t \u0061 \uD83D\uDE00"`,
		`{"":"","opaque":{"array":[null,false,0,"",[],{}]}}`,
		` [ { "nested" : [ 1, 2, 3 ] } ] `,
	}
	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			require.True(t, json.Valid([]byte(body)))
			assert.True(t, scanJSON([]byte(body), 64, 10_000, 128))
		})
	}
}

func TestScanJSONLimits(t *testing.T) {
	t.Run("depth below at above", func(t *testing.T) {
		assert.True(t, scanJSON([]byte(`[[0]]`), 3, 100, 16))
		assert.True(t, scanJSON([]byte(`[[[0]]]`), 3, 100, 16))
		assert.False(t, scanJSON([]byte(`[[[[0]]]]`), 3, 100, 16))
	})

	t.Run("tokens below at above", func(t *testing.T) {
		assert.True(t, scanJSON([]byte(`[0,0,0]`), 8, 5, 16))
		assert.True(t, scanJSON([]byte(`[0,0,0,0]`), 8, 5, 16))
		assert.False(t, scanJSON([]byte(`[0,0,0,0,0]`), 8, 5, 16))
	})

	t.Run("number bytes below at above", func(t *testing.T) {
		assert.True(t, scanJSON([]byte(`1234`), 8, 10, 5))
		assert.True(t, scanJSON([]byte(`12345`), 8, 10, 5))
		assert.False(t, scanJSON([]byte(`123456`), 8, 10, 5))
		assert.False(t, scanJSON([]byte(`1.2345`), 8, 10, 5))
		assert.False(t, scanJSON([]byte(`1e12345`), 8, 10, 5))
	})

	t.Run("large nesting stays iterative", func(t *testing.T) {
		body := strings.Repeat("[", 10_000) + "0" + strings.Repeat("]", 10_000)
		assert.False(t, scanJSON([]byte(body), 9_999, 20_001, 16))
	})
}

func TestScanJSONProviderWireGoldens(t *testing.T) {
	goldens := map[string]int{
		"comprehensive-unions.json": 1,
		"headers.json":              2,
		"scalar-presence.json":      1,
		"sequence.json":             2,
		"streaming.json":            1,
	}

	for name, expectedRecords := range goldens {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("../../../test/providerwire-v4/goldens", name)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var records []struct {
				Body json.RawMessage `json:"body"`
			}
			require.NoError(t, json.Unmarshal(data, &records))
			require.Len(t, records, expectedRecords)
			for _, record := range records {
				require.True(t, json.Valid(record.Body), string(record.Body))
				require.True(t, scanJSON(record.Body, 128, 1_000_000, 128), string(record.Body))
			}
		})
	}
}
