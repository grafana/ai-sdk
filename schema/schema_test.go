package schema

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type simpleStruct struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type withEnum struct {
	Status string `json:"status" jsonschema:"enum=active,enum=inactive,enum=pending"`
}

type withDescription struct {
	Name string `json:"name" jsonschema:"title=User Name,description=The full name of the user"`
}

type withNumericConstraints struct {
	Score float64 `json:"score" jsonschema:"minimum=0,maximum=100"`
}

type withStringConstraints struct {
	Code string `json:"code" jsonschema:"minLength=3,maxLength=10,pattern=^[A-Z]+$"`
}

type customDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

func (customDate) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:    "string",
		Pattern: `^\d{4}-\d{2}$`,
	}
}

type withCustomType struct {
	Date customDate `json:"date"`
}

type withStringMap struct {
	Values map[string]string `json:"values"`
}

func TestSchemaFor(t *testing.T) {
	t.Run("SimpleStruct", func(t *testing.T) {
		s, err := SchemaFor[simpleStruct]()
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(s.JSON(), &m))

		assert.Equal(t, "object", m["type"])

		props, ok := m["properties"].(map[string]any)
		require.True(t, ok, "missing properties")

		name, ok := props["name"].(map[string]any)
		require.True(t, ok, "missing name property")
		assert.Equal(t, "string", name["type"])

		age, ok := props["age"].(map[string]any)
		require.True(t, ok, "missing age property")
		assert.Equal(t, "integer", age["type"])
	})

	t.Run("StripsMeta", func(t *testing.T) {
		s, err := SchemaFor[simpleStruct]()
		require.NoError(t, err)

		raw := string(s.JSON())
		assert.NotContains(t, raw, "$schema")
		assert.NotContains(t, raw, "$id")
	})

	t.Run("Enum", func(t *testing.T) {
		s, err := SchemaFor[withEnum]()
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(s.JSON(), &m))

		props := m["properties"].(map[string]any)
		status := props["status"].(map[string]any)
		enumVals, ok := status["enum"].([]any)
		require.True(t, ok, "missing enum on status")

		want := []string{"active", "inactive", "pending"}
		require.Len(t, enumVals, len(want))
		for i, v := range enumVals {
			assert.Equal(t, want[i], v.(string))
		}
	})

	t.Run("Description", func(t *testing.T) {
		s, err := SchemaFor[withDescription]()
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(s.JSON(), &m))

		props := m["properties"].(map[string]any)
		name := props["name"].(map[string]any)
		assert.Equal(t, "User Name", name["title"])
		assert.Equal(t, "The full name of the user", name["description"])
	})

	t.Run("NumericConstraints", func(t *testing.T) {
		s, err := SchemaFor[withNumericConstraints]()
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(s.JSON(), &m))

		props := m["properties"].(map[string]any)
		score := props["score"].(map[string]any)
		assert.Equal(t, float64(0), score["minimum"].(float64))
		assert.Equal(t, float64(100), score["maximum"].(float64))
	})

	t.Run("StringConstraints", func(t *testing.T) {
		s, err := SchemaFor[withStringConstraints]()
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(s.JSON(), &m))

		props := m["properties"].(map[string]any)
		code := props["code"].(map[string]any)
		assert.Equal(t, "^[A-Z]+$", code["pattern"])
		assert.Equal(t, float64(3), code["minLength"].(float64))
		assert.Equal(t, float64(10), code["maxLength"].(float64))
	})

	t.Run("MapValueSchema", func(t *testing.T) {
		s, err := SchemaFor[withStringMap]()
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(s.JSON(), &m))
		values := m["properties"].(map[string]any)["values"].(map[string]any)
		additional := values["additionalProperties"].(map[string]any)
		assert.Equal(t, "string", additional["type"])
		require.NoError(t, s.Validate(json.RawMessage(`{"values":{"valid":"value"}}`)))
		assert.Error(t, s.Validate(json.RawMessage(`{"values":{"invalid":1}}`)))
	})

	t.Run("CustomJSONSchema", func(t *testing.T) {
		s, err := SchemaFor[withCustomType]()
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(s.JSON(), &m))

		props := m["properties"].(map[string]any)
		date := props["date"].(map[string]any)
		assert.Equal(t, "string", date["type"])
		assert.Equal(t, `^\d{4}-\d{2}$`, date["pattern"])
	})

	t.Run("EndToEnd", func(t *testing.T) {
		type recipe struct {
			Name        string   `json:"name"`
			Ingredients []string `json:"ingredients"`
		}

		s, err := SchemaFor[recipe]()
		require.NoError(t, err)

		valid := json.RawMessage(`{"name":"Pasta","ingredients":["flour","water"]}`)
		require.NoError(t, s.Validate(valid))

		invalid := json.RawMessage(`{"name":123}`)
		assert.Error(t, s.Validate(invalid))
	})
}

