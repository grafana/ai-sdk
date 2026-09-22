package grafana

import (
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"
)

func validJSON(data []byte) bool {
	if !utf8.Valid(data) || !json.Valid(data) {
		return false
	}
	quoted := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || data[i] != '\\' {
			continue
		}
		i++
		if data[i] != 'u' {
			continue
		}
		value, _ := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
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
