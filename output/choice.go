package output

import (
	"encoding/json"
	"fmt"

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
	name          *string
	desc          *string
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

func (o *ChoiceOutput) setName(name string)        { o.name = &name }
func (o *ChoiceOutput) setDescription(desc string) { o.desc = &desc }
func (o *ChoiceOutput) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{
		Type:        provider.ResponseFormatJSON,
		Schema:      o.wrappedSchema.JSON(),
		Name:        o.name,
		Description: o.desc,
	}
}

func (o *ChoiceOutput) ParseComplete(text string) (any, error) {
	data := json.RawMessage(text)
	if err := o.wrappedSchema.Validate(data); err != nil {
		return nil, fmt.Errorf("%w: %v", aisdk.ErrNoObjectGenerated, err)
	}

	var wrapper struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(text), &wrapper); err != nil {
		return nil, fmt.Errorf("%w: unmarshaling: %v", aisdk.ErrNoObjectGenerated, err)
	}
	return wrapper.Result, nil
}

func (o *ChoiceOutput) ParsePartial(text string) (any, bool) {
	parsed, state := parsePartialJSONState(text)
	if state == partialParseFailed || state == partialParseUndefined {
		return nil, false
	}

	var wrapper struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(parsed, &wrapper); err != nil {
		return nil, false
	}

	matches := make([]string, 0, len(o.options))
	for _, option := range o.options {
		if len(wrapper.Result) <= len(option) && option[:len(wrapper.Result)] == wrapper.Result {
			matches = append(matches, option)
		}
	}
	if state == partialParseSuccessful {
		for _, match := range matches {
			if match == wrapper.Result {
				return wrapper.Result, true
			}
		}
		return nil, false
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return nil, false
}
