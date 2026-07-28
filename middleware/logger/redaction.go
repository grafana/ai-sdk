package logger

import (
	"context"
	"log/slog"
	"reflect"
	"strings"
)

const redactedValue = "[REDACTED]"

var sensitiveKeyPatterns = []string{
	"authorization",
	"x-api-key",
	"api-key",
	"apikey",
	"access_token",
	"refresh_token",
	"id_token",
	"token",
	"password",
	"secret",
	"credential",
	"set-cookie",
	"cookie",
}

// Redactor transforms log attributes immediately before they are emitted.
type Redactor interface {
	RedactAttrs(ctx context.Context, event EventKind, attrs []slog.Attr) []slog.Attr
}

// RedactorFunc adapts a function into a Redactor.
type RedactorFunc func(ctx context.Context, event EventKind, attrs []slog.Attr) []slog.Attr

// RedactAttrs calls f(ctx, event, attrs).
func (f RedactorFunc) RedactAttrs(ctx context.Context, event EventKind, attrs []slog.Attr) []slog.Attr {
	return f(ctx, event, attrs)
}

// DefaultRedactor returns a redactor for common secret-bearing keys.
func DefaultRedactor() Redactor { return defaultRedactor{} }

// DefaultRedactorWithExtraKeys returns a default redactor extended with additional
// case-insensitive secret-bearing key patterns.
func DefaultRedactorWithExtraKeys(keys ...string) Redactor {
	return defaultRedactor{extraKeys: keys}
}

type defaultRedactor struct {
	extraKeys []string
}

func (r defaultRedactor) RedactAttrs(_ context.Context, _ EventKind, attrs []slog.Attr) []slog.Attr {
	patterns := sensitiveKeyPatterns
	if len(r.extraKeys) > 0 {
		patterns = append(append([]string(nil), sensitiveKeyPatterns...), r.extraKeys...)
	}
	redactor := attrRedactor{patterns: patterns}
	out := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, redactor.redactAttr(attr))
	}
	return out
}

type attrRedactor struct {
	patterns []string
}

func (r attrRedactor) redactAttr(attr slog.Attr) slog.Attr {
	if r.isSensitiveKey(attr.Key) {
		return slog.String(attr.Key, redactedValue)
	}
	attr.Value = r.redactValue(attr.Value)
	return attr
}

func (r attrRedactor) redactValue(value slog.Value) slog.Value {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindGroup:
		group := value.Group()
		redacted := make([]slog.Attr, 0, len(group))
		for _, attr := range group {
			redacted = append(redacted, r.redactAttr(attr))
		}
		return slog.GroupValue(redacted...)
	case slog.KindAny:
		return slog.AnyValue(r.redactAny(value.Any()))
	default:
		return value
	}
}

func (r attrRedactor) redactAny(value any) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if r.isSensitiveKey(key) {
				out[key] = redactedValue
				continue
			}
			out[key] = r.redactAny(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = r.redactAny(item)
		}
		return out
	case []slog.Attr:
		// User-supplied slog.Any attrs may wrap nested slog attrs; middleware-owned
		// payloads are JSON-normalized before redaction.
		out := make([]slog.Attr, 0, len(v))
		for _, attr := range v {
			out = append(out, r.redactAttr(attr))
		}
		return out
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}
	redacted, ok := r.redactReflect(rv)
	if ok {
		return redacted
	}
	return value
}

func (r attrRedactor) redactReflect(value reflect.Value) (any, bool) {
	if !value.IsValid() {
		return nil, true
	}
	if value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil, true
		}
		return r.redactReflect(value.Elem())
	}

	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if r.isSensitiveKey(key) {
				out[key] = redactedValue
				continue
			}
			out[key] = r.redactAny(iter.Value().Interface())
		}
		return out, true
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return value.Interface(), false
		}
		out := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			out[i] = r.redactAny(value.Index(i).Interface())
		}
		return out, true
	default:
		return nil, false
	}
}

func (r attrRedactor) isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range r.patterns {
		if lower == pattern {
			return true
		}
		if pattern == "token" {
			if strings.Contains(lower, "token") && !isUsageTokenCounterKey(lower) {
				return true
			}
			continue
		}
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func isUsageTokenCounterKey(lower string) bool {
	return (strings.HasPrefix(lower, "ai_sdk.usage.") || strings.HasPrefix(lower, "gen_ai.usage.")) &&
		(strings.Contains(lower, "_tokens.") || strings.HasSuffix(lower, "_tokens"))
}
