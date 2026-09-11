// Package v4 implements the strict ProviderWire V4 language-model HTTP
// runtime. It validates the complete registered request shape while executing
// the bounded unary and streaming text-generation subset; deferred request
// capabilities are rejected before provider invocation, and unsupported stream
// part families terminate through the closed safe-error dialect.
package v4
