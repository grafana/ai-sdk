package grafana

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

func validJSON(data []byte) bool { return utf8.Valid(data) && json.Valid(data) }

func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return nil, errors.New("grafana: expected JSON object")
	}
	return object, nil
}

func decodeFields(data []byte, target any, names ...string) error {
	object, err := decodeObject(data)
	if err != nil {
		return err
	}
	return decodeSelectedFields(object, target, names...)
}

func decodeSelectedFields(object map[string]json.RawMessage, target any, names ...string) error {
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