func TestSchemaFromJSON(t *testing.T) {
	t.Run("ValidSchema", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
		s, err := SchemaFromJSON(raw)
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(raw), s.JSON())

		require.NoError(t, s.Validate(json.RawMessage(`{"name":"Alice"}`)))
		assert.Error(t, s.Validate(json.RawMessage(`{"age":30}`)))
	})

	t.Run("PreservesSchemaValuedAdditionalProperties", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","additionalProperties":{"type":"string"}}`)
		s, err := SchemaFromJSON(raw)
		require.NoError(t, err)
		assert.Equal(t, raw, s.JSON())
		require.NoError(t, s.Validate(json.RawMessage(`{"valid":"value"}`)))
		assert.Error(t, s.Validate(json.RawMessage(`{"invalid":1}`)))
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		_, err := SchemaFromJSON(json.RawMessage(`{not json`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("InvalidSchema", func(t *testing.T) {
		_, err := SchemaFromJSON(json.RawMessage(`{"type":"notavalidtype"}`))
		require.Error(t, err)
	})
}

func TestSchema_ZeroValue(t *testing.T) {
	var s Schema
	assert.Nil(t, s.JSON())
	assert.Error(t, s.Validate(json.RawMessage(`{}`)))
}

func TestSchema_MarshalJSON(t *testing.T) {
	t.Run("WithSchema", func(t *testing.T) {
		s, err := SchemaFromJSON(json.RawMessage(`{"type":"object"}`))
		require.NoError(t, err)

		data, err := json.Marshal(s)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"object"}`, string(data))
	})

	t.Run("ZeroValue", func(t *testing.T) {
		var s Schema
		data, err := json.Marshal(s)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})
}

func TestSchema_UnmarshalJSON(t *testing.T) {
	t.Run("RoundTrip", func(t *testing.T) {
		original, err := SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
		require.NoError(t, err)

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var restored Schema
		require.NoError(t, json.Unmarshal(data, &restored))

		assert.JSONEq(t, string(original.JSON()), string(restored.JSON()))
		require.NoError(t, restored.Validate(json.RawMessage(`{"name":"Alice"}`)))
		assert.Error(t, restored.Validate(json.RawMessage(`{"age":30}`)))
	})

	t.Run("Null", func(t *testing.T) {
		var s Schema
		require.NoError(t, json.Unmarshal([]byte("null"), &s))
		assert.Nil(t, s.JSON())
		assert.Nil(t, s.Compiled())
	})

	t.Run("InvalidSchema", func(t *testing.T) {
		var s Schema
		err := json.Unmarshal([]byte(`{"type":"notavalidtype"}`), &s)
		require.Error(t, err)
	})

	t.Run("InStructField", func(t *testing.T) {
		type wrapper struct {
			S Schema `json:"s"`
		}

		original := wrapper{}
		original.S, _ = SchemaFromJSON(json.RawMessage(`{"type":"object"}`))

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var restored wrapper
		require.NoError(t, json.Unmarshal(data, &restored))
		assert.JSONEq(t, `{"type":"object"}`, string(restored.S.JSON()))
		require.NotNil(t, restored.S.Compiled())
	})
}

func TestCompileSchema(t *testing.T) {
	t.Run("ValidSchema", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
		compiled, err := CompileSchema(raw)
		require.NoError(t, err)
		require.NotNil(t, compiled)
	})

	t.Run("InvalidSchema", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"notavalidtype"}`)
		_, err := CompileSchema(raw)
		require.Error(t, err)
	})
}

