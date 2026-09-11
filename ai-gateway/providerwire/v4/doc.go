// Package v4 implements the strict ProviderWire V4 unary language-model HTTP
// runtime. It validates the complete registered request shape while executing
// only the bounded text-generation subset; streaming and other deferred
// capabilities are rejected before provider invocation.
package v4
