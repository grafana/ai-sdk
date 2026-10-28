// Package v4 implements the strict ProviderWire V4 language-model HTTP codec
// over the Gateway execution runtime. It validates the complete registered
// request shape, maps it to provider call options, and encodes unary results,
// SSE frames and its fixed error documents for the bounded unary and streaming
// text and function-tool subset. Clients own function execution and send
// continuation history in subsequent HTTP requests. Deferred request
// capabilities are rejected before provider invocation, and unsupported stream
// part families terminate through the closed safe-error dialect.
//
// The runtime selects and invokes the model, bounds stream idle and drain time,
// and classifies failures; this package maps each failure category to its
// document and owns commitment and the stream state machine. Neither limits
// the size of requests or outputs. Hosts bound the request body: the Gateway
// plugin relies on the plugin host's route message size, and a host that
// applies [net/http.MaxBytesHandler] gets the fixed invalid-request document
// for a body that exceeds it. The outbound transport bounds each provider
// response, and the host owns admission.
package v4
