package v4

import (
	"bytes"
	"encoding/json"
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
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}
