package output

import (
	"encoding/json"
	"errors"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recipe struct {
	Name        string   `json:"name"`
	Ingredients []string `json:"ingredients"`
}

type city struct {
	Name       string `json:"name"`
	Population int    `json:"population"`
}

func mustSchema(t *testing.T, raw string) schema.Schema {
	t.Helper()
	s, err := schema.SchemaFromJSON(json.RawMessage(raw))
	require.NoError(t, err)
	return s
}

func TestObjectOutput_ResponseFormat(t *testing.T) {
	s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := Object[recipe](s, WithName("recipe"), WithDescription("A recipe"))
	require.NoError(t, err)

	rf := out.ResponseFormat()
	assert.Equal(t, provider.ResponseFormatJSON, rf.Type)
	assert.Equal(t, new("recipe"), rf.Name)
	assert.Equal(t, "A recipe", rf.Description)
	assert.NotNil(t, rf.Schema)
}

func TestObjectOutput_ParseComplete_Valid(t *testing.T) {
	s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"ingredients":{"type":"array","items":{"type":"string"}}},"required":["name","ingredients"]}`)
	out, err := Object[recipe](s)
	require.NoError(t, err)

	result, err := out.ParseComplete(`{"name":"Lasagna","ingredients":["pasta","cheese"]}`)
	require.NoError(t, err)

	r, ok := result.(recipe)
	require.True(t, ok, "result type: got %T, want recipe", result)
	assert.Equal(t, "Lasagna", r.Name)
	assert.Len(t, r.Ingredients, 2)
}

func TestObjectOutput_ParseComplete_Invalid(t *testing.T) {
	s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := Object[recipe](s)
	require.NoError(t, err)

	_, err = out.ParseComplete(`{"age": 30}`)
	require.Error(t, err)
	assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
}

func TestObjectOutput_ParseComplete_InvalidJSON(t *testing.T) {
	s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := Object[recipe](s)
	require.NoError(t, err)

	_, err = out.ParseComplete(`not json`)
	require.Error(t, err)
	assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
}

func TestObjectOutput_ParsePartial(t *testing.T) {
	s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := Object[recipe](s)
	require.NoError(t, err)

	v, ok := out.ParsePartial(`{"name":"Las`)
	assert.True(t, ok, "expected partial parse to succeed on truncated JSON via fixJSON")
	assert.NotNil(t, v)

	v, ok = out.ParsePartial(`{"name":"Lasagna"}`)
	require.True(t, ok, "expected partial parse to succeed on valid JSON")
	assert.NotNil(t, v)
}

func TestArrayOutput_ParseComplete_Valid(t *testing.T) {
	elemSchema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]}`)
	out, err := Array[city](elemSchema)
	require.NoError(t, err)

	result, err := out.ParseComplete(`{"elements":[{"name":"Paris","population":2161000},{"name":"London","population":8982000}]}`)
	require.NoError(t, err)

	cities, ok := result.([]city)
	require.True(t, ok, "result type: got %T, want []city", result)
	require.Len(t, cities, 2)
	assert.Equal(t, "Paris", cities[0].Name)
}

func TestArrayOutput_ParseComplete_Invalid(t *testing.T) {
	elemSchema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := Array[city](elemSchema)
	require.NoError(t, err)

	_, err = out.ParseComplete(`{"elements":"not an array"}`)
	require.Error(t, err)
	assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
}

func TestArrayOutput_ResponseFormat(t *testing.T) {
	elemSchema := mustSchema(t, `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := Array[city](elemSchema, WithName("cities"), WithDescription("City list"))
	require.NoError(t, err)

	rf := out.ResponseFormat()
	assert.Equal(t, provider.ResponseFormatJSON, rf.Type)
	assert.Equal(t, new("cities"), rf.Name)
	assert.Equal(t, "City list", rf.Description)

	var s map[string]any
	require.NoError(t, json.Unmarshal(rf.Schema, &s))
	assert.Equal(t, "http://json-schema.org/draft-07/schema#", s["$schema"])

	props := s["properties"].(map[string]any)
	elements := props["elements"].(map[string]any)
	assert.Equal(t, "array", elements["type"])
	items := elements["items"].(map[string]any)
	assert.NotContains(t, items, "$schema")

	rootDefinitions := map[string]string{
		"definitions": `{"type":"object","properties":{"shared":{"$ref":"#/definitions/Shared"}},"required":["shared"],"definitions":{"Shared":{"type":"string"}}}`,
		"$defs":       `{"type":"object","properties":{"shared":{"$ref":"#/$defs/Shared"}},"required":["shared"],"$defs":{"Shared":{"type":"string"}}}`,
	}
	for keyword, raw := range rootDefinitions {
		t.Run(keyword, func(t *testing.T) {
			out, err := Array[map[string]string](mustSchema(t, raw))
			require.NoError(t, err)

			var wrapped map[string]any
			require.NoError(t, json.Unmarshal(out.ResponseFormat().Schema, &wrapped))
			assert.Equal(t, map[string]any{"Shared": map[string]any{"type": "string"}}, wrapped[keyword])
			properties := wrapped["properties"].(map[string]any)
			elements := properties["elements"].(map[string]any)
			items := elements["items"].(map[string]any)
			assert.NotContains(t, items, keyword)
			assert.Equal(t, "#/"+keyword+"/Shared", items["properties"].(map[string]any)["shared"].(map[string]any)["$ref"])
		})
	}
}

