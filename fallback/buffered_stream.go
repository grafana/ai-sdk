package fallback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

// ErrStreamBufferLimit indicates that a buffered attempt exceeded its byte or part limit.
var ErrStreamBufferLimit = errors.New("fallback: stream buffer limit exceeded")

// ErrIncompleteStream indicates that a buffered stream closed without a finish part.
var ErrIncompleteStream = errors.New("fallback: stream ended without finish")

const maxBufferedStreamParts = 4096

type streamBufferConfig struct {
	maxBytes       int
	attemptTimeout time.Duration
}

// WithBufferedStream delays stream selection until a candidate closes successfully
// after a finish part. Failed attempts emit no parts to the caller. maxBytes limits
// the total JSON-encoded size of the retained parts; at most 4096 parts are retained.
// Both maxBytes and attemptTimeout must be positive. The timeout covers each
// candidate's setup and complete stream, within the parent request's deadline.
// This affects DoStream only and must be configured before concurrent use.
// Use only when delaying all output is acceptable and replaying the request is safe;
// buffering cannot undo side effects from provider-executed tools.
func (m *Model) WithBufferedStream(maxBytes int, attemptTimeout time.Duration) (*Model, error) {
	if maxBytes <= 0 || attemptTimeout <= 0 {
		return nil, errors.New("fallback: buffered stream requires positive byte limit and attempt timeout")
	}
	m.streamBuffer = &streamBufferConfig{maxBytes: maxBytes, attemptTimeout: attemptTimeout}
	return m, nil
}

func (m *Model) doBufferedStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	var failures []failedAttempt
	for i, candidate := range m.candidates {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(failedAttemptsError(failures), err)
		}
		startedAt := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, m.streamBuffer.attemptTimeout)
		result, err := startStream(attemptCtx, candidate, params)
		var parts []provider.StreamPart
		if err == nil {
			parts, err = collectStream(attemptCtx, result, m.streamBuffer.maxBytes)
		}
		cancel()
		if err != nil && result != nil && result.Stream != nil {
			go drainStream(result.Stream)
		}
		outcome, next, err := m.decision(ctx, i, err)
		m.observeAttempt(ctx, i, candidate, startedAt, err, outcome, next)
		if err != nil {
			failures = append(failures, failedAttempt{candidate: candidate, err: err})
			if !next {
				return nil, failedAttemptsError(failures)
			}
			continue
		}
		stream := make(chan provider.StreamPart)
		go func() {
			defer close(stream)
			for _, part := range parts {
				select {
				case stream <- part:
				case <-ctx.Done():
					return
				}
			}
		}()
		return &provider.StreamResult{Stream: stream, Request: result.Request, Response: result.Response}, nil
	}
	return nil, failedAttemptsError(failures)
}

func collectStream(ctx context.Context, result *provider.StreamResult, maxBytes int) ([]provider.StreamPart, error) {
	if result == nil || result.Stream == nil {
		return nil, ErrInvalidResult
	}
	var parts []provider.StreamPart
	var bytes int
	finished := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case part, ok := <-result.Stream:
			if !ok {
				if !finished {
					return nil, ErrIncompleteStream
				}
				return parts, nil
			}
			if part.Type == provider.PartError {
				if part.APICallError == nil {
					return nil, ErrInvalidResult
				}
				return nil, part.APICallError
			}
			if part.Type == provider.PartFinish {
				if part.FinishReason == nil {
					return nil, ErrInvalidResult
				}
				if part.FinishReason.Unified == provider.FinishReasonError {
					return nil, ErrIncompleteStream
				}
				finished = true
			}
			encoded, err := json.Marshal(part)
			if err != nil {
				return nil, fmt.Errorf("%w: cannot encode stream part", ErrInvalidResult)
			}
			if len(parts) >= maxBufferedStreamParts || len(encoded) > maxBytes-bytes {
				return nil, ErrStreamBufferLimit
			}
			bytes += len(encoded)
			parts = append(parts, part)
		}
	}
}
