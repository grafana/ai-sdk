package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixJSON_Empty(t *testing.T) {
	assert.Equal(t, "", fixJSON(""))
}

func TestFixJSON_Literals(t *testing.T) {
	assert.Equal(t, "null", fixJSON("nul"))
	assert.Equal(t, "true", fixJSON("t"))
	assert.Equal(t, "false", fixJSON("fals"))
}

func TestFixJSON_Numbers(t *testing.T) {
	t.Run("incomplete decimal", func(t *testing.T) {
		assert.Equal(t, "12", fixJSON("12."))
	})
	t.Run("number with dot", func(t *testing.T) {
		assert.Equal(t, "12.2", fixJSON("12.2"))
	})
	t.Run("negative", func(t *testing.T) {
		assert.Equal(t, "-12", fixJSON("-12"))
	})
	t.Run("incomplete negative", func(t *testing.T) {
		assert.Equal(t, "", fixJSON("-"))
	})
	t.Run("e-notation", func(t *testing.T) {
		assert.Equal(t, "2.5", fixJSON("2.5e"))
		assert.Equal(t, "2.5", fixJSON("2.5e-"))
		assert.Equal(t, "2.5e3", fixJSON("2.5e3"))
		assert.Equal(t, "-2.5e3", fixJSON("-2.5e3"))
	})
	t.Run("uppercase E-notation", func(t *testing.T) {
		assert.Equal(t, "2.5", fixJSON("2.5E"))
		assert.Equal(t, "2.5", fixJSON("2.5E-"))
		assert.Equal(t, "2.5E3", fixJSON("2.5E3"))
		assert.Equal(t, "-2.5E3", fixJSON("-2.5E3"))
	})
	t.Run("incomplete with e", func(t *testing.T) {
		assert.Equal(t, "12", fixJSON("12.e"))
		assert.Equal(t, "12.34", fixJSON("12.34e"))
		assert.Equal(t, "5", fixJSON("5e"))
	})
}