func TestArrayOutput_ParsePartial(t *testing.T) {
	elemSchema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]}`)
	out, err := Array[city](elemSchema)
	require.NoError(t, err)

	v, ok := out.ParsePartial(`{"elements":[{"name":"Paris","population":2161000},{"name":"bad"}]}`)
	require.True(t, ok)
	require.Len(t, v.([]json.RawMessage), 1)

	v, ok = out.ParsePartial(`{"elements":[{"name":"Paris","population":2161000},{"name":"Lon`)
	require.True(t, ok)
	require.Len(t, v.([]json.RawMessage), 1, "repaired partial drops the final element")
}

func TestArrayOutput_Bounds(t *testing.T) {
	elemSchema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]}`)

	t.Run("request schema", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			opts []ArrayOption
			want string
		}{
			{name: "unset", want: `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]}}`},
			{name: "zero", opts: []ArrayOption{WithMinItems(0), WithMaxItems(0)}, want: `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]},"minItems":0,"maxItems":0}`},
			{name: "minimum only", opts: []ArrayOption{WithMinItems(1)}, want: `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]},"minItems":1}`},
			{name: "maximum only", opts: []ArrayOption{WithMaxItems(2)}, want: `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]},"maxItems":2}`},
			{name: "paired", opts: []ArrayOption{WithMinItems(1), WithMaxItems(2)}, want: `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]},"minItems":1,"maxItems":2}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, err := Array[city](elemSchema, tc.opts...)
				require.NoError(t, err)
				var wrapped struct {
					Properties struct {
						Elements json.RawMessage `json:"elements"`
					} `json:"properties"`
				}
				require.NoError(t, json.Unmarshal(out.ResponseFormat().Schema, &wrapped))
				assert.JSONEq(t, tc.want, string(wrapped.Properties.Elements))
			})
		}
	})

	t.Run("invalid bounds", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			opts []ArrayOption
			want string
		}{
			{name: "negative minimum", opts: []ArrayOption{WithMinItems(-1)}, want: "output.Array: minItems must be greater than or equal to 0"},
			{name: "negative maximum", opts: []ArrayOption{WithMaxItems(-1)}, want: "output.Array: maxItems must be greater than or equal to 0"},
			{name: "minimum above maximum", opts: []ArrayOption{WithMinItems(3), WithMaxItems(2)}, want: "output.Array: minItems must be less than or equal to maxItems"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, err := Array[city](elemSchema, tc.opts...)
				assert.Nil(t, out)
				assert.EqualError(t, err, tc.want)
			})
		}
	})

	t.Run("complete length", func(t *testing.T) {
		out, err := Array[city](elemSchema, WithMinItems(1), WithMaxItems(2))
		require.NoError(t, err)
		for _, tc := range []struct {
			name    string
			text    string
			wantLen int
			wantErr string
		}{
			{name: "empty", text: `{"elements":[]}`, wantErr: "elements array must contain at least 1 items"},
			{name: "minimum", text: `{"elements":[{"name":"Paris","population":1}]}`, wantLen: 1},
			{name: "maximum", text: `{"elements":[{"name":"Paris","population":1},{"name":"Rome","population":2}]}`, wantLen: 2},
			{name: "above maximum", text: `{"elements":[{"name":"Paris","population":1},{"name":"Rome","population":2},{"name":"Oslo","population":3}]}`, wantErr: "elements array must contain at most 2 items"},
			{name: "length checked before elements", text: `{"elements":[{"name":"Paris"},{"name":"Rome"},{"name":"Oslo"}]}`, wantErr: "elements array must contain at most 2 items"},
			{name: "elements still validated", text: `{"elements":[{"name":"Paris"}]}`, wantErr: "population"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				v, err := out.ParseComplete(tc.text)
				if tc.wantErr != "" {
					require.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
					assert.ErrorIs(t, err, aisdk.ErrInvalidOutputText)
					assert.ErrorContains(t, err, tc.wantErr)
					return
				}
				require.NoError(t, err)
				assert.Len(t, v.([]city), tc.wantLen)
			})
		}
	})

	t.Run("max items", func(t *testing.T) {
		var _ aisdk.BoundedArrayOutput = (*ArrayOutput[city])(nil)

		unbounded, err := Array[city](elemSchema, WithMinItems(1))
		require.NoError(t, err)
		_, ok := unbounded.MaxItems()
		assert.False(t, ok)

		zero, err := Array[city](elemSchema, WithMaxItems(0))
		require.NoError(t, err)
		limit, ok := zero.MaxItems()
		assert.True(t, ok)
		assert.Equal(t, 0, limit)
	})

	t.Run("partial snapshots are not capped", func(t *testing.T) {
		out, err := Array[city](elemSchema, WithMinItems(1), WithMaxItems(1))
		require.NoError(t, err)
		v, ok := out.ParsePartial(`{"elements":[{"name":"Paris","population":1},{"name":"Rome","population":2}]}`)
		require.True(t, ok)
		assert.Len(t, v.([]json.RawMessage), 2)
	})
}

