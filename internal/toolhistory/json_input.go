package toolhistory

import (
	"encoding/json"
	"strings"
)

const jsonWhitespace = " \t\r\n"

// Object parses JSON text stored as a tool input string and accepts only objects.
func Object(text string) (map[string]any, bool) {
	value, ok := parse(text)
	if !ok {
		return nil, false
	}
	object, ok := value.(map[string]any)
	return object, ok
}

// Arguments unwraps one JSON string when its contents are already serialized arguments.
func Arguments(input string) string {
	if !strings.HasPrefix(strings.TrimLeft(input, jsonWhitespace), `"`) {
		return input
	}
	var text string
	if json.Unmarshal([]byte(input), &text) != nil {
		return input
	}
	if _, ok := parse(text); ok {
		return text
	}
	return input
}

func parse(text string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil || strings.Trim(text[dec.InputOffset():], jsonWhitespace) != "" {
		return nil, false
	}
	return value, !hasForbiddenPrototypeKeys(value)
}

func hasForbiddenPrototypeKeys(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if _, ok := v["__proto__"]; ok {
			return true
		}
		if constructor, ok := v["constructor"].(map[string]any); ok {
			if _, ok := constructor["prototype"]; ok {
				return true
			}
		}
		for _, child := range v {
			if hasForbiddenPrototypeKeys(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if hasForbiddenPrototypeKeys(child) {
				return true
			}
		}
	}
	return false
}