func TestValidate(t *testing.T) {
	t.Run("ValidData", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"integer"}},"required":["name"]}`)
		data := json.RawMessage(`{"name":"Alice","age":30}`)
		require.NoError(t, Validate(raw, data))
	})

	t.Run("InvalidData_MissingRequired", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
		data := json.RawMessage(`{"age":30}`)

		err := Validate(raw, data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("InvalidData_WrongType", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","properties":{"age":{"type":"integer"}},"required":["age"]}`)
		data := json.RawMessage(`{"age":"notanumber"}`)
		require.Error(t, Validate(raw, data))
	})

	t.Run("InvalidData_BadEnum", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["active","inactive"]}},"required":["status"]}`)
		data := json.RawMessage(`{"status":"deleted"}`)
		require.Error(t, Validate(raw, data))
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object"}`)
		data := json.RawMessage(`{invalid json`)
		require.Error(t, Validate(raw, data))
	})

	t.Run("ErrorContainsJSONPointer", func(t *testing.T) {
		raw := json.RawMessage(`{
			"type": "object",
			"properties": {
				"user": {
					"type": "object",
					"properties": {
						"age": {"type": "integer"}
					},
					"required": ["age"]
				}
			},
			"required": ["user"]
		}`)
		data := json.RawMessage(`{"user": {"age": "notanumber"}}`)

		err := Validate(raw, data)
		require.Error(t, err)
		errStr := err.Error()
		assert.True(t, strings.Contains(errStr, "/user/age") || strings.Contains(errStr, "age"),
			"error should reference the failing location, got: %v", errStr)
	})
}

func TestSchema_ConcurrentValidation(t *testing.T) {
	s, err := SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			valid := json.RawMessage(`{"name":"test"}`)
			if err := s.Validate(valid); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		assert.NoError(t, err, "concurrent validation error")
	}
}

func TestCleanSchema_InlineDefs(t *testing.T) {
	raw := json.RawMessage(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id": "https://example.com/schema",
		"$ref": "#/$defs/MyType",
		"$defs": {
			"MyType": {
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"required": ["name"]
			}
		}
	}`)

	cleaned, err := cleanSchema(raw)
	require.NoError(t, err)

	s := string(cleaned)
	assert.NotContains(t, s, "$schema")
	assert.NotContains(t, s, "$id")
	assert.NotContains(t, s, "$ref")
	assert.NotContains(t, s, "$defs")

	var m map[string]any
	require.NoError(t, json.Unmarshal(cleaned, &m))
	assert.Equal(t, "object", m["type"])

	props, ok := m["properties"].(map[string]any)
	require.True(t, ok, "missing properties after inlining")
	assert.Contains(t, props, "name")
}

func TestSchemaFromFile(t *testing.T) {
	t.Run("ValidFile", func(t *testing.T) {
		dir := t.TempDir()
		path := dir + "/schema.json"
		require.NoError(t, os.WriteFile(path, []byte(`{"type":"object","properties":{"count":{"type":"integer","minimum":0}},"required":["count"]}`), 0644))

		s, err := SchemaFromFile(path)
		require.NoError(t, err)

		require.NoError(t, s.Validate(json.RawMessage(`{"count":5}`)))
		assert.Error(t, s.Validate(json.RawMessage(`{"count":-1}`)))
	})

	t.Run("FileNotFound", func(t *testing.T) {
		_, err := SchemaFromFile("/nonexistent/path/schema.json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading schema file")
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		dir := t.TempDir()
		path := dir + "/bad.json"
		require.NoError(t, os.WriteFile(path, []byte(`{not valid json`), 0644))

		_, err := SchemaFromFile(path)
		require.Error(t, err)
	})
}
