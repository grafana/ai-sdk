## Why

The current Go `provider.LanguageModel` request model collapses valid distinctions emitted by the registered `@ai-sdk/gateway@4.0.52` client: fractional numeric settings, explicit false and empty optional scalars, and empty inline-text file data. Phase 1 established the exact pinned request evidence and executable loss witnesses; the provider contract must now preserve those semantics before a strict ProviderWire V4 mapper can be implemented.

## What Changes

- **BREAKING**: Replace the three narrowed integer settings with a focused exact `LanguageModelNumber` value that preserves historical Go integers and every finite JavaScript-number value without losing legacy bytes.
- **BREAKING**: Make the affected boolean and string request fields presence-aware while keeping source/response filename APIs unchanged.
- **BREAKING**: Make request-side `providerExecuted` presence-aware on tool-call content.
- Expose an exact transport-neutral `DataContent` selection API whose empty inline data and text arms are publicly constructible and inspectable without adding response-visible structural state.
- Update root orchestration, middleware, fallback, and every affected provider implementation to preserve exact supported behavior for the three redesigned numeric settings; their `LanguageModelNumber` values reject NaN and infinities.
- Preserve the deployed tolerant legacy ProviderWire bytes for every request accepted by the parent encoder, including permissive inactive-arm states, while pinning parent-decoder evidence to commit `32e5ab7f1ab9e524477cc0ece04c690a89854a24` only where that decoder succeeded.
- Retain provider custom request JSON methods only as compatibility behavior while moving legacy HTTP request authority to a request-only private adapter; response codecs remain unchanged.
- Resolve the Phase 1 delta lifecycle by replacing loss witnesses with positive provider-contract assertions and keeping `check-providerwire-v4` non-mutating and non-vacuous.
- Keep response-domain APIs and strict ProviderWire V4 runtime work out of this change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `provider-v4-content-model`: Preserve optional prompt-content scalar presence without changing source/response filename or file-data structure, and define the exact public file-data selection API.
- `provider-v4-core-types`: Preserve historical integers, finite registered JavaScript numbers, and optional-scalar presence in `CallOptions` and related request types.
- `provider-wire`: Recast only legacy request transport around a private explicit adapter while preserving parent-pinned bytes and decoder evidence; keep response codecs unchanged.
- `grafana-provider`: Keep hosted-client request transport compatible with the redesigned provider contract and the deployed legacy wire.
- `gateway-providerwire-server`: Preserve handler request semantics and legacy wire behavior while passing redesigned request values to models.
- `providerwire-v4-contract-evidence`: Mark the Phase 2 delta resolved and replace historical loss witnesses with non-vacuous positive provider-contract checks.
- `v4-tool-result-alignment`: Preserve absent versus explicit-empty tool-result file filenames while retaining the existing tagged file-data and stream-result behavior.
- `v4-tool-type-split`: Make function-tool descriptions presence-aware while retaining the flat tool model and compatibility JSON behavior.
- `to-response-messages`: Move generated file filenames into the request-only field when response content is converted into the next provider prompt.
- `server-tools`: Read request-file filename presence from the request-only field while preserving generated citation source filenames.
- `structured-logging-middleware`: Clear both request and response/source filename fields whenever media capture is disabled.
- `agent-observability-middleware`: Read request filenames from the request-only field and generated filenames from the response field when mapping media.
- `openai-responses-provider`: Preserve absent versus explicit-empty tool-result file filenames in custom-tool continuation requests.

## Impact

The source-breaking provider API changes affect the root orchestration package, `provider`, `gateway/providerwire`, fallback and middleware packages, `ToResponseMessages`, server-tool citation tracking, the Anthropic/Vertex Anthropic, Bedrock, OpenAI Responses, OpenAI-compatible, and Grafana provider modules, the ProviderWire V4 evidence workflow, conformance request assertions, examples or docs that construct affected values, and canonical OpenSpec requirements. No package-version upgrade, response-domain/source-filename API change, strict V4 HTTP runtime, response-wire change, or frontend UI-message change is introduced.
