// Package ptr provides mechanical pointer operations without defining optional-value policy.
package ptr

// To returns a pointer to value.
func To[T any](value T) *T { return &value }

// Clone returns a shallow copy of value or nil when value is nil.
func Clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return To(*value)
}

// Deref returns the pointed-to value or fallback when value is nil.
func Deref[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
