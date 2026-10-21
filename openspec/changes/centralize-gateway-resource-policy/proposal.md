# Proposal

## Why

Gateway resource limits are currently enforced inside the ProviderWire V4 mapper: per-field/string/cardinality preflights, separate unary and SSE-frame byte budgets, a provider stream-part counter, an independent 1 MiB raw-usage cap, and fit-based omission of execution diagnostics. A future adapter (Chat Completions #248, Responses) would have to copy this machinery. A ResponseWriter-side byte budget also cannot prevent allocations that have already happened, and it can terminate a stream after commitment. #394 approves replacing these with a few shared route and transport controls that every adapter inherits. Now that the Gateway runs as an App Platform plugin (#428), the host also owns the outer request bounds. The Go client needs to accept every output the server now produces.

## What Changes

- **Shared route request-body bound.** The existing `max_request_bytes` plugin setting moves from the ProviderWire handler to the plugin's shared route handler, which applies it to every API route and both access populations, independent of adapter and JSON shape. Each adapter maps the overflow to its own fixed invalid-request document. Host-owned bounds are documented and not duplicated: the front door, the plugin route message size and the 60s route deadline.
- **Shared outbound transport owns the upstream bounds.** It retains no-redirect, the fixed 10s response-header timeout and the fixed 16 MiB per-response byte limit, and documents that the limit counts decompressed bytes.
- **BREAKING (ProviderWire V4): adapter byte machinery is removed.**
  - Limits and fixed constants: `RequestBytes` (its setting moves to the route handler), `UnaryResponseBytes`, `StreamParts` and `StreamFrameBytes`.
  - Preflights: `unarySuccessPreflight`, `streamEventPreflight`, and the source/warning/reasoning/identity/custom sizing.
  - Constants and checks: the fixed-frame capacity check, `maxRawUsageBytes`, `maxErrorResponseBytes` and the part-count drain bound.

  Correctness validation is unchanged: UTF-8, registered discriminators, metadata/raw-usage object shape, token ranges and tool/text lifecycle.
- **Encode once, no fit-based omission.** Unary results, finish events and error documents are assembled once, with optional execution metadata and native-summary enrichment, then encoded once. Execution diagnostics and native `gateway` relocation are no longer dropped to fit a local budget. Ordinary JSON encoding failures keep their existing safe errors. JSON is never truncated, and a failed primary output never becomes a success.
- **Deadlines and ownership stay as they are.** Model duration, stream idle and drain durations, cancellation, single cleanup ownership, commitment and no replay after commitment are all kept. Drain becomes bounded by duration only.
- **BYOK account bound becomes execution policy.** The eight-account limit applies to the selected provider's ordered attempts at model construction. It no longer applies to unused provider arrays during decoding.
- **BREAKING (`providers/grafana`): client limits are simplified to match.** `Limits` keeps `DiscoveryBytes` and replaces `UnaryBytes`, `ErrorBytes`, `StreamBytes`, `StreamEventBytes` and `StreamEvents` with one per-response `ResponseBytes` bound, defaulting to 64 MiB. The 1 MiB raw-usage cap is removed. The client keeps an independent memory bound with a documented rationale.
- **Docs and evidence.** Add a limit ledger to `ai-gateway/README.md` that names one owner for each retained limit and lists each removed restriction. Update the specs, client docs, PARITY rows and tests, with both registered clients through the protocol test host.

## Capabilities

### New Capabilities
- `gateway-resource-policy`: shared ownership of Gateway resource limits: the route request-body bound, documented host-owned bounds, outbound transport bounds, execution deadlines, the ledger of retained and removed limits, documented boundary trade-offs, and how future adapters compose with it.

### Modified Capabilities
- `providerwire-v4-unary-runtime`: construction limits, request reading, unary success encoding, metadata/raw-usage validity, error enrichment without fit checks.
- `providerwire-v4-streaming-runtime`: streaming limits reduced to durations; part-count, frame-budget and preflight requirements removed; drain bounded by duration.
- `gateway-provider-metadata`: aggregate metadata budgets replaced by validity-only processing under shared bounds; enrichment no longer fit-based.
- `gateway-sources`: source sizing and preflight removed; validity and lifecycle retained.
- `gateway-custom-content`: custom metadata validity no longer tied to unary or frame budgets.
- `gateway-execution-overview`: summaries no longer take a source/read budget; metadata placement and relocation are unconditional.
- `gateway-attempt-failure-evidence`: optional evidence is delivered without envelope fit-based omission.
- `gateway-unary-function-tools`: tool output bounds reference shared policy instead of the unary budget.
- `gateway-streaming-function-tools`: tool events no longer use part count or frame budgets.
- `gateway-provider-tools`: history accounting and tool metadata no longer reference part or aggregate budgets.
- `gateway-file-inputs`: file input size is governed by the shared route request-body bound.
- `gateway-request-byok`: request bound moves to the route handler; account bound applies only to selected-provider attempts.
- `gateway-provider-configuration`: shared transport bounds described as fixed values under gateway-resource-policy, replacing the removed `anthropic.response-*` flag references.
- `grafana-gateway-client`: client limits simplified to `DiscoveryBytes` plus `ResponseBytes`; per-event, event-count and raw-usage caps removed.

## Impact

- **AGPL `ai-gateway/`:**
  - `providerwire/v4`: handler, response, stream, stream_tools, sources, warnings, reasoning, response_identity, error_response, host_error_writer, execution_capture.
  - `internal/execution`: summary and projection signatures.
  - `cmd/grafana-ai-gateway/internal/{config,gateway,byok,service}`.
  - Tests: `test/platformhost` and the ProviderWire testserver and TS suites.
- **Apache `providers/grafana`:** breaking `Limits` API and decoding limits. Coordinates with #375, which owns the decoder simplification.
- **Operators:** no setting changes; `max_request_bytes` and `max_model_duration` keep their names and meaning. The README limits section gains the ledger.
- **Docs:** `ai-gateway/README.md` (settings table and "Limits"), `docs/providers/grafana-gateway.md`, and the Gateway rows in `test/conformance/PARITY.md`.
- **Delivery:** one PR on top of #428 (`nrbrd/app-platform`), whose migration change is already archived into the main specs.
- **Out of scope:** host admission and capacity (the plugin host), #395 request validation (including MCP option caps), #396 JSON helpers, #116 raw output, and telemetry-sink caps. Existing provider recordings stay untouched, and no Go upgrade is needed.
