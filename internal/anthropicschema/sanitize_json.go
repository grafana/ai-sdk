package anthropicschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// SanitizeJSON sanitizes a raw JSON schema like Sanitize and encodes the
// result with every object's members in the order the input lists them.
//
// Anthropic's constrained decoder writes object properties in the order the
// schema lists them, so callers use property order to make the model write,
// for example, its reasoning before its verdict. Sanitize works on Go maps,
// which encoding/json writes in alphabetical order, so encoding its result
// directly would reorder every properties object. Upstream keeps the order
// because JavaScript objects keep insertion order.
func SanitizeJSON(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if schema == nil {
		return nil, fmt.Errorf("anthropicschema: schema must be an object")
	}
	var out bytes.Buffer
	if err := encodeInOrder(&out, Sanitize(schema), raw); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// encodeInOrder writes value, listing each object's members in the order of
// the matching object in original. Members original does not have, such as the
// additionalProperties Sanitize adds, follow in alphabetical order. Arrays
// pair with original by index.
func encodeInOrder(out *bytes.Buffer, value any, original json.RawMessage) error {
	switch value := value.(type) {
	case map[string]any:
		return encodeObjectInOrder(out, value, original)
	case []any:
		var originalItems []json.RawMessage
		_ = json.Unmarshal(original, &originalItems)
		out.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				out.WriteByte(',')
			}
			var originalItem json.RawMessage
			if i < len(originalItems) {
				originalItem = originalItems[i]
			}
			if err := encodeInOrder(out, item, originalItem); err != nil {
				return err
			}
		}
		out.WriteByte(']')
		return nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		out.Write(encoded)
		return nil
	}
}

func encodeObjectInOrder(out *bytes.Buffer, value map[string]any, original json.RawMessage) error {
	members, err := objectMembers(original)
	if err != nil {
		return err
	}
	originalHas := func(name string) bool {
		return slices.ContainsFunc(members, func(m objectMember) bool { return m.name == name })
	}

	written := make(map[string]bool, len(value))
	out.WriteByte('{')
	write := func(key string, originalValue json.RawMessage) error {
		child, ok := value[key]
		if !ok || written[key] {
			return nil
		}
		if len(written) > 0 {
			out.WriteByte(',')
		}
		written[key] = true
		name, err := json.Marshal(key)
		if err != nil {
			return err
		}
		out.Write(name)
		out.WriteByte(':')
		return encodeInOrder(out, child, originalValue)
	}

	for _, member := range members {
		key := member.name
		// Sanitize rewrites oneOf as anyOf, and drops it when anyOf exists.
		if key == "oneOf" {
			if originalHas("anyOf") {
				continue
			}
			key = "anyOf"
		}
		if err := write(key, member.value); err != nil {
			return err
		}
	}

	rest := make([]string, 0, len(value)-len(written))
	for key := range value {
		if !written[key] {
			rest = append(rest, key)
		}
	}
	slices.Sort(rest)
	for _, key := range rest {
		if err := write(key, nil); err != nil {
			return err
		}
	}
	out.WriteByte('}')
	return nil
}

type objectMember struct {
	name  string
	value json.RawMessage
}

// objectMembers lists the members of a JSON object in order. A repeated name
// keeps its first position and its last value, which is the value
// encoding/json decodes. It returns no members when original is not an
// object.
func objectMembers(original json.RawMessage) ([]objectMember, error) {
	decoder := json.NewDecoder(bytes.NewReader(original))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, nil
	}
	var members []objectMember
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("anthropicschema: expected object member name")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(members, func(m objectMember) bool { return m.name == name }); i >= 0 {
			members[i].value = value
			continue
		}
		members = append(members, objectMember{name: name, value: value})
	}
	return members, nil
}
