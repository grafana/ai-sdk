// Package v4 implements the strict ProviderWire V4 language-model HTTP
// runtime. It validates the complete registered request shape while executing
// the bounded unary and streaming text and function-tool subset. Clients own
// function execution and send continuation history in subsequent HTTP requests.
// Deferred request capabilities are rejected before provider invocation, and
// unsupported stream part families terminate through the closed safe-error dialect.
//
// The handler reads the request body to EOF. Hosts bound it, for example with
// [net/http.MaxBytesHandler]; a body that exceeds the host bound receives the
// fixed invalid-request document.
package v4
