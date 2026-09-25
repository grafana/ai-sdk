## ADDED Requirements

### Requirement: Provider streaming HTTP headers reach completed core steps
`StreamText` SHALL initialize each step's response headers from that step's `provider.StreamResult.Response.Headers` when available. A non-nil `PartResponseMeta.ResponseHeaders` SHALL override those headers for that step; a metadata part without response headers SHALL preserve the stream-result headers. The step's `Response`, `OnStepFinish` callback, and final `StreamTextResult.Response()` SHALL expose the selected headers from the appropriate completed step. Request metadata SHALL continue to come from that same step's `StreamResult.Request`. These provider result headers and request values SHALL NOT by themselves be emitted as frontend SSE chunks.

#### Scenario: Stream result headers with ordinary metadata part
- **WHEN** a model returns a stream with request JSON and HTTP headers and later emits a response-metadata part containing ID/model but no HTTP headers
- **THEN** the completed step, `OnStepFinish`, and final response retain the stream result's headers and the step request retains its request JSON

#### Scenario: Explicit part headers supersede the stream fallback
- **WHEN** a stream result supplies HTTP headers and a later response-metadata part explicitly supplies different non-nil HTTP headers
- **THEN** the completed step and final response use the part's headers

#### Scenario: Multiple completed steps do not leak previous transport data
- **WHEN** a tool loop completes more than one provider call with different request bodies and HTTP headers
- **THEN** each recorded step and callback contains its own metadata, and the final result reports only the last completed step's response
