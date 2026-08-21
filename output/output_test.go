package output

import (
	"encoding/json"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
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
	require.NotNil(t, rf.Name)
	assert.Equal(t, "recipe", *rf.Name)
	require.NotNil(t, rf.Description)
	assert.Equal(t, "A recipe", *rf.Description)
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
	require.NotNil(t, rf.Name)
	assert.Equal(t, "cities", *rf.Name)
	require.NotNil(t, rf.Description)
	assert.Equal(t, "City list", *rf.Description)

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
	require.NotNil(t, rf.Name)
	assert.Equal(t, "weather", *rf.Name)
	require.NotNil(t, rf.Description)
	assert.Equal(t, "Weather choice", *rf.Description)

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
	require.NotNil(t, rf.Name)
	assert.Equal(t, "payload", *rf.Name)
	require.NotNil(t, rf.Description)
	assert.Equal(t, "Raw JSON payload", *rf.Description)
}

func TestOutputResponseFormat_OptionalStringPresence(t *testing.T) {
	absent := JSON().ResponseFormat()
	assert.Nil(t, absent.Name)
	assert.Nil(t, absent.Description)

	explicit := JSON(WithName(""), WithDescription("")).ResponseFormat()
	require.NotNil(t, explicit.Name)
	assert.Empty(t, *explicit.Name)
	require.NotNil(t, explicit.Description)
	assert.Empty(t, *explicit.Description)
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

var (
	_ aisdk.Output = (*ObjectOutput[recipe])(nil)
	_ aisdk.Output = (*ArrayOutput[city])(nil)
	_ aisdk.Output = (*ChoiceOutput)(nil)
	_ aisdk.Output = (*JSONOutput)(nil)
	_ aisdk.Output = (*TextOutput)(nil)
)
