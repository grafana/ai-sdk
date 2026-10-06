package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
	validateschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Schema bundles a JSON Schema definition with pre-compiled validation.
// It is the Go equivalent of the upstream TS SDK's Schema type.
//
// A zero-value Schema has nil raw bytes and no compiled validator;
// calling Validate on a zero-value Schema returns an error.
type Schema struct {
	raw      json.RawMessage
	compiled *CompiledSchema
}

// JSON returns the raw JSON Schema bytes. This is the extraction point for
// passing schemas across package boundaries that use json.RawMessage
// (e.g. provider.Tool.InputSchema).
func (s Schema) JSON() json.RawMessage { return s.raw }

// Validate validates JSON data against the pre-compiled schema.
// Returns nil if validation succeeds, or an error with JSON pointer
// details indicating which part of the data failed.
func (s Schema) Validate(data json.RawMessage) error {
	if s.compiled == nil {
		return fmt.Errorf("schema: no compiled schema available")
	}
	return s.compiled.Validate(data)
}

// MarshalJSON implements json.Marshaler. The result is the raw JSON Schema
// bytes, identical to JSON().
func (s Schema) MarshalJSON() ([]byte, error) {
	if s.raw == nil {
		return []byte("null"), nil
	}
	return s.raw, nil
}

// UnmarshalJSON implements json.Unmarshaler. It reads raw JSON Schema bytes
// and compiles them for validation, making Schema safe for JSON round-trips.
func (s *Schema) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		s.raw = nil
		s.compiled = nil
		return nil
	}
	compiled, err := CompileSchema(json.RawMessage(data))
	if err != nil {
		return err
	}
	s.raw = json.RawMessage(data)
	s.compiled = compiled
	return nil
}

// Compiled returns the underlying CompiledSchema for callers that need
// direct access to the pre-compiled validator.
func (s Schema) Compiled() *CompiledSchema { return s.compiled }

// SchemaFor generates a Schema from a Go type using struct tags.
// It supports rich struct tag annotations via the "jsonschema" tag including
// enum, pattern, title, description, format, default, minimum, maximum, etc.
//
// The schema is cleaned for LLM provider compatibility: $schema and $id fields
// are stripped, and simple $defs/$ref structures are inlined.
func SchemaFor[T any]() (Schema, error) {
	return SchemaForType(reflect.TypeOf((*T)(nil)).Elem())
}

// SchemaForType generates a Schema from a reflect.Type.
// See SchemaFor for details.
func SchemaForType(t reflect.Type) (Schema, error) {
	r := &jsonschema.Reflector{
		DoNotReference: true,
	}
	s := r.ReflectFromType(t)

	data, err := json.Marshal(s)
	if err != nil {
		return Schema{}, fmt.Errorf("schema: marshaling schema: %w", err)
	}

	data, err = cleanSchema(data)
	if err != nil {
		return Schema{}, fmt.Errorf("schema: cleaning schema: %w", err)
	}

	return SchemaFromJSON(json.RawMessage(data))
}

// SchemaFromJSON creates a Schema from raw JSON Schema bytes. The schema is
// compiled immediately for validation; an error is returned if the bytes are
// not valid JSON or not a valid JSON Schema.
func SchemaFromJSON(raw json.RawMessage) (Schema, error) {
	if !json.Valid(raw) {
		return Schema{}, fmt.Errorf("schema: invalid JSON")
	}
	compiled, err := CompileSchema(raw)
	if err != nil {
		return Schema{}, err
	}
	return Schema{raw: raw, compiled: compiled}, nil
}

// SchemaFromFile reads a JSON Schema from a file and returns a Schema.
// The file must contain valid JSON Schema.
func SchemaFromFile(path string) (Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Schema{}, fmt.Errorf("schema: reading schema file: %w", err)
	}
	return SchemaFromJSON(json.RawMessage(data))
}

// CompiledSchema is a pre-compiled JSON Schema for repeated validation.
// It is safe for concurrent use from multiple goroutines.
type CompiledSchema struct {
	schema *validateschema.Schema
}

// CompileSchema pre-compiles a JSON Schema for repeated validation.
// The compiled schema can be reused across multiple Validate calls
// and is safe for concurrent use.
func CompileSchema(raw json.RawMessage) (*CompiledSchema, error) {
	compiler := validateschema.NewCompiler()
	compiler.DefaultDraft(validateschema.Draft2020)

	doc, err := validateschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("schema: parsing schema resource: %w", err)
	}

	if err := compiler.AddResource("schema.json", doc); err != nil {
		return nil, fmt.Errorf("schema: adding schema resource: %w", err)
	}

	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("schema: compiling schema: %w", err)
	}

	return &CompiledSchema{schema: compiled}, nil
}

// Validate validates JSON data against the compiled schema.
// Returns nil if validation succeeds, or an error with JSON pointer
// details indicating which part of the data failed validation.
func (cs *CompiledSchema) Validate(data json.RawMessage) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("schema: unmarshaling data for validation: %w", err)
	}

	if err := cs.schema.Validate(v); err != nil {
		return fmt.Errorf("schema: schema validation failed: %w", err)
	}

	return nil
}

// Validate validates JSON data against a JSON Schema in a single call.
// For repeated validation against the same schema, use CompileSchema
// to avoid recompilation.
func Validate(raw json.RawMessage, data json.RawMessage) error {
	compiled, err := CompileSchema(raw)
	if err != nil {
		return err
	}
	return compiled.Validate(data)
}

// cleanSchema strips $schema and $id fields from a JSON Schema for LLM
// provider compatibility. Most LLM providers do not expect these metadata
// fields and some reject them.
func cleanSchema(data []byte) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return data, nil
	}

	delete(m, "$schema")
	delete(m, "$id")

	if defsRaw, ok := m["$defs"]; ok {
		if refRaw, hasRef := m["$ref"]; hasRef {
			inlined, err := inlineDefs(m, defsRaw, refRaw)
			if err == nil {
				return inlined, nil
			}
		}
	}

	return json.Marshal(m)
}

// inlineDefs attempts to inline a simple $defs/$ref structure where the root
// schema is just a $ref to a single definition. This produces a cleaner
// schema for LLM providers that don't understand $ref.
func inlineDefs(root map[string]json.RawMessage, defsRaw, refRaw json.RawMessage) ([]byte, error) {
	var ref string
	if err := json.Unmarshal(refRaw, &ref); err != nil {
		return nil, err
	}

	const defsPrefix = "#/$defs/"
	if !strings.HasPrefix(ref, defsPrefix) {
		return nil, fmt.Errorf("unsupported ref: %s", ref)
	}
	defName := ref[len(defsPrefix):]

	var defs map[string]json.RawMessage
	if err := json.Unmarshal(defsRaw, &defs); err != nil {
		return nil, err
	}

	defSchema, ok := defs[defName]
	if !ok {
		return nil, fmt.Errorf("definition %q not found", defName)
	}

	var defMap map[string]json.RawMessage
	if err := json.Unmarshal(defSchema, &defMap); err != nil {
		return nil, err
	}

	for k, v := range root {
		if k == "$defs" || k == "$ref" {
			continue
		}
		defMap[k] = v
	}

	delete(defMap, "$schema")
	delete(defMap, "$id")

	return json.Marshal(defMap)
}
