// Package v4 implements the strict ProviderWire V4 language-model HTTP
// runtime. It validates the complete registered request shape and supports
// bounded text generation and function calls in unary and streaming responses.
// Clients execute tools and send their results in later requests.
// Deferred request capabilities are rejected before provider invocation, and
// unsupported stream parts terminate through the closed safe-error dialect.
package v4
