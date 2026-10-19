package jsonschema

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// DecodeObject decodes one JSON object without rounding numbers. Numeric tokens
// use RawMessage so both encoding/json and native SDK marshalers retain them.
func DecodeObject(raw json.RawMessage) (map[string]any, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("jsonschema: invalid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("jsonschema: schema must be an object")
	}
	preserveNumbers(object)
	return object, nil
}

func preserveNumbers(value any) any {
	switch value := value.(type) {
	case json.Number:
		return json.RawMessage(value)
	case map[string]any:
		for key, child := range value {
			value[key] = preserveNumbers(child)
		}
	case []any:
		for i, child := range value {
			value[i] = preserveNumbers(child)
		}
	}
	return value
}
