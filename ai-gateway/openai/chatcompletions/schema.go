package chatcompletions

import (
	"bytes"
	"encoding/json"

	validateschema "github.com/santhosh-tekuri/jsonschema/v6"
)

type chatSchema struct{ compiled *validateschema.Schema }

func compileSchema(raw json.RawMessage) (*chatSchema, error) {
	if !jsonObject(raw) {
		return nil, errRequest
	}
	document, err := validateschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, errRequest
	}
	compiler := validateschema.NewCompiler()
	compiler.DefaultDraft(validateschema.Draft2020)
	compiler.UseLoader(validateschema.SchemeURLLoader{})
	if err := compiler.AddResource("schema.json", document); err != nil {
		return nil, errRequest
	}
	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, errRequest
	}
	return &chatSchema{compiled: compiled}, nil
}

func (s *chatSchema) Validate(raw json.RawMessage) error {
	document, err := validateschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return s.compiled.Validate(document)
}
