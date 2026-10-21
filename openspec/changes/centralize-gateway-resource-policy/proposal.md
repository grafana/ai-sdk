# Proposal

## Why

The Gateway serves one API contract today, ProviderWire V4, and more are coming: OpenAI Chat Completions (#248), Anthropic Messages (#427) and OpenAI Responses. Today ProviderWire owns not only its wire contract but also:
- the resource limits;
- model selection, deadlines, invocation workers, stream ownership and draining;
- provider-error classification and attempt capture;
- host request security policy (credential-bearing call headers, reserved provider-option namespaces, gateway controls);
- the host's access-refusal and discovery error documents.

#248 shows the cost. It copied the limits, request reading, invocation worker, drain loop and error classification, with weaker guarantees (no panic containment, no attempt capture), and wired itself in by editing the router, process, catalog and telemetry code.

#394 approved moving resource policy to shared boundaries. This change applies the same principle to everything an adapter should not own. The result is one execution runtime and one host composition that every adapter uses, so a new adapter owns only its request mapping, wire encoding, error documents and protocol state machine.

## What Changes

- **Shared route request-body bound.** The existing `max_request_bytes` setting moves from the ProviderWire handler to one wrapper around the plugin route mux. Every current and future route inherits it. Host-owned bounds (front door, 16 MiB route message, 60s route deadline) are documented and not duplicated.
- **Shared outbound transport owns the upstream bounds.** It keeps no-redirect, the fixed 10s response-header timeout and the fixed 16 MiB per-response byte limit, and documents that the limit counts decompressed bytes.
- **Shared execution runtime (`ai-gateway/internal/execution`).** It takes over from ProviderWire:
  - shared execution limits and their validation;
  - access-aware selection under the execution deadline;
  - the unary invocation worker;
  - single-owner stream setup, idle and total waits, and duration-bounded drain;
  - closed failure categories;
  - attempt capture for every selection (configured and BYOK) and execution views;
  - provider output lifecycle validation for streams and unary results, and tool-history correlation, with today's ProviderWire rules.

  ProviderWire is constructed from the shared runtime; it has no consumer outside this module.
- **Host request policy moves into the runtime.** It extracts call-level `gateway` controls for selection and refuses the reserved `grafana`/`grafana-ai-sdk` namespaces and credential-bearing call headers, for every adapter. The upstream service accepts `providerOptions.gateway` on its Chat Completions and Responses APIs too, so controls are a cross-adapter concept.
- **Per-attempt native option translation.** An adapter can supply a translator that maps its API semantics to native options for each physical attempt's provider type. The host wraps every configured candidate and BYOK account once, at construction, and translated options pass host policy. This replaces adapter-specific catalog wrappers such as #248's Chat defaults.
- **Adapter host contract.** Host composition serves every manifest route from one registration table (method, path, label, handler, host-error writer) with the same body bound, access middleware and labels. One access-aware selector replaces the duplicated configured and BYOK handler pairs. Host-produced errors are written by each route's adapter, and catalog listing is host-owned while each adapter projects discovery. Settings carry `execution.Limits` rather than ProviderWire limits.
- **BREAKING (ProviderWire V4): adapter byte machinery is removed.**
  - Limits: `RequestBytes`, `UnaryResponseBytes`, `StreamParts` and `StreamFrameBytes`.
  - Preflights: `unarySuccessPreflight`, `streamEventPreflight`, and source/warning/reasoning/identity/custom sizing.
  - Constants and checks: the fixed-frame capacity check, `maxRawUsageBytes`, `maxErrorResponseBytes` and the part-count drain bound.

  Correctness validation is unchanged: UTF-8, discriminators, metadata/raw-usage object shape, token ranges and lifecycle.
- **Encode once, no fit-based omission.** Unary results, finish events and error documents are encoded once with their optional execution metadata and native summaries. JSON is never truncated, and a failed primary output never becomes a success.
- **Deadlines and ownership are preserved** inside the runtime: cancellation, commitment and no replay after commitment.
- **BYOK account bound becomes execution policy**, applied to the selected provider's attempts only.
- **BREAKING (`providers/grafana`): client limits are simplified to match.** `Limits` becomes `DiscoveryBytes` plus `ResponseBytes` (64 MiB default). The per-event, event-count, error-specific and 1 MiB raw-usage caps are removed.
- **Docs and evidence.** A limit ledger in `ai-gateway/README.md`; updated specs, client docs and tests with both registered clients through the protocol test host; and a test-only non-ProviderWire adapter that proves composition without implementing a real one.

## Capabilities

### New Capabilities
- `gateway-resource-policy`: route request-body bound, host-owned bounds, outbound transport bounds, deadline ownership, the limit ledger and boundary trade-offs.
- `gateway-execution-runtime`: the protocol-independent runtime every adapter uses for host request policy, access-aware selection, per-attempt option translation, shared limits, invocation, stream lifecycle, provider output validation, failure categories and attempt observation.

### Modified Capabilities
- `gateway-plugin-host`: adapter route registration, adapter-formatted host errors and host-owned catalog listing (added requirements; the two-route manifest is unchanged).
- `providerwire-v4-unary-runtime`: construction from the shared runtime, request reading, unary success encoding, metadata/raw-usage validity, error enrichment without fit checks.
- `providerwire-v4-streaming-runtime`: streaming limits reduced to durations; part-count, frame-budget and preflight requirements removed; drain bounded by duration.
- `gateway-provider-metadata`: aggregate metadata budgets replaced by validity-only processing; enrichment no longer fit-based.
- `gateway-sources`: source sizing and preflight removed; validity and lifecycle retained.
- `gateway-custom-content`: custom metadata validity no longer tied to unary or frame budgets.
- `gateway-execution-overview`: summaries no longer take a source/read budget; metadata placement and relocation are unconditional.
- `gateway-attempt-failure-evidence`: optional evidence is delivered without envelope fit-based omission.
- `gateway-unary-function-tools`: tool output bounds reference shared policy instead of the unary budget.
- `gateway-streaming-function-tools`: tool events no longer use part count or frame budgets.
- `gateway-provider-tools`: history accounting and tool metadata no longer reference part or aggregate budgets.
- `gateway-file-inputs`: file input size is governed by the shared route request-body bound.
- `gateway-request-byok`: request bound moves to the route handler; account bound applies only to selected-provider attempts.
- `gateway-provider-configuration`: shared transport bounds described as fixed values, replacing the removed `anthropic.response-*` flag references.
- `grafana-gateway-client`: client limits simplified to `DiscoveryBytes` plus `ResponseBytes`; per-event, event-count and raw-usage caps removed.

## Impact

- **AGPL `ai-gateway/`:**
  - `internal/execution` gains the runtime and host request policy.
  - `providerwire/v4` keeps only its codec: envelope, schema, mapping, DTO encoding, documents and state machine.
  - `cmd/grafana-ai-gateway/internal/{config,gateway,service,discovery,byok,outbound}` adopt the runtime, the registration table and the shared settings.
  - Tests: `test/platformhost`, the ProviderWire testserver and the TS suites.
- **Apache `providers/grafana`:** breaking `Limits` API. Coordinates with #375.
- **Operators:** no setting changes. `max_request_bytes` and `max_model_duration` keep their names.
- **Future adapters:** #248 and the #427 adapter wait for this refactor and rework onto the runtime, translator, validator and registration table. Their needs shaped this design.
- **Delivery:** one PR on top of #428 (`nrbrd/app-platform`), in ordered commits.
- **Out of scope:**
  - adding any new adapter or route;
  - request validation (#395), client decoding (#375), raw output (#116);
  - publishing BYOK attempt overviews (#420; capture is added here);
  - changing the lifecycle rules for sequential native responses (#393; the shared validator is where they will change);
  - native-SDK credential headers (platform-owned);
  - platform route deadline and roles (#460);
  - telemetry-sink caps.

  Existing provider recordings stay untouched.
