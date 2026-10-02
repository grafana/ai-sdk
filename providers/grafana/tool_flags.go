package grafana

import "encoding/json"

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
