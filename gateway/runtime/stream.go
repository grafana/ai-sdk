package runtime

import (
	"context"
	"sync"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
)

const streamBufferSize = 64

// StreamInvocation exposes one ordered stream and its runtime lifecycle.
type StreamInvocation struct {
	identity      Identity
	parts         chan provider.StreamPart
	done          chan struct{}
	ctx           context.Context
	cancel        context.CancelCauseFunc
	timeoutCancel context.CancelFunc
	includeRaw    bool
	cancelOnce    sync.Once
	waitErr       error
}

func newStreamInvocation(
	identity Identity,
	source <-chan provider.StreamPart,
	ctx context.Context,
	cancel context.CancelCauseFunc,
	timeoutCancel context.CancelFunc,
	includeRaw bool,
) *StreamInvocation {
	invocation := &StreamInvocation{
		identity:      identity,
		parts:         make(chan provider.StreamPart, streamBufferSize),
		done:          make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
		timeoutCancel: timeoutCancel,
		includeRaw:    includeRaw,
	}
	go invocation.forward(source)
	return invocation
}

// Identity returns the immutable invocation identity.
func (invocation *StreamInvocation) Identity() Identity { return invocation.identity }

// Parts returns the single-consumer ordered provider-part stream.
func (invocation *StreamInvocation) Parts() <-chan provider.StreamPart { return invocation.parts }

// Wait blocks until Parts closes and returns only a runtime lifecycle error.
// Provider PartError values are ordinary stream data and do not affect Wait.
func (invocation *StreamInvocation) Wait() error {
	<-invocation.done
	return invocation.waitErr
}

// Cancel requests stream termination. Repeated calls are harmless; the first
// cause wins.
func (invocation *StreamInvocation) Cancel(cause error) {
	invocation.cancelOnce.Do(func() {
		invocation.cancel(cause)
	})
}

func (invocation *StreamInvocation) forward(source <-chan provider.StreamPart) {
	defer close(invocation.done)
	defer close(invocation.parts)
	defer invocation.timeoutCancel()
	defer invocation.cancel(nil)

	for {
		select {
		case <-invocation.ctx.Done():
			invocation.waitErr = streamContextError(invocation.ctx)
			return
		case part, ok := <-source:
			if !ok {
				select {
				case <-invocation.ctx.Done():
					invocation.waitErr = streamContextError(invocation.ctx)
				default:
				}
				return
			}
			if part.Type == provider.PartRaw && !invocation.includeRaw {
				continue
			}
			select {
			case invocation.parts <- part:
			case <-invocation.ctx.Done():
				invocation.waitErr = streamContextError(invocation.ctx)
				return
			}
		}
	}
}

func streamContextError(ctx context.Context) error {
	cause := context.Cause(ctx)
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return failure.Wrap(failure.ErrTimeout, cause)
	case context.Canceled:
		return failure.Wrap(failure.ErrCanceled, cause)
	default:
		return cause
	}
}
