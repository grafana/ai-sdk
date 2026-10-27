// Package v4 implements the strict ProviderWire V4 language-model HTTP
// runtime. It validates the complete registered request shape while executing
// the bounded unary and streaming text and function-tool subset. Clients own
// function execution and send continuation history in subsequent HTTP requests.
// Deferred request capabilities are rejected before provider invocation, and
// unsupported stream part families terminate through the closed safe-error dialect.
//
// The handler bounds only stream idle and drain time. Selection and the model
// call end at the request context's deadline, which the host sets; an ended
// request context is reported as a timeout for an expired deadline and as a
// cancellation otherwise. The handler does not limit the size of requests or
// outputs. Hosts bound the request body:
// the Gateway plugin relies on the plugin host's route message size, and a host
// that applies [net/http.MaxBytesHandler] gets the fixed invalid-request
// document for a body that exceeds it. The outbound transport
// bounds each provider response, and the host owns admission.
package v4
