package fallback

import "context"

type attemptObserverKey struct{}

// WithAttemptObserver registers request-scoped observation without mutating a
// shared model. The observer runs before the model observer at each decision;
// panics are recovered independently. A nil observer disables inherited request
// observation. Callbacks must return promptly and synchronize shared state.
// Repeated or nested fallback calls are observed too; indices restart per call.
// Native source errors remain in-process and require safe projection before
// publication in metadata or responses.
func WithAttemptObserver(ctx context.Context, observer AttemptObserver) context.Context {
	return context.WithValue(ctx, attemptObserverKey{}, observer)
}
