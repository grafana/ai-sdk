package output

import (
	"context"
	"encoding/json"
	"fmt"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

// OutputAccessor provides access to structured output values.
// Both StreamTextResult and GenerateTextResult satisfy this interface.
type OutputAccessor interface {
	OutputValue() any
	OutputError() error
}

// Value extracts a typed value from a structured output result.
// Returns an error if the output is nil, if output validation failed,
// or if the stored value cannot be type-asserted to T.
func Value[T any](result OutputAccessor) (T, error) {
	var zero T

	if err := result.OutputError(); err != nil {
		return zero, err
	}

	val := result.OutputValue()
	if val == nil {
		return zero, fmt.Errorf("%w: no output produced", aisdk.ErrNoObjectGenerated)
	}

	typed, ok := val.(T)
	if !ok {
		return zero, fmt.Errorf("output: type mismatch: got %T, want %T", val, zero)
	}
	return typed, nil
}

// TypedElementStream wraps ElementStream() with per-element json.Unmarshal
// into the target type T. Elements that fail to unmarshal are silently skipped.
func TypedElementStream[T any](result *aisdk.StreamTextResult) <-chan T {
	out := make(chan T, 64)
	go func() {
		defer close(out)
		for raw := range result.ElementStream() {
			var v T
			if err := json.Unmarshal(raw, &v); err == nil {
				out <- v
			}
		}
	}()
	return out
}

// ObjectResult wraps a GenerateTextResult with typed access to the output.
type ObjectResult[T any] struct {
	*aisdk.GenerateTextResult
}

// Object returns the typed output value.
func (r *ObjectResult[T]) Object() (T, error) {
	return Value[T](r)
}

// OutputValue implements OutputAccessor.
func (r *ObjectResult[T]) OutputValue() any { return r.Output }

// OutputError implements OutputAccessor.
func (r *ObjectResult[T]) OutputError() error { return r.GenerateTextResult.OutputError }

// StreamObjectResult wraps a StreamTextResult with typed access.
type StreamObjectResult[T any] struct {
	*aisdk.StreamTextResult
}

// Object returns the typed final output value. Blocks until stream completes.
func (r *StreamObjectResult[T]) Object() (T, error) {
	return Value[T](r.StreamTextResult)
}

// GenerateObject is a convenience wrapper around GenerateText that provides
// typed structured output. It injects the Output and returns a typed ObjectResult.
func GenerateObject[T any](ctx context.Context, model provider.LanguageModel, out aisdk.Output, opts ...aisdk.GenerateOption) (*ObjectResult[T], error) {
	result, err := aisdk.GenerateText(ctx, model, append(opts, aisdk.WithOutput(out))...)
	if err != nil {
		return nil, err
	}
	return &ObjectResult[T]{GenerateTextResult: result}, nil
}

// StreamObject is a convenience wrapper around StreamText that provides
// typed structured output streaming. It injects the Output and returns
// a typed StreamObjectResult.
func StreamObject[T any](ctx context.Context, model provider.LanguageModel, out aisdk.Output, opts ...aisdk.StreamOption) *StreamObjectResult[T] {
	result := aisdk.StreamText(ctx, model, append(opts, aisdk.WithOutput(out))...)
	return &StreamObjectResult[T]{StreamTextResult: result}
}
