package v4

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

const maxRawUsageBytes = 1 << 20

func validRawUsage(raw json.RawMessage, limit int64) bool {
	if len(raw) == 0 {
		return true
	}
	if int64(len(raw)) > limit || len(raw) > maxRawUsageBytes || !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	first := 0
	for first < len(raw) && (raw[first] == ' ' || raw[first] == '\n' || raw[first] == '\r' || raw[first] == '\t') {
		first++
	}
	if first == len(raw) || raw[first] != '{' {
		return false
	}
	for i := first + 1; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		for i++; i < len(raw) && raw[i] != '"'; i++ {
			if raw[i] != '\\' {
				continue
			}
			i++
			if raw[i] != 'u' {
				continue
			}
			value, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			i += 4
			if value >= 0xdc00 && value <= 0xdfff {
				return false
			}
			if value >= 0xd800 && value <= 0xdbff {
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, _ := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			}
		}
	}
	return true
}