func TestFixJSON_Strings(t *testing.T) {
	t.Run("incomplete", func(t *testing.T) {
		assert.Equal(t, `"abc"`, fixJSON(`"abc`))
	})
	t.Run("escape sequences", func(t *testing.T) {
		assert.Equal(t,
			`"value with \"quoted\" text and \\ escape"`,
			fixJSON(`"value with \"quoted\" text and \\ escape`))
	})
	t.Run("incomplete escape", func(t *testing.T) {
		assert.Equal(t, `"value with "`, fixJSON(`"value with \`))
	})
	t.Run("unicode characters", func(t *testing.T) {
		assert.Equal(t,
			"\"value with unicode \u003C\"",
			fixJSON("\"value with unicode \u003C\""))
	})
	t.Run("partial unicode escape", func(t *testing.T) {
		assert.Equal(t, `"value with unicode "`, fixJSON(`"value with unicode \u00`))
		assert.Equal(t, `"value with unicode \u003C"`, fixJSON(`"value with unicode \u003C`))
	})
}

func TestFixJSON_Arrays(t *testing.T) {
	t.Run("incomplete array", func(t *testing.T) {
		assert.Equal(t, "[]", fixJSON("["))
	})
	t.Run("closing after number", func(t *testing.T) {
		assert.Equal(t, "[[1], [2]]", fixJSON("[[1], [2"))
	})
	t.Run("closing after string", func(t *testing.T) {
		assert.Equal(t, `[["1"], ["2"]]`, fixJSON(`[["1"], ["2`))
	})
	t.Run("closing after literal", func(t *testing.T) {
		assert.Equal(t, "[[false], [null]]", fixJSON("[[false], [nu"))
	})
	t.Run("closing after array", func(t *testing.T) {
		assert.Equal(t, "[[[]], [[]]]", fixJSON("[[[]], [[]"))
	})
	t.Run("closing after object", func(t *testing.T) {
		assert.Equal(t, "[[{}], [{}]]", fixJSON("[[{}], [{"))
	})
	t.Run("trailing comma", func(t *testing.T) {
		assert.Equal(t, "[1]", fixJSON("[1, "))
	})
	t.Run("closing array", func(t *testing.T) {
		assert.Equal(t, "[[], 123]", fixJSON("[[], 123"))
	})
}

func TestFixJSON_Objects(t *testing.T) {
	t.Run("keys without values", func(t *testing.T) {
		assert.Equal(t, "{}", fixJSON(`{"key":`))
	})
	t.Run("closing after number", func(t *testing.T) {
		assert.Equal(t,
			`{"a": {"b": 1}, "c": {"d": 2}}`,
			fixJSON(`{"a": {"b": 1}, "c": {"d": 2`))
	})
	t.Run("closing after string", func(t *testing.T) {
		assert.Equal(t,
			`{"a": {"b": "1"}, "c": {"d": 2}}`,
			fixJSON(`{"a": {"b": "1"}, "c": {"d": 2`))
	})
	t.Run("closing after literal", func(t *testing.T) {
		assert.Equal(t,
			`{"a": {"b": false}, "c": {"d": 2}}`,
			fixJSON(`{"a": {"b": false}, "c": {"d": 2`))
	})
	t.Run("closing after array", func(t *testing.T) {
		assert.Equal(t,
			`{"a": {"b": []}, "c": {"d": 2}}`,
			fixJSON(`{"a": {"b": []}, "c": {"d": 2`))
	})
	t.Run("closing after object", func(t *testing.T) {
		assert.Equal(t,
			`{"a": {"b": {}}, "c": {"d": 2}}`,
			fixJSON(`{"a": {"b": {}}, "c": {"d": 2`))
	})
	t.Run("partial keys first", func(t *testing.T) {
		assert.Equal(t, "{}", fixJSON(`{"ke`))
	})
	t.Run("partial keys second", func(t *testing.T) {
		assert.Equal(t, `{"k1": 1}`, fixJSON(`{"k1": 1, "k2`))
	})
	t.Run("partial keys with colon", func(t *testing.T) {
		assert.Equal(t, `{"k1": 1}`, fixJSON(`{"k1": 1, "k2":`))
	})
	t.Run("trailing whitespace", func(t *testing.T) {
		assert.Equal(t, `{"key": "value"}`, fixJSON(`{"key": "value"  `))
	})
	t.Run("closing after empty object", func(t *testing.T) {
		assert.Equal(t, `{"a": {"b": {}}}`, fixJSON(`{"a": {"b": {}`))
	})
}

func TestFixJSON_Nesting(t *testing.T) {
	t.Run("nested arrays with numbers", func(t *testing.T) {
		assert.Equal(t, "[1, [2, 3, []]]", fixJSON("[1, [2, 3, ["))
	})
	t.Run("nested arrays with literals", func(t *testing.T) {
		assert.Equal(t, "[false, [true, []]]", fixJSON("[false, [true, ["))
	})
	t.Run("nested objects", func(t *testing.T) {
		assert.Equal(t, `{"key": {}}`, fixJSON(`{"key": {"subKey":`))
	})
	t.Run("nested objects with numbers", func(t *testing.T) {
		assert.Equal(t,
			`{"key": 123, "key2": {}}`,
			fixJSON(`{"key": 123, "key2": {"subKey":`))
	})
	t.Run("nested objects with literals", func(t *testing.T) {
		assert.Equal(t,
			`{"key": null, "key2": {}}`,
			fixJSON(`{"key": null, "key2": {"subKey":`))
	})
	t.Run("arrays within objects", func(t *testing.T) {
		assert.Equal(t, `{"key": [1, 2, {}]}`, fixJSON(`{"key": [1, 2, {`))
	})
	t.Run("objects within arrays", func(t *testing.T) {
		assert.Equal(t,
			`[1, 2, {"key": "value"}]`,
			fixJSON(`[1, 2, {"key": "value",`))
	})
	t.Run("nested arrays and objects", func(t *testing.T) {
		assert.Equal(t,
			`{"a": {"b": ["c", {"d": "e"}]}}`,
			fixJSON(`{"a": {"b": ["c", {"d": "e",`))
	})
	t.Run("deeply nested objects", func(t *testing.T) {
		assert.Equal(t,
			`{"a": {"b": {"c": {}}}}`,
			fixJSON(`{"a": {"b": {"c": {"d":`))
	})
	t.Run("potential nested arrays or objects", func(t *testing.T) {
		assert.Equal(t, `{"a": 1, "b": []}`, fixJSON(`{"a": 1, "b": [`))
		assert.Equal(t, `{"a": 1, "b": {}}`, fixJSON(`{"a": 1, "b": {`))
		assert.Equal(t, `{"a": 1, "b": ""}`, fixJSON(`{"a": 1, "b": "`))
	})
}

func TestFixJSON_Regression(t *testing.T) {
	t.Run("complex nesting", func(t *testing.T) {
		input := "{\n  \"a\": [\n    {\n      \"a1\": \"v1\",\n      \"a2\": \"v2\",\n      \"a3\": \"v3\"\n    }\n  ],\n  \"b\": [\n    {\n      \"b1\": \"n"
		expected := "{\n  \"a\": [\n    {\n      \"a1\": \"v1\",\n      \"a2\": \"v2\",\n      \"a3\": \"v3\"\n    }\n  ],\n  \"b\": [\n    {\n      \"b1\": \"n\"}]}"
		assert.Equal(t, expected, fixJSON(input))
	})
	t.Run("empty objects inside nested objects and arrays", func(t *testing.T) {
		assert.Equal(t,
			`{"type":"div","children":[{"type":"Card","props":{}}]}`,
			fixJSON(`{"type":"div","children":[{"type":"Card","props":{}`))
	})
}
