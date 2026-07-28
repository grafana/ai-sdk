// Package middleware provides a model-agnostic interception layer for
// provider.LanguageModel. A Middleware is a struct with optional function
// fields that can transform call parameters, wrap generate/stream
// operations, and override model metadata.
//
// Middlewares compose via WrapLanguageModel, which returns a new
// LanguageModel transparent to all consumers (StreamText, GenerateText, etc.).
//
// Built-in middlewares:
//   - DefaultSettings: applies fallback values for CallOptions fields
//   - SimulateStreaming: wraps non-streaming models to present a streaming interface
//   - ExtractReasoning: strips XML-tagged reasoning from text output into reasoning parts
//
// Utilities:
//   - TransformStream: channel-based stream transformation with context cancellation and flush support
package middleware
