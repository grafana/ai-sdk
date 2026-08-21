package output

import (
	"encoding/json"
	"fmt"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

// ArrayOutput implements aisdk.Output for generating an array of typed elements.
// The element schema is wrapped in an outer object {"elements": [...]} for the
// provider, since LLMs cannot reliably produce bare JSON arrays.
type ArrayOutput[T any] struct {
	elementSchema schema.Schema
	wrappedSchema schema.Schema
	name          *string
	desc          *string
}

// Array creates an ArrayOutput that generates an array of typed elements
// matching the given element schema.
func Array[T any](elementSchema schema.Schema, opts ...ObjectOption) (*ArrayOutput[T], error) {
	wrappedRaw, err := buildArrayWrapperSchema(elementSchema.JSON())
	if err != nil {
		return nil, fmt.Errorf("output.Array: building wrapper schema: %w", err)
	}

	wrappedSchema, err := schema.SchemaFromJSON(wrappedRaw)
	if err != nil {
		return nil, fmt.Errorf("output.Array: %w", err)
	}

	o := &ArrayOutput[T]{
		elementSchema: elementSchema,
		wrappedSchema: wrappedSchema,
	}
	for _, opt := range opts {
		opt.applyObject(o)
	}
	return o, nil
}

func buildArrayWrapperSchema(elementSchema json.RawMessage) (json.RawMessage, error) {
	elements := map[string]any{
		"type":  "array",
		"items": json.RawMessage(elementSchema),
	}
	wrapper := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"properties": map[string]any{
			"elements": elements,
		},
		"required":             []string{"elements"},
		"additionalProperties": false,
	}

	var item map[string]json.RawMessage
	if err := json.Unmarshal(elementSchema, &item); err == nil {
		delete(item, "$schema")
		for _, keyword := range []string{"definitions", "$defs"} {
			definition, ok := item[keyword]
			delete(item, keyword)
			if ok && string(definition) != "null" {
				wrapper[keyword] = definition
			}
		}
		data, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		elements["items"] = json.RawMessage(data)
	}

	return json.Marshal(wrapper)
}

func (o *ArrayOutput[T]) setName(name string)        { o.name = &name }
func (o *ArrayOutput[T]) setDescription(desc string) { o.desc = &desc }
func (o *ArrayOutput[T]) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{
		Type:        provider.ResponseFormatJSON,
		Schema:      o.wrappedSchema.JSON(),
		Name:        o.name,
		Description: o.desc,
	}
}

func (o *ArrayOutput[T]) ParseComplete(text string) (any, error) {
	data := json.RawMessage(text)
	if err := o.wrappedSchema.Validate(data); err != nil {
		return nil, fmt.Errorf("%w: %v", aisdk.ErrNoObjectGenerated, err)
	}

	var wrapper struct {
		Elements []T `json:"elements"`
	}
	if err := json.Unmarshal([]byte(text), &wrapper); err != nil {
		return nil, fmt.Errorf("%w: unmarshaling: %v", aisdk.ErrNoObjectGenerated, err)
	}
	return wrapper.Elements, nil
}

func (o *ArrayOutput[T]) ParsePartial(text string) (any, bool) {
	parsed, state := parsePartialJSONState(text)
	if state == partialParseFailed || state == partialParseUndefined {
		return nil, false
	}

	var wrapper struct {
		Elements []json.RawMessage `json:"elements"`
	}
	if err := json.Unmarshal(parsed, &wrapper); err != nil {
		return nil, false
	}
	if wrapper.Elements == nil {
		return nil, false
	}
	elements := wrapper.Elements
	if state == partialParseRepaired && len(elements) > 0 {
		elements = elements[:len(elements)-1]
	}

	var validElements []json.RawMessage
	for _, elem := range elements {
		if err := o.elementSchema.Validate(elem); err == nil {
			validElements = append(validElements, elem)
		}
	}
	return validElements, true
}
