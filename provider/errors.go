package provider

import "errors"

// ErrUnsupportedFunctionality reports that a provider adapter rejected a
// request before any provider I/O because it contains content or settings the
// adapter cannot represent. It is distinct from [APICallError]: no call was
// made, so it carries no status code and is never retryable. Adapters wrap it
// with context using %w and callers match it with errors.Is.
//
// This mirrors the intent of the upstream UnsupportedFunctionalityError.
var ErrUnsupportedFunctionality = errors.New("provider: unsupported functionality")
