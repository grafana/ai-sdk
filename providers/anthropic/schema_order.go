package anthropic

import (
	"bytes"
	"encoding/json"
	"sort"
)

type orderedSchema struct {
	value    any
	original json.RawMessage
}

func (s orderedSchema) MarshalJSON() ([]byte, error) {
	switch value := s.value.(type) {
	case map[string]any:
		if value == nil {
			return []byte("null"), nil
		}
		var original map[string]json.RawMessage
		if err := json.Unmarshal(s.original, &original); err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(s.original))
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		var keys []string
		seen := make(map[string]bool, len(value))
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key := token.(string)
			var child json.RawMessage
			if err := decoder.Decode(&child); err != nil {
				return nil, err
			}
			if key == "oneOf" && (len(original["anyOf"]) == 0 || bytes.Equal(bytes.TrimSpace(original["anyOf"]), []byte("null"))) {
				if _, ok := value["oneOf"]; !ok {
					key = "anyOf"
					original[key] = original["oneOf"]
				}
			}
			if _, ok := value[key]; ok && !seen[key] {
				keys = append(keys, key)
				seen[key] = true
			}
		}
		var added []string
		for key := range value {
			if !seen[key] {
				added = append(added, key)
			}
		}
		sort.Strings(added)
		keys = append(keys, added...)
		var out bytes.Buffer
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			encodedKey, err := json.Marshal(key)
			if err != nil {
				return nil, err
			}
			out.Write(encodedKey)
			out.WriteByte(':')
			child := value[key]
			var encoded []byte
			if raw := original[key]; len(raw) > 0 {
				encoded, err = json.Marshal(orderedSchema{value: child, original: raw})
			} else {
				encoded, err = json.Marshal(child)
			}
			if err != nil {
				return nil, err
			}
			out.Write(encoded)
		}
		out.WriteByte('}')
		return out.Bytes(), nil
	case []any:
		if value == nil {
			return []byte("null"), nil
		}
		var original []json.RawMessage
		if err := json.Unmarshal(s.original, &original); err != nil {
			return nil, err
		}
		ordered := make([]any, len(value))
		for i, child := range value {
			ordered[i] = child
			if i < len(original) {
				ordered[i] = orderedSchema{value: child, original: original[i]}
			}
		}
		return json.Marshal(ordered)
	default:
		return json.Marshal(value)
	}
}
