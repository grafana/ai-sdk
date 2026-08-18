package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type jsonObjectMember struct {
	name  string
	value json.RawMessage
}

func decodeJSONObject(input json.RawMessage) ([]jsonObjectMember, error) {
	decoder := json.NewDecoder(bytes.NewReader(input))
	start, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if start != json.Delim('{') {
		return nil, fmt.Errorf("expected JSON object")
	}

	var members []jsonObjectMember
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := nameToken.(string)
		if !ok {
			return nil, fmt.Errorf("expected JSON object member name")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		members = append(members, jsonObjectMember{name: name, value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return members, nil
}

func encodeJSONObject(members []jsonObjectMember) (json.RawMessage, error) {
	var output bytes.Buffer
	output.WriteByte('{')
	for index, member := range members {
		if index > 0 {
			output.WriteByte(',')
		}
		name, err := json.Marshal(member.name)
		if err != nil {
			return nil, err
		}
		output.Write(name)
		output.WriteByte(':')
		output.Write(member.value)
	}
	output.WriteByte('}')
	return output.Bytes(), nil
}

func removeJSONObjectMember(input json.RawMessage, name string) (json.RawMessage, error) {
	members, err := decodeJSONObject(input)
	if err != nil {
		return nil, err
	}

	filtered := make([]jsonObjectMember, 0, len(members))
	for _, member := range members {
		if member.name != name {
			filtered = append(filtered, member)
		}
	}
	return encodeJSONObject(filtered)
}

func prependJSONObjectMember(input json.RawMessage, name string, value json.RawMessage) (json.RawMessage, error) {
	members, err := decodeJSONObject(input)
	if err != nil {
		return nil, err
	}

	ordered := []jsonObjectMember{{name: name, value: value}}
	for _, member := range members {
		if member.name != name {
			ordered = append(ordered, member)
		}
	}
	return encodeJSONObject(ordered)
}
