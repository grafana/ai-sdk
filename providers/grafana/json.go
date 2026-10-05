package grafana

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

func validJSON(data []byte) bool { return utf8.Valid(data) && json.Valid(data) }

func validToolFlags(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	for _, name := range []string{"providerExecuted", "dynamic", "preliminary", "isError"} {
		if value, ok := fields[name]; ok {
			var marker bool
			if string(value) == "null" || json.Unmarshal(value, &marker) != nil {
				return false
			}
		}
	}
	return true
}

func decodeFields(data []byte, target any, names ...string) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return errors.New("grafana: expected JSON object")
	}
	selected := make(map[string]json.RawMessage, len(names))
	for _, name := range names {
		if value, ok := object[name]; ok {
			selected[name] = value
		}
	}
	filtered, err := json.Marshal(selected)
	if err != nil {
		return err
	}
	return json.Unmarshal(filtered, target)
}
