package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

var errDiscoveryString = errors.New("grafana: invalid discovery string")

func decodeDiscoveryFields(data []byte, target any, names ...string) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return errDiscoveryString
	}
	for _, name := range names {
		if !validDiscoveryStringJSON(fields[name]) {
			return errDiscoveryString
		}
	}
	return decodeFields(data, target, names...)
}

func validDiscoveryStringJSON(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '"' {
		return true
	}
	if len(raw) > 6*maxDiscoveryStringBytes+2 {
		return false
	}
	for i := 1; i < len(raw)-1; i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if !utf16.IsSurrogate(rune(unit)) {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || utf16.DecodeRune(rune(unit), rune(low)) == utf8.RuneError {
			return false
		}
		i += 6
	}
	return true
}
