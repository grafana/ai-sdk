package output

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

// ChoiceOutput implements aisdk.Output for selecting from a predefined set
// of string options. The options are wrapped in an outer object
// {"result": "..."} with an enum constraint.
type ChoiceOutput struct {
	options       []string
	wrappedSchema schema.Schema
	name          string
	desc          string
}

// Choice creates a ChoiceOutput that constrains the LLM to select from
// the given string options.
func Choice(options ...string) (*ChoiceOutput, error) {
	return ChoiceWithOptions(options)
}

// ChoiceWithOptions creates a ChoiceOutput with response format metadata.
func ChoiceWithOptions(options []string, opts ...ObjectOption) (*ChoiceOutput, error) {
	if len(options) == 0 {
		return nil, fmt.Errorf("output.Choice: at least one option is required")
	}

	wrappedRaw, err := buildChoiceWrapperSchema(options)
	if err != nil {
		return nil, fmt.Errorf("output.Choice: building wrapper schema: %w", err)
	}

	wrappedSchema, err := schema.SchemaFromJSON(wrappedRaw)
	if err != nil {
		return nil, fmt.Errorf("output.Choice: %w", err)
	}

	o := &ChoiceOutput{
		options:       options,
		wrappedSchema: wrappedSchema,
	}
	for _, opt := range opts {
		opt.applyObject(o)
	}
	return o, nil
}

func buildChoiceWrapperSchema(options []string) (json.RawMessage, error) {
	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = o
	}

	wrapper := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"properties": map[string]any{
			"result": map[string]any{
				"type": "string",
				"enum": enumValues,
			},
		},
		"required":             []string{"result"},
		"additionalProperties": false,
	}
	return json.Marshal(wrapper)
}

func (o *ChoiceOutput) setName(name string)        { o.name = name }
func (o *ChoiceOutput) setDescription(desc string) { o.desc = desc }
func (o *ChoiceOutput) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{
		Type:        provider.ResponseFormatJSON,
		Schema:      o.wrappedSchema.JSON(),
		Name:        o.name,
		Description: o.desc,
	}
}

// ParseComplete extracts the choice from the generated wrapper object. It
// checks the required field and its value rather than the whole request
// schema, so a wrapper property the model added is ignored, which is how the
// registered upstream runtime reads the same response.
func (o *ChoiceOutput) ParseComplete(text string) (any, error) {
	wrapper, err := unmarshalWrapperObject(text)
	if err != nil {
		return nil, fmt.Errorf("%w: unmarshaling: %v", aisdk.ErrNoObjectGenerated, err)
	}
	result, ok := wrapperString(wrapper, "result")
	if !ok || !slices.Contains(o.options, result) {
		return nil, fmt.Errorf("%w: response must be an object that contains a choice value", aisdk.ErrNoObjectGenerated)
	}
	return result, nil
}

func (o *ChoiceOutput) ParsePartial(text string) (any, bool) {
	parsed, state := parsePartialJSONState(text)
	if state == partialParseFailed || state == partialParseUndefined {
		return nil, false
	}

	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(parsed, &wrapper); err != nil {
		return nil, false
	}
	// A missing, null or non-string result is not a choice the model has made
	// yet. Treating it as the empty string would prefix-match every option and
	// publish a choice from a partial object that carries none.
	result, ok := wrapperString(wrapper, "result")
	if !ok {
		return nil, false
	}

	matches := make([]string, 0, len(o.options))
	for _, option := range o.options {
		if strings.HasPrefix(option, result) {
			matches = append(matches, option)
		}
	}
	if state == partialParseSuccessful {
		if slices.Contains(matches, result) {
			return result, true
		}
		return nil, false
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return nil, false
}

// unmarshalWrapperObject decodes the generated text as the JSON object the
// wrapped outputs require. A non-object document fails here.
func unmarshalWrapperObject(text string) (map[string]json.RawMessage, error) {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &wrapper); err != nil {
		return nil, err
	}
	if wrapper == nil {
		return nil, fmt.Errorf("response is null")
	}
	return wrapper, nil
}

// wrapperString reports the named field when it is present and holds a JSON
// string. A null value unmarshals into a string without error, so presence is
// checked through a pointer.
func wrapperString(wrapper map[string]json.RawMessage, field string) (string, bool) {
	raw, ok := wrapper[field]
	if !ok {
		return "", false
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return "", false
	}
	return *value, true
}