func TestChoiceOutput_ParseComplete_Valid(t *testing.T) {
	out, err := Choice("sunny", "rainy", "snowy")
	require.NoError(t, err)

	result, err := out.ParseComplete(`{"result":"sunny"}`)
	require.NoError(t, err)

	choice, ok := result.(string)
	require.True(t, ok, "result type: got %T, want string", result)
	assert.Equal(t, "sunny", choice)
}

func TestChoiceOutput_ParseComplete_InvalidOption(t *testing.T) {
	out, err := Choice("sunny", "rainy", "snowy")
	require.NoError(t, err)

	_, err = out.ParseComplete(`{"result":"cloudy"}`)
	require.Error(t, err)
	assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
}

func TestChoiceOutput_NoOptions(t *testing.T) {
	_, err := Choice()
	require.Error(t, err)
}

func TestChoiceOutput_ResponseFormat(t *testing.T) {
	out, err := ChoiceWithOptions([]string{"sunny", "rainy"}, WithName("weather"), WithDescription("Weather choice"))
	require.NoError(t, err)

	rf := out.ResponseFormat()
	assert.Equal(t, provider.ResponseFormatJSON, rf.Type)
	assert.Equal(t, new("weather"), rf.Name)
	assert.Equal(t, "Weather choice", rf.Description)

	var s map[string]any
	require.NoError(t, json.Unmarshal(rf.Schema, &s))
	assert.Equal(t, "http://json-schema.org/draft-07/schema#", s["$schema"])
}

func TestChoiceOutput_ParsePartial(t *testing.T) {
	out, err := Choice("sunny", "rainy", "snowy")
	require.NoError(t, err)

	v, ok := out.ParsePartial(`{"result":"sunny"}`)
	require.True(t, ok)
	assert.Equal(t, "sunny", v)

	_, ok = out.ParsePartial(`{"result":"cloudy"}`)
	assert.False(t, ok)

	v, ok = out.ParsePartial(`{"result":"rai`)
	require.True(t, ok)
	assert.Equal(t, "rainy", v)

	_, ok = out.ParsePartial(`{"result":"s`)
	assert.False(t, ok, "ambiguous repaired prefix should not emit")
}

