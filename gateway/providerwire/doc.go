// Package providerwire implements the tolerant legacy JSON over HTTP and SSE
// transport for remote [provider.LanguageModel] calls.
//
// The [provider] package remains the transport-agnostic in-process contract.
// This package owns the remote protocol constants and codecs together with the
// server request lifecycle. Hosts provide authentication, authorization, model
// policy, route mounting, and observability around [Handler].
//
// The protocol emits upstream LanguageModelV4-compatible JSON, routes, headers,
// and SSE framing, so both the Go Grafana provider and upstream gateway clients
// can call [Handler]. Decoders also accept legacy Go-to-Go payloads. The strict
// pinned V4 contract is documented by the sibling providerwire/v4 package; it
// does not replace this active handler. This is not the UIMessageChunk protocol
// consumed directly by @ai-sdk/react.
//
// Requests use [PathLanguageModel], [HeaderModelID], [HeaderStreaming], and
// [HeaderSpecVersion]. Unary results are JSON; streaming results are SSE events
// containing JSON [provider.StreamPart] values. A clean stream closes without a
// [DONE] sentinel. Authentication headers are outside this package's scope.
package providerwire
