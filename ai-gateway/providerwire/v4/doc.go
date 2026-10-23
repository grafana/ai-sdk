// Package v4 implements the strict ProviderWire V4 language-model HTTP
// runtime. It validates the complete registered request shape while executing
// the bounded unary and streaming text and function-tool subset. Clients own
// function execution and send continuation history in subsequent HTTP requests.
// Deferred request capabilities are rejected before provider invocation, and
// unsupported stream part families terminate through the closed safe-error dialect.
//
// The handler bounds only time: the model, stream idle and drain durations. It
// does not limit the size of requests or outputs. Hosts bound the request body,
// for example with [net/http.MaxBytesHandler], and a body that exceeds that
// bound receives the fixed invalid-request document. The outbound transport
// bounds each provider response, and the host owns admission.
package v4
