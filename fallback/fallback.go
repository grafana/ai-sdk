package fallback

import (
	"context"
	"errors"
	"regexp"

	"github.com/grafana/ai-sdk/provider"
)

// ErrNoCandidates is returned by New when no candidates are provided.
var ErrNoCandidates = errors.New("fallback: at least one candidate is required")

// Model wraps an ordered list of LanguageModel candidates and itself
// implements LanguageModel. It tries candidates in order, falling back
// on errors according to the configured decider.
type Model struct {
	candidates []provider.LanguageModel
	decider    func(error) bool
}

// New creates a FallbackModel from the given candidates.
// Returns ErrNoCandidates if no candidates are provided.
func New(candidates ...provider.LanguageModel) (*Model, error) {
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	return &Model{
		candidates: candidates,
		decider:    defaultDecider,
	}, nil
}

// WithDecider sets a custom fallback decider. The decider returns true
// if the error should trigger fallback to the next candidate.
func (m *Model) WithDecider(fn func(error) bool) *Model {
	m.decider = fn
	return m
}

func (m *Model) SpecificationVersion() string               { return m.candidates[0].SpecificationVersion() }
func (m *Model) Provider() string                           { return m.candidates[0].Provider() }
func (m *Model) ModelID() string                            { return m.candidates[0].ModelID() }
func (m *Model) SupportedURLs() map[string][]*regexp.Regexp { return m.candidates[0].SupportedURLs() }

func (m *Model) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	var lastErr error
	for _, c := range m.candidates {
		if ctx.Err() != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ctx.Err()
		}
		result, err := c.DoGenerate(ctx, params)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !m.decider(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (m *Model) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	var lastErr error
	for _, c := range m.candidates {
		if ctx.Err() != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ctx.Err()
		}

		result, err := c.DoStream(ctx, params)
		if err != nil {
			lastErr = err
			if !m.decider(err) {
				return nil, err
			}
			continue
		}

		var first provider.StreamPart
		var ok bool
		select {
		case first, ok = <-result.Stream:
		case <-ctx.Done():
			go func() {
				for range result.Stream {
				}
			}()
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ctx.Err()
		}

		if !ok {
			return result, nil
		}

		if first.Type == provider.PartError {
			go func() {
				for range result.Stream {
				}
			}()
			// Synthesize a non-retryable APICallError when a producer emits
			// a PartError without a populated APICallError. Without this
			// the fallback would lastErr=nil and could ultimately return
			// (nil, nil) on the final attempt, which violates the
			// (value, error) contract.
			apiErr := first.APICallError
			if apiErr == nil {
				apiErr = provider.NewAPICallError(provider.APICallErrorOptions{
					Message: "PartError received without APICallError details (provider bug or wire decoding issue)",
				})
			}
			var partErr error = apiErr
			lastErr = partErr
			if !m.decider(partErr) {
				return nil, partErr
			}
			continue
		}

		ch := make(chan provider.StreamPart, 64)
		go func() {
			defer close(ch)
			ch <- first
			for part := range result.Stream {
				ch <- part
			}
		}()
		return &provider.StreamResult{
			Stream:   ch,
			Request:  result.Request,
			Response: result.Response,
		}, nil
	}
	return nil, lastErr
}

// defaultDecider returns true (try next candidate) for retryable API errors
// and unknown errors. Returns false (stop) for non-retryable API errors. It also
// returns false for context-window/context-length failures, because the next
// candidate would fail identically.
func defaultDecider(err error) bool {
	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		if isContextWindowError(apiErr) {
			return false
		}
		return apiErr.IsRetryable
	}
	return true
}

// contextLengthSignal matches the common ways providers phrase a context-window
// overflow. This heuristic is intentionally confined to the fallback decider --
// upstream has no context-window error category, and we do not expose one.
var contextLengthSignal = regexp.MustCompile(`(?i)context (length|window)|maximum context|too many tokens|prompt is too long`)

// isContextWindowError reports whether apiErr represents a context-window/
// context-length overflow. Anthropic surfaces this as an invalid_request_error
// at HTTP 400 with a context-length message rather than a dedicated type, so we
// match on status plus the structured Data/Message signal.
func isContextWindowError(apiErr *provider.APICallError) bool {
	if apiErr.StatusCode != 400 {
		return false
	}
	if contextLengthSignal.MatchString(apiErr.Message) {
		return true
	}
	if len(apiErr.Data) > 0 && contextLengthSignal.Match(apiErr.Data) {
		return true
	}
	return false
}