func TestJSONOutput_ParseComplete_Valid(t *testing.T) {
	out := JSON()

	result, err := out.ParseComplete(`{"any":"json","num":42}`)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestJSONOutput_ParseComplete_Invalid(t *testing.T) {
	out := JSON()

	_, err := out.ParseComplete(`not json at all`)
	require.Error(t, err)
	assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
}

func TestJSONOutput_ResponseFormat(t *testing.T) {
	out := JSON(WithName("payload"), WithDescription("Raw JSON payload"))
	rf := out.ResponseFormat()
	assert.Equal(t, provider.ResponseFormatJSON, rf.Type)
	assert.Nil(t, rf.Schema)
	assert.Equal(t, new("payload"), rf.Name)
	assert.Equal(t, "Raw JSON payload", rf.Description)
}

func TestTextOutput_ParseComplete(t *testing.T) {
	out := Text()

	result, err := out.ParseComplete("hello world")
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

func TestTextOutput_ResponseFormat(t *testing.T) {
	out := Text()
	rf := out.ResponseFormat()
	assert.Equal(t, provider.ResponseFormatText, rf.Type)
}

func TestTextOutput_ParsePartial(t *testing.T) {
	out := Text()
	v, ok := out.ParsePartial("partial text")
	assert.True(t, ok, "ParsePartial should always succeed for text")
	assert.Equal(t, "partial text", v)
}

type failingUnmarshaler struct{}

var errTypedConversion = errors.New("typed conversion failed")

func (*failingUnmarshaler) UnmarshalJSON([]byte) error { return errTypedConversion }

func TestOutputRepairErrorClassification(t *testing.T) {
	objectSchema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	obj, err := Object[struct {
		Name int `json:"name"`
	}](objectSchema)
	require.NoError(t, err)
	array, err := Array[struct {
		Name int `json:"name"`
	}](objectSchema)
	require.NoError(t, err)
	choice, err := Choice("sunny")
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		out  aisdk.Output
		text string
	}{
		{"object JSON", obj, `{"name":`},
		{"object schema", obj, `{"wrong":"value"}`},
		{"array JSON", array, `{"elements":`},
		{"array schema", array, `{"elements":[{"wrong":"value"}]}`},
		{"array missing elements", array, `{}`},
		{"array invalid elements", array, `{"elements":null}`},
		{"choice schema", choice, `{"result":"rainy"}`},
		{"choice missing result", choice, `{}`},
		{"JSON syntax", JSON(), `{"name":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.out.ParseComplete(tc.text)
			require.Error(t, err)
			assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
			assert.ErrorIs(t, err, aisdk.ErrInvalidOutputText)
			if json.Valid([]byte(tc.text)) {
				var validationErr *jsonschema.ValidationError
				assert.ErrorAs(t, err, &validationErr)
			} else {
				var syntaxErr *json.SyntaxError
				assert.ErrorAs(t, err, &syntaxErr)
			}
		})
	}

	for _, tc := range []struct {
		name  string
		out   aisdk.Output
		text  string
		cause error
	}{
		{"object type mismatch", obj, `{"name":"valid"}`, nil},
		{"array type mismatch", array, `{"elements":[{"name":"valid"}]}`, nil},
		{"object custom unmarshaler", mustObject[failingUnmarshaler](t, objectSchema), `{"name":"valid"}`, errTypedConversion},
		{"array custom unmarshaler", mustArray[failingUnmarshaler](t, mustSchema(t, `{"type":"object"}`)), `{"elements":[{}]}`, errTypedConversion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.out.ParseComplete(tc.text)
			require.Error(t, err)
			assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
			assert.NotErrorIs(t, err, aisdk.ErrInvalidOutputText)
			if tc.cause != nil {
				assert.ErrorIs(t, err, tc.cause)
			} else {
				var typeErr *json.UnmarshalTypeError
				assert.ErrorAs(t, err, &typeErr)
			}
		})
	}
}

func mustObject[T any](t *testing.T, s schema.Schema) aisdk.Output {
	t.Helper()
	out, err := Object[T](s)
	require.NoError(t, err)
	return out
}

func mustArray[T any](t *testing.T, s schema.Schema) aisdk.Output {
	t.Helper()
	out, err := Array[T](s)
	require.NoError(t, err)
	return out
}

var (
	_ aisdk.Output = (*ObjectOutput[recipe])(nil)
	_ aisdk.Output = (*ArrayOutput[city])(nil)
	_ aisdk.Output = (*ChoiceOutput)(nil)
	_ aisdk.Output = (*JSONOutput)(nil)
	_ aisdk.Output = (*TextOutput)(nil)
)

func TestChoiceOutput_ParsePartial_RequiresStringResult(t *testing.T) {
	// A single option exposes the bug a multi-option set masks: an absent
	// result read as the empty string prefix-matches the only option.
	out, err := Choice("yes")
	require.NoError(t, err)

	for _, text := range []string{`{`, `{"other":`, `{"result":null,`, `{"result":null}`, `{"result":1}`, `{"other":"yes"}`, `{}`} {
		t.Run(text, func(t *testing.T) {
			_, ok := out.ParsePartial(text)
			assert.False(t, ok, "no choice has been generated yet")
		})
	}

	v, ok := out.ParsePartial(`{"result":"ye`)
	require.True(t, ok, "a unique prefix still resolves")
	assert.Equal(t, "yes", v)

	_, ok = out.ParsePartial(`{"result":""}`)
	assert.False(t, ok, "an empty string is not one of the options")
}

func TestChoiceOutput_ParseComplete_WrapperExtraction(t *testing.T) {
	out, err := Choice("yes", "no")
	require.NoError(t, err)

	result, err := out.ParseComplete(`{"result":"yes","extra":true}`)
	require.NoError(t, err, "an unrelated wrapper property is not the caller's schema")
	assert.Equal(t, "yes", result)

	for _, text := range []string{`{}`, `{"result":null}`, `{"result":1}`, `{"result":"maybe"}`, `null`, `"yes"`, `["yes"]`, `not json`} {
		t.Run(text, func(t *testing.T) {
			_, err := out.ParseComplete(text)
			assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
		})
	}
}

func TestArrayOutput_ParseComplete_WrapperExtraction(t *testing.T) {
	elemSchema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"population":{"type":"integer"}},"required":["name","population"]}`)
	out, err := Array[city](elemSchema)
	require.NoError(t, err)

	result, err := out.ParseComplete(`{"elements":[{"name":"Paris","population":2161000}],"extra":true}`)
	require.NoError(t, err, "an unrelated wrapper property is not the caller's schema")
	assert.Equal(t, []city{{Name: "Paris", Population: 2161000}}, result)

	result, err = out.ParseComplete(`{"elements":[]}`)
	require.NoError(t, err, "an empty array is a complete value")
	assert.Empty(t, result)

	for _, text := range []string{`{}`, `{"elements":null}`, `{"elements":{"name":"Paris"}}`, `{"elements":[{"name":"bad"}]}`, `null`, `[]`} {
		t.Run(text, func(t *testing.T) {
			_, err := out.ParseComplete(text)
			assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
		})
	}
}

func TestWrappedOutputs_RequestSchemasStayStrict(t *testing.T) {
	choice, err := Choice("yes", "no")
	require.NoError(t, err)
	array, err := Array[city](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		schema   json.RawMessage
		required string
	}{
		{"choice", choice.ResponseFormat().Schema, "result"},
		{"array", array.ResponseFormat().Schema, "elements"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s map[string]any
			require.NoError(t, json.Unmarshal(tc.schema, &s))
			assert.Equal(t, false, s["additionalProperties"], "runtime extraction is permissive, the request is not")
			assert.Equal(t, []any{tc.required}, s["required"])
		})
	}
}

func TestOutputNamePresence(t *testing.T) {
	s, err := schema.SchemaFor[recipe]()
	require.NoError(t, err)
	for _, name := range []*string{nil, new(""), new("answer")} {
		var opts []ObjectOption
		if name != nil {
			opts = append(opts, WithName(*name))
		}
		object, err := Object[recipe](s, opts...)
		require.NoError(t, err)
		var arrayOpts []ArrayOption
		for _, opt := range opts {
			arrayOpts = append(arrayOpts, opt)
		}
		array, err := Array[recipe](s, arrayOpts...)
		require.NoError(t, err)
		choice, err := ChoiceWithOptions([]string{"yes", "no"}, opts...)
		require.NoError(t, err)
		for _, format := range []*provider.ResponseFormat{object.ResponseFormat(), array.ResponseFormat(), choice.ResponseFormat(), JSON(opts...).ResponseFormat()} {
			assert.Equal(t, name, format.Name)
		}
	}
}
