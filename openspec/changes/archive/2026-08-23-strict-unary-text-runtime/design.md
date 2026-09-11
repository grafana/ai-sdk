## Context

The registered `@ai-sdk/gateway@4.0.52` client sends the complete LanguageModelV4 request object, consumes unary JSON permissively, then replaces unary warnings, request information, and response information with client-owned values. The runtime therefore needs strict ingress, private failure handling, and bounded text output, but does not need a richer server-owned unary result dialect. Gateway protocol, catalog, runtime, and in-process integration source lives in the isolated AGPL module; reusable provider-domain and provider changes remain in the Apache SDK.

## Goals / Non-Goals

**Goals:**
- Preserve the one-way `ai-gateway -> SDK` module and license boundary.
- Execute the supported unary text/scalar subset through one bounded handler.
- Validate the complete registered request before deciding support.
- Keep provider and transport details private.
- Bound handler latency and success encoding.
- Prove compatibility through exact pinned client integration.

**Non-Goals:**
- Streaming execution.
- Tools, files, structured output, provider options, body-header forwarding, or raw output.
- A host-policy abstraction without a concrete consumer.
- Unary warnings or response metadata that supported clients replace.
- Compatibility claims for Vercel's private Gateway service or an unimplemented Go client.

## Decisions

### Keep Gateway implementation inside the AGPL module

`ai-gateway/catalog`, `ai-gateway/providerwire/v4`, and the in-process runtime testserver are AGPL-owned. The root SDK keeps only provider-domain, provider implementation, generic middleware, and conformance changes. The Gateway module remains outside `go.work`, has no committed replace, and pins the immutable proxy-resolvable root prerequisite containing the Apache phase-3 changes.

### Validate complete ingress, then map the supported subset

The handler validates the HTTP envelope, reads through a request byte limit, rejects invalid UTF-8, and applies the complete request schema. A typed private DTO then maps text and scalar fields. Standard Go JSON behavior is intentional: duplicate members use the last value and escaped lone surrogates normalize to U+FFFD. Standard Go integer decoding preserves the required plain integer lexical behavior and range checks. Unsupported branches need only shallow family detection because schema validation already owns their detailed shape.

Alternative: a text-only schema. Rejected because it would misclassify registered unsupported requests as malformed. Alternative: recursively revalidate every unsupported union in the mapper. Rejected because it duplicates schema ownership and adds no client-visible value.

### Do not define multi-capability precedence

Each unsupported family has a stable fixed document. Requests activating one family receive that document. Requests activating several families may receive whichever family the mapper encounters first; that traversal order is not part of the contract.

### Resolve directly without a speculative policy layer

After mapping, the handler resolves the exact model ID and validates a non-empty canonical catalog ID and V4 model. Authentication, authorization, quotas, and request middleware remain host HTTP responsibilities until a concrete model-policy use case defines a better interface.

### Keep non-cooperative model containment

`DoGenerate` runs under request cancellation and a configured duration in a child goroutine with panic recovery and a buffered result handoff. This bounds HTTP handler latency and graceful shutdown waits even if a model ignores context. A permanently blocked implementation may still retain its goroutine.

### Use fixed safe errors

All runtime error bodies are precomputed from finite categories and unsupported families. No cause text is serialized. There is no configurable error-size limit or runtime error-schema validation because the documents are fixed and validated exhaustively in tests.

Provider API status, context, and transport errors reduce to model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, or internal documents. Authentication and permission documents are not owned by this handler after removal of the unused policy abstraction; Gateway client classification for those host-level errors remains tested synthetically.

### Emit the minimal unary result

The server emits only content, finish reason, and usage. Provider warnings, response metadata, canonical ID, backend identity, request/response bodies, raw usage, and provider metadata are omitted. Canonical catalog identity remains an internal routing and telemetry invariant. The Gateway client owns unary warnings, request, and response fields, and raw response-body details outside the minimal document are not guaranteed.

Streaming differs: future streaming work must preserve observable stream warnings, response-metadata parts, lifecycle parts, usage, finish, and errors.

### Bound success preflight and encode with standard JSON before commitment

Before encoding, the mapper uses overflow-safe subtraction to bound content count and aggregate text plus raw-finish bytes. UTF-8 validation runs only after that preflight. A private tagged DTO containing only content, finish reason, and usage is then encoded with `encoding/json`; provider-domain marshalers never control the response. The complete bytes are checked against the unary limit before HTTP 200. Worst-case escaping may allocate a bounded constant multiple of the configured limit, which is acceptable because provider strings are already resident and removes handwritten JSON correctness surface. Success schemas remain test-time contract checks rather than per-response runtime validation.

### Keep one cross-language runtime layer

The AGPL ProviderWire contract workspace builds its co-located in-process testserver with `GOWORK=off` and calls it through the exact pinned Gateway client for minimal success, representative errors, and cancellation. Apache integration tests do not import the Gateway module. The same workspace retains schema/golden checks and synthetic client-class tests.

## Risks / Trade-offs

- Standard JSON decoding accepts duplicate members and normalizes escaped lone surrogates. Supported SDK serializers do not emit those forms, and request bytes remain bounded.
- Removing unary warning and metadata output makes the raw response body intentionally smaller than the full provider result. Supported clients already replace those semantic fields.
- Shallow unsupported detection depends on complete-schema tests remaining synchronized with the registered request surface.
- A defective model can retain one goroutine after the handler returns; latency, not defective-provider resource ownership, is bounded.

## Migration Plan

1. Publish an immutable root prerequisite containing Apache phase-3 changes and no Gateway implementation.
2. Pin the isolated AGPL module to that proxy-resolvable root pseudo-version.
3. Add the catalog, bounded handler, request schema, typed mapper, resolver, fixed errors, and minimal response encoder under `ai-gateway`.
4. Replay committed request goldens and add co-located raw and pinned-client tests for privacy, errors, bounds, and cancellation.
5. Keep streaming deferred and document remaining capability gaps.
