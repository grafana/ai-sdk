package execution

import (
	"encoding/json"
	"slices"
	"strings"
)

// SummaryRedactor protects configured API keys in projected native failure fields.
type SummaryRedactor struct {
	keys     []string
	replacer *strings.Replacer
}

// NewSummaryRedactor copies configured keys into an immutable summary policy.
func NewSummaryRedactor(keys []string) SummaryRedactor {
	keys = slices.Clone(keys)
	slices.SortFunc(keys, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	keys = slices.DeleteFunc(slices.Compact(keys), func(key string) bool { return key == "" })
	redactor := SummaryRedactor{keys: keys}
	if len(keys) > 0 {
		pairs := make([]string, 0, len(keys)*2)
		for _, key := range keys {
			pairs = append(pairs, key, "[REDACTED]")
		}
		redactor.replacer = strings.NewReplacer(pairs...)
	}
	return redactor
}

// Redact protects summary fields without changing the projected source.
func (redactor SummaryRedactor) Redact(failure *Failure) *Failure {
	if failure == nil || redactor.replacer == nil {
		return failure
	}
	result := *failure
	result.Message = redactor.text(result.Message)
	result.Type = redactor.text(result.Type)
	if len(result.Code) > 0 && result.Code[0] == '"' {
		code := readString(result.Code)
		protected := redactor.text(code)
		if code != "" && protected == "" {
			result.Code = nil
		} else {
			result.Code, _ = json.Marshal(protected)
		}
	}
	return availableFailure(&result)
}

func (redactor SummaryRedactor) text(value string) string {
	value = redactor.replacer.Replace(value)
	for _, key := range redactor.keys {
		if strings.Contains(value, key) {
			return ""
		}
	}
	return value
}
