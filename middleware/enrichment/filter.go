package enrichment

import (
	"context"
	"strings"
)

var secretKeyFragments = []string{
	"authorization",
	"x-api-key",
	"api-key",
	"apikey",
	"token",
	"access_token",
	"refresh_token",
	"id_token",
	"password",
	"secret",
	"credential",
	"cookie",
	"set-cookie",
}

type defaultRedactor struct{}

// DefaultRedactor marks known secret-looking keys as sensitive.
func DefaultRedactor() Redactor { return defaultRedactor{} }

func (defaultRedactor) RedactValue(_ context.Context, value Value) (Value, bool) {
	lower := strings.ToLower(value.Key)
	for _, fragment := range secretKeyFragments {
		if strings.Contains(lower, fragment) {
			value.Sensitive = true
			return value, true
		}
	}
	return value, true
}

func collectValues(ctx context.Context, opts normalizedOptions, input CallInput) ([]Value, error) {
	values := cloneValues(opts.values)
	if opts.contextValues {
		values = append(values, ValuesFromContext(ctx)...)
	}
	if opts.dynamicValues != nil {
		dynamic, err := opts.dynamicValues(ctx, input)
		if err != nil {
			return nil, err
		}
		values = append(values, cloneValues(dynamic)...)
	}
	return values, nil
}

func normalizeValues(ctx context.Context, values []Value, opts normalizedOptions) []Value {
	if len(values) == 0 {
		return nil
	}
	out := make([]Value, 0, len(values))
	for _, value := range values {
		value.Key = normalizedKey(value.Key)
		if value.Key == "" {
			continue
		}

		beforeRedaction := value.Value
		if opts.redactor != nil {
			redacted, ok := opts.redactor.RedactValue(ctx, value)
			if !ok {
				continue
			}
			value = redacted
			value.Key = normalizedKey(value.Key)
			if value.Key == "" {
				continue
			}
		}

		if value.Sensitive {
			if !opts.filter.RedactSensitive {
				continue
			}
			if value.Value == beforeRedaction {
				value.Value = redactedValue
			}
		}

		if opts.filter.MaxValueLength > 0 && len(value.Value) > opts.filter.MaxValueLength {
			continue
		}
		if opts.filter.DropHighCardinality && value.Cardinality == CardinalityHigh {
			continue
		}
		out = append(out, value)
	}
	return out
}

func selectedValues(values []Value, include, specific, exclude map[string]struct{}) []Value {
	if len(values) == 0 {
		return nil
	}
	out := make([]Value, 0, len(values))
	for _, value := range values {
		if isSelected(value.Key, include, specific, exclude) {
			out = append(out, value)
		}
	}
	return out
}
