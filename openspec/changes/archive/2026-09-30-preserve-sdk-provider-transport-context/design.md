## Context

Issue [#229](https://github.com/grafana/ai-sdk/issues/229) concerns native provider observability, not Gateway transport policy. The registered reference is `4e8c387622ee1bb0d55841664416d38754d5c9a3`: Anthropic 4.0.59, OpenAI 4.0.72, provider-utils 5.0.45. `PARITY.md` classifies provider adapters as mixed and explicitly excludes returned SDK metadata from request-capture proof.

Anthropic drops call headers and uses the SDK's typed streaming iterator, which filters pings and hides malformed/error frames. OpenAI already forwards call headers and owns a frame pump, but neither adapter returns complete transport metadata or emits raw parts. OpenAI unary conversion already retains full response JSON. Core copies stream request metadata into steps, but reads response headers from `PartResponseMeta`, not `StreamResult.Response`.

Existing SDKs expose public middleware and response hooks: anthropic-sdk-go v1.75.0 and openai-go/v3 v3.66.0. Vertex rewrites request bodies through middleware; Bedrock Mantle reuses a preconfigured OpenAI signing client.

## Goals / Non-Goals

**Goals:**

- Honor Anthropic per-call headers and preserve existing OpenAI precedence.
- Return outbound JSON, actual response headers, and unary response JSON through existing types.
- Expose opt-in event evidence before normalization, including failures hidden by typed iteration.
- Preserve client/auth/retry ownership, core lifecycle, and Gateway privacy.
- Prove provider-returned and high-level metadata with deterministic HTTP tests.

**Non-Goals:**

- New public types, SDK upgrades, shared transport framework, or replacement authentication.
- Full upstream schema-validation parity or a general stream-error/EOF redesign.
- Provider-options/capabilities changes, ProviderWire redesign, or enabling Gateway raw output.

## Decisions

### 1. Capture actual outbound JSON within the existing SDK pipeline

Use invocation-local middleware after SDK serialization and body-changing middleware, plus public response hooks. Capture the successful/final attempt rather than an earlier retry. Preserve body bytes, content length, and independent replayability; do not assume `GetBody` cannot move a shared reader. Capture errors must propagate rather than return knowingly incorrect metadata.

Reject marshaling buildParams as transport evidence: it misses JSON overrides, streaming flags, and Vertex transformations. Do not replace a configured HTTP client or record request auth headers. Retain SDK response JSON through `RawJSON()` for unary results instead of reading an already-consumed body.

This proposes an explicit observability adaptation: upstream sometimes reports logical/pre-transform arguments, including an OpenAI streaming body without its HTTP stream flag. Go Request.Body will represent the actual outbound JSON. Tests compare semantic JSON; whitespace and object-member order are not cross-language contract evidence. Caller-supplied non-JSON SDK body overrides cannot be represented as request JSON; omit Request.Body for that unsupported metadata case rather than store invalid JSON or change the payload.

### 2. Separate ordinary header precedence from Anthropic beta union

Append ordinary call-header options after constructor options. Merge beta header values from configured, call, and generated feature sources case-insensitively by HTTP header name, normalize beta tokens to lowercase, trim blanks, and deduplicate. Preserve feature-required betas rather than replacing them with caller values. Perform composition before sending, without mutating stored options or caller maps.

Reject naive beta `WithHeader` replacement: generated SDK message methods append feature beta values, while replacement drops configured values. OpenAI already implements ordinary precedence and needs regression protection, not a second header mechanism.

### 3. Keep transport capture adapter-local and enrich existing metadata events

Populate provider GenerateResult.Request, GenerateResult.Response.Headers/Body, and StreamResult.Request/Response. Include response headers in the existing `PartResponseMeta` emission, allowing current core step/result propagation to work without a new event or root type. Never create duplicate response-metadata events just to carry headers. Flatten multi-valued headers with comma-separated values, matching the native HTTP adapters; clone maps for call isolation.

Test completed core steps and final response accessors through StreamText, GenerateText, and Agent. GenerateText/Agent.Generate collect streams, so unary-provider tests alone are insufficient. Prefer provider changes over adding a generic core fallback solely for these adapters; streams without identity metadata retain their existing lifecycle.

### 4. Decode Anthropic frames before typed normalization

Obtain the raw response through the existing SDK client's public Post/Execute path and acquire one SDK framing Decoder, then decode into the existing event union/adapter. Reproduce the generated streaming message method's endpoint, beta-header translation, streaming flag, and relevant request behavior. A bounded deterministic test/prototype is the first implementation task and must demonstrate equivalence to the current streaming request before migration.

Reject successful-event RawJSON-only emission: it cannot expose pings or malformed/error frames. Reject global RegisterDecoder mutation and parallel whole-stream tees. Do not substitute unary Messages.New with a response override: it adds unary timeout/output-processing behavior. Preserve initial-error wrapping, provider error classification, and current lifecycle/recovery contracts; raw selection must not change normalized outcomes. Any newly discovered upstream recovery mismatch must be classified, not silently accepted as full parity or folded into this change without scope review.

OpenAI extends its existing responseStreamItem with raw evidence through pump, preflight buffering, and consumption. It keeps the existing framing decoder, accepted-stream grace period, recoverable malformed-event handling, and cancellation path.

### 5. Raw means JSON event evidence, not wire-byte archival

Emit stream-start first, then one raw part before each event's normalized parts when requested. Include Anthropic pings and ignored JSON events; exclude SSE comments, empty non-data frames, and `[DONE]`. Preserve original valid JSON, including unknown fields and provider error envelopes, even when typed decoding fails. Invalid JSON produces nil RawValue followed by an error when the SDK would decode that event, representing upstream's undefined rawValue; JSON `null` remains distinct valid raw JSON. Anthropic retains the SDK's event-name filtering: malformed ping/unknown payloads remain ignored by normalization but are raw-observable with nil RawValue.

Emit no raw parts when omitted/false. Do not accumulate a whole stream or duplicate raw storage when disabled beyond decoding/error needs. Preflight errors return no stream result, so they cannot also deliver raw parts; post-preflight errors retain raw-before-error ordering. Closing/canceling owns decoder cleanup once and must unblock channel sends with a stalled consumer.

Both adapters limit startup retention to 64 data frames and 1 MiB of data, independently of raw selection. Exceeding either limit returns a non-retryable setup error and cancels the decoder without handing off a stream or truncating raw evidence. This approved resource-safety adaptation preserves ordinary initial-error behavior but rejects oversized/pathological startup prefixes.

### 6. Preserve the Gateway boundary

Keep native metadata local to native provider/core surfaces. Gateway continues rejecting raw-output requests and projecting only its allowlisted fields; backend headers and request/response bodies must not cross that boundary. Existing privacy tests remain acceptance gates, not a reason to broaden Gateway DTOs.

## Risks / Trade-offs

- [Capture changes replay/signing behavior] → Test retries, Vertex rewriting, Mantle authentication, and preconfigured/custom transports; preserve bytes and replay ownership.
- [Anthropic lower-level request misses convenience-method behavior] → Prove request equivalence and error preflight before migrating; stop if the SDK path requires an unapproved auth/API change.
- [Raw parts add sensitive event content and buffering] → Keep output opt-in, queues bounded, and Gateway policy unchanged; do not add automatic logging.
- [SDK parsing differs from upstream schemas] → Test pinned supported inputs and focused malformed cases; report unproved schema/recovery behavior separately rather than claim full parity.
- [Metadata visible only at provider boundary] → Require high-level result/step tests in addition to HTTP capture.

## Migration Plan

Implement failing focused tests first, then headers/metadata and frame-level raw support incrementally. Do not fabricate recorded provider fixtures. Run affected candidate-source module/core tests, Gateway privacy regressions, and the parity gates with workspace conformance. Public-module (`GOWORK=off`) validation is excluded from this implementation scope. Add schema-parsed cross-language regression evidence if frontend wire behavior changes. Update durable parity coverage only after tests establish the new boundary; record the outbound-body adaptation without copying an issue backlog into PARITY.md.

No public API migration is needed. Rollback reverts adapter-local implementation and its new tests without changing client construction or Gateway policy. Completion requires both adapters and result propagation; metadata-only progress does not close #229.

## Open Questions

No blocking product choice remains in this proposal: actual outbound JSON, nil invalid raw values, and frame-level decoding are the proposed defaults, subject to approval before implementation. The Anthropic raw-response prototype must verify generated-request equivalence and resource ownership. If it cannot, stop and revise the design rather than ship typed-event-only support as completion.
