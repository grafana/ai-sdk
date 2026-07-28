package enrichment

import "context"

type valuesContextKey struct{}

// WithValue appends an enrichment value to ctx using a collision-safe package key.
func WithValue(ctx context.Context, key, value string, opts ...ValueOption) context.Context {
	v := Value{Key: key, Value: value}
	for _, opt := range opts {
		if opt != nil {
			opt(&v)
		}
	}
	return WithValues(ctx, v)
}

// WithValues appends enrichment values to ctx using a collision-safe package key.
func WithValues(ctx context.Context, values ...Value) context.Context {
	if len(values) == 0 {
		return ctx
	}
	existing := ValuesFromContext(ctx)
	combined := make([]Value, 0, len(existing)+len(values))
	combined = append(combined, existing...)
	combined = append(combined, values...)
	return context.WithValue(ctx, valuesContextKey{}, combined)
}

// ValuesFromContext returns enrichment values stored by WithValue or WithValues.
func ValuesFromContext(ctx context.Context) []Value {
	if ctx == nil {
		return nil
	}
	values, ok := ctx.Value(valuesContextKey{}).([]Value)
	if !ok || len(values) == 0 {
		return nil
	}
	return cloneValues(values)
}
