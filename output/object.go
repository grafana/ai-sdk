package output

import (
	"encoding/json"
	"fmt"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

// ObjectOutput implements aisdk.Output for generating a single typed object.
type ObjectOutput[T any] struct {
	schema schema.Schema
	name   *string
	desc   *string
}

// Object creates an ObjectOutput that generates a single typed object matching
// the given schema. The schema is used both to guide the LLM (via
// ResponseFormat) and to validate the response.
//
// Returns an error if the schema is uninitialized (zero-value).
func Object[T any](s schema.Schema, opts ...ObjectOption) (*ObjectOutput[T], error) {
	if s.Compiled() == nil {
		return nil, fmt.Errorf("output.Object: schema has no compiled validator; use schema.SchemaFor, schema.SchemaFromJSON, or schema.SchemaFromFile to create one")
	}
	o := &ObjectOutput[T]{
		schema: s,
	}
	for _, opt := range opts {
		opt.applyObject(o)
	}
	return o, nil
}

// ObjectOption configures optional parameters for Object.
type ObjectOption interface {
	applyObject(o any)
}

type objectName string

func (n objectName) applyObject(o any) {
	switch v := o.(type) {
	case interface{ setName(string) }:
		v.setName(string(n))
	}
}

// WithName sets the schema name passed to the provider in ResponseFormat.
func WithName(name string) ObjectOption { return objectName(name) }

type objectDescription string

func (d objectDescription) applyObject(o any) {
	switch v := o.(type) {
	case interface{ setDescription(string) }:
		v.setDescription(string(d))
	}
}

// WithDescription sets the schema description passed to the provider.
func WithDescription(desc string) ObjectOption { return objectDescription(desc) }

func (o *ObjectOutput[T]) setName(name string)        { o.name = &name }
func (o *ObjectOutput[T]) setDescription(desc string) { o.desc = &desc }
func (o *ObjectOutput[T]) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{
		Type:        provider.ResponseFormatJSON,
		Schema:      o.schema.JSON(),
		Name:        o.name,
		Description: o.desc,
	}
}

func (o *ObjectOutput[T]) ParseComplete(text string) (any, error) {
	data := json.RawMessage(text)
	if err := o.schema.Validate(data); err != nil {
		return nil, fmt.Errorf("%w: %v", aisdk.ErrNoObjectGenerated, err)
	}

	var result T
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("%w: unmarshaling: %v", aisdk.ErrNoObjectGenerated, err)
	}
	return result, nil
}

func (o *ObjectOutput[T]) ParsePartial(text string) (any, bool) {
	return parsePartialJSON(text)
}
