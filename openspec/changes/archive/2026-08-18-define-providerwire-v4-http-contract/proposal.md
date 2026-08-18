## Why

The reusable provider wire is currently defined by tolerant Go codecs and implementation-oriented types, while the pinned `@ai-sdk/gateway` client speaks an HTTP protocol whose exact serialized contract is not checked in or independently executable. Establishing that contract first prevents future strict handler and client work from inventing incompatible request, response, union, extension, error, or SSE semantics.

## What Changes

- Add a contract-only `gateway-providerwire-v4` capability for the source commit and package versions registered in `test/conformance/upstream.yaml`.
- Check in OpenAPI 3.1 for `POST /language-model` and curated JSON Schema 2020-12 contracts for requests, unary results, stream parts, and safe errors.
- Define closed standard objects and exact discriminator-selected union arms while keeping only declared opaque JSON and keyed extension boundaries open.
- Add reproducible, privacy-safe captures from the exact pinned stock Gateway client, storing the HTTP envelope separately from semantic JSON and recording capture ownership and authority.
- Add positive request/response corpora, local negative fixtures, offline schema/OpenAPI validation, and repeatable contract checks.
- Select and document a strict JSON syntax mechanism that rejects duplicate names, trailing values, invalid UTF-8, and invalid Unicode scalar encodings before later schema validation.
- Classify the new contract evidence in the parity coverage map while leaving user-facing provider-wire documentation unchanged until a runtime exists.
- Narrow existing `provider-wire` and `gateway-providerwire-server` requirements to identify them as the active tolerant legacy transport rather than the authority for the new strict V4 contract; their runtime behavior and public API remain unchanged.
- Explicitly defer production V4 decoding, model adaptation, host policy, handler execution, SSE serving, reusable Go client behavior, and Grafana adoption.

## Capabilities

### New Capabilities

- `gateway-providerwire-v4`: Defines the pinned `/language-model` HTTP envelope, machine-readable JSON contracts, strict syntax policy, capture ownership, semantic compatibility rules, validation evidence, baseline evolution, and contract-only lifecycle.

### Modified Capabilities

- `provider-wire`: Reclassifies the existing Go-type JSON/SSE codecs and helpers as the tolerant legacy transport that remains supported during V4 development.
- `gateway-providerwire-server`: Reclassifies the existing handler as the active legacy server and requires continued legacy/Grafana behavior while no strict V4 handler exists.

## Impact

The change adds contract artifacts and tests under `gateway/providerwire/v4`, pinned capture tooling and fixtures under `test/interop`, contract-validation tasks, and parity coverage metadata. It may add narrowly scoped validation tooling or test dependencies selected by the design, but does not change production request handling, exported provider types, the existing `gateway/providerwire` API, Grafana defaults, model resolution, provider invocation, or frontend wire behavior.
