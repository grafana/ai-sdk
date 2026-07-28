package middleware

import (
	"context"

	"github.com/grafana/ai-sdk/provider"
)

// TransformStream creates a new StreamResult with the stream channel
// transformed by the given function. The transform function receives each
// StreamPart and an emit callback to produce zero, one, or many output
// parts per input part. The emit callback supports stateful buffering
// across chunks.
//
// A flush function can be provided to emit any buffered data when the
// input stream closes. Pass nil if no flush behavior is needed.
//
// The transform goroutine respects context cancellation. Callers must
// either drain the returned stream or cancel the context to avoid
// goroutine leaks.
func TransformStream(
	ctx context.Context,
	result *provider.StreamResult,
	transform func(part provider.StreamPart, emit func(provider.StreamPart)),
	flush func(emit func(provider.StreamPart)),
) *provider.StreamResult {
	ch := make(chan provider.StreamPart, 64)

	go func() {
		defer close(ch)

		emit := func(part provider.StreamPart) {
			select {
			case ch <- part:
			case <-ctx.Done():
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case part, ok := <-result.Stream:
				if !ok {
					if flush != nil {
						flush(emit)
					}
					return
				}
				transform(part, emit)
			}
		}
	}()

	return &provider.StreamResult{
		Stream:   ch,
		Request:  result.Request,
		Response: result.Response,
	}
}
