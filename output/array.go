package output

import (
	"encoding/json"
	"errors"
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
	desc          string
	minItems      *int
	maxItems      *int
}

// Array creates an ArrayOutput that generates an array of typed elements
// matching the given element schema. It returns an error when a bound set with
// WithMinItems or WithMaxItems is negative or the minimum exceeds the maximum.
func Array[T any](elementSchema schema.Schema, opts ...ObjectOption) (*ArrayOutput[T], error) {
	o := &ArrayOutput[T]{elementSchema: elementSchema}
	for _, opt := range opts {
		opt.applyObject(o)
	}
	if o.minItems != nil && *o.minItems < 0 {
		return nil, errors.New("output.Array: minItems must be greater than or equal to 0")
	}
	if o.maxItems != nil && *o.maxItems < 0 {
		return nil, errors.New("output.Array: maxItems must be greater than or equal to 0")
	}
	if o.minItems != nil && o.maxItems != nil && *o.minItems > *o.maxItems {
		return nil, errors.New("output.Array: minItems must be less than or equal to maxItems")
	}

	wrappedRaw, err := buildArrayWrapperSchema(elementSchema.JSON(), o.minItems, o.maxItems)
	if err != nil {
		return nil, fmt.Errorf("output.Array: building wrapper schema: %w", err)
	}

	o.wrappedSchema, err = schema.SchemaFromJSON(wrappedRaw)
	if err != nil {
		return nil, fmt.Errorf("output.Array: %w", err)
	}
	return o, nil
}

func buildArrayWrapperSchema(elementSchema json.RawMessage, minItems, maxItems *int) (json.RawMessage, error) {
	elements := map[string]any{
		"type":  "array",
		"items": json.RawMessage(elementSchema),
	}
	if minItems != nil {
		elements["minItems"] = *minItems
	}
	if maxItems != nil {
		elements["maxItems"] = *maxItems
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
func (o *ArrayOutput[T]) setDescription(desc string) { o.desc = desc }
func (o *ArrayOutput[T]) setMinItems(n int)          { o.minItems = &n }
func (o *ArrayOutput[T]) setMaxItems(n int)          { o.maxItems = &n }

// MaxItems returns the maximum set with WithMaxItems, and false when the array
// has no maximum. It implements [aisdk.BoundedArrayOutput].
func (o *ArrayOutput[T]) MaxItems() (max int, ok bool) {
	if o.maxItems == nil {
		return 0, false
	}
	return *o.maxItems, true
}
func (o *ArrayOutput[T]) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{
		Type:        provider.ResponseFormatJSON,
		Schema:      o.wrappedSchema.JSON(),
		Name:        o.name,
		Description: o.desc,
	}
}

// ParseComplete extracts the elements from the generated wrapper object. It
// checks the required field and validates each element against the element
// schema rather than validating the whole request schema, so a wrapper
// property the model added is ignored, which is how the registered upstream
// runtime reads the same response.
func (o *ArrayOutput[T]) ParseComplete(text string) (any, error) {
	wrapper, err := unmarshalWrapperObject(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: unmarshaling: %w", aisdk.ErrNoObjectGenerated, aisdk.ErrInvalidOutputText, err)
	}
	raw, ok := wrapper["elements"]
	var rawElements []json.RawMessage
	if !ok || json.Unmarshal(raw, &rawElements) != nil || rawElements == nil {
		return nil, fmt.Errorf("%w: %w: response must be an object with an elements array: %w", aisdk.ErrNoObjectGenerated, aisdk.ErrInvalidOutputText, o.wrappedSchema.Validate(json.RawMessage(text)))
	}
	if o.minItems != nil && len(rawElements) < *o.minItems {
		return nil, fmt.Errorf("%w: %w: elements array must contain at least %d items", aisdk.ErrNoObjectGenerated, aisdk.ErrInvalidOutputText, *o.minItems)
	}
	if o.maxItems != nil && len(rawElements) > *o.maxItems {
		return nil, fmt.Errorf("%w: %w: elements array must contain at most %d items", aisdk.ErrNoObjectGenerated, aisdk.ErrInvalidOutputText, *o.maxItems)
	}

	elements := make([]T, 0, len(rawElements))
	for _, rawElement := range rawElements {
		if err := o.elementSchema.Validate(rawElement); err != nil {
			return nil, fmt.Errorf("%w: %w: %w", aisdk.ErrNoObjectGenerated, aisdk.ErrInvalidOutputText, err)
		}
		var element T
		if err := json.Unmarshal(rawElement, &element); err != nil {
			return nil, fmt.Errorf("%w: unmarshaling: %w", aisdk.ErrNoObjectGenerated, err)
		}
		elements = append(elements, element)
	}
	return elements, nil
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
