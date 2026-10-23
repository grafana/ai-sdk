# Proposal

## Why

Every resource limit should have one owner, at a boundary that every adapter shares. Today ProviderWire V4 owns the request-body bound and a set of output budgets: an 8 MiB unary response, a 1 MiB SSE frame, 100,000 stream parts, a 1 MiB raw-usage cap and a 64 KiB error document. A new adapter would have to copy all of them (#394).

The output budgets also do not protect anything that a shared boundary does not already protect:
- the outbound transport bounds each provider response at 16 MiB;
- the model, idle and drain durations and the 60s platform route deadline bound time;
- the plugin host owns admission and concurrency (#104).

Instead, the budgets reject valid output that the provider already delivered. Optional execution metadata is also dropped whenever it does not fit.

The Go client removes its own limits in #479, so after this change the server and both registered clients accept the same outputs.

## What Changes

- **Route request-body bound.** `max_request_bytes` keeps its name and 1 MiB default. The Gateway applies it once around the plugin route mux, so every current and future route inherits it. A refused caller's body is never read. ProviderWire reads the body to EOF and maps an overflow to the existing fixed invalid-request document. The ProviderWire testserver handlers sit behind the same bound.
- **Outbound transport.** Rename `NewAnthropicClient` to `NewModelClient`, since it serves every provider. The fixed 16 MiB response bound and 10s header timeout stay. Tests pin that the bound counts decompressed bytes and applies to each request separately.
- **BYOK account bound.** The eight-account bound applies only to the selected provider's accounts. Unused provider entries still have to be well-formed. A request over the bound still gets the fixed BYOK credentials 400, with zero native calls.
- **BREAKING (Go API)**: remove the ProviderWire size budgets:
  - `Limits.RequestBytes`, `UnaryResponseBytes`, `StreamFrameBytes` and `StreamParts`;
  - the stream part counter and the canonical-frame construction check;
  - every output preflight: unary success, stream events, warnings, sources, reasoning and generated files, response identity, tool events and custom parts;
  - `maxRawUsageBytes` and `maxErrorResponseBytes`;
  - `sourceBytes` in execution summaries and overviews.

  Drain is then bounded by the drain duration alone. No operator setting changes.
- **Keep validity checks:** UTF-8, discriminators, metadata namespace and raw-usage object shape, token ranges, timestamps, tool and text lifecycles.
- **Encode once.**
  - Unary results, finish frames, and HTTP and SSE errors are encoded once, including their optional execution metadata and native summaries.
  - An error falls back to its fixed base document only if that encoding fails.
  - Execution metadata always relocates a native `gateway` value.
  - JSON is never truncated, and a failed primary output never becomes a success.
- **Limit ledger.** The "Limits" section of `ai-gateway/README.md` lists:
  - one owner for each retained limit;
  - each removed limit and what replaces it;
  - the boundary trade-offs from #394, including memory growth from encoding expansion and concurrency.

## Capabilities

### New Capabilities

- `gateway-resource-policy`: which boundary owns each Gateway resource limit (route request body, outbound transport, execution durations, plugin host), and the guarantee that adapters neither size-limit nor truncate output.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: construction takes only durations. The body read is host-bounded. Unary preflight, final byte bounds and the raw-usage cap are removed, and errors and success encode once.
- `providerwire-v4-streaming-runtime`: removes the part count, frame bound, start/identity/raw-usage preflights and the frame construction check. Drain is duration-bounded, and finish and errors encode once.
- `gateway-provider-metadata`: removes the shared metadata budgets and aggregate preflight, and keeps the shape and UTF-8 validation.
- `gateway-sources`: removes the aggregate source budgets, and keeps source validity.
- `gateway-custom-content`: invalid custom output is defined by kind and metadata validity, not by size.
- `gateway-execution-overview`: summaries have no size-based degradation, and relocation of the native `gateway` value is unconditional.
- `gateway-attempt-failure-evidence`: enrichment is encoded with the primary output and omitted only when encoding fails.
- `gateway-provider-tools`, `gateway-streaming-function-tools`, `gateway-unary-function-tools`: tool output and state are no longer bounded by the part count or the response and frame budgets.
- `gateway-file-inputs`: file request bytes are bounded by the route request-body bound.
- `gateway-request-byok`: the account bound applies to the selected provider only, and the request bound is route-owned.
- `gateway-provider-configuration`: the shared model transport is described by its fixed bounds, not by the retired `anthropic.*` flags.
- `gateway-generated-media`, `gateway-raw-output`, `gateway-structured-output`: these specs also depend on the removed budgets, so their size-failure scenarios are replaced.

## Impact

- **Code:**
  - `ai-gateway/providerwire/v4`: `handler.go`, `response.go`, `stream.go`, `stream_tools.go`, `sources.go`, `warnings.go`, `response_identity.go`, `reasoning.go`, `error_response.go`, `host_error_writer.go`, `execution_capture.go`, `doc.go`;
  - `ai-gateway/internal/execution`;
  - `cmd/grafana-ai-gateway/internal`: `config`, `gateway`, `outbound`, `byok`, `service`;
  - `ai-gateway/test/providerwire-v4/testserver`;
  - the affected Go and TS tests.
- **Public Go API:**
  - `providerwire/v4.Limits` keeps only `ModelDuration`, `StreamIdleDuration` and `StreamDrainDuration`;
  - `execution.Summary` and `execution.Project` lose `sourceBytes`.

  Both are breaking changes, and the PR title carries `!`. There is no consumer outside the `ai-gateway` module.
- **Operators:** no setting changes. Outputs that previously failed with an internal error now succeed.
- **Wire:** output that used to be within the limits is unchanged. A native `gateway` namespace is now always relocated under `gateway.nativeMetadata` when an overview is attached.
- **Docs:**
  - `ai-gateway/README.md` Limits ledger;
  - `docs/providers/grafana-gateway.md`: raw events, generated files, overview availability, and the server-owned bounds paragraph;
  - `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md` where they describe handler bounds;
  - the ProviderWire package godoc.
- **Parity:** Gateway resource policy is a Go extension with no upstream counterpart (`@ai-sdk/gateway@4.0.103` defines only the client). `knownGaps`, recorded provider fixtures and conformance expectations are unchanged.
- **Depends on #479** (client limits removed first). Base branch: `nrbrd/gateway-client-limits`.
- **Out of scope:** runtime extraction (#481), route table and host composition (#482), lifecycle rules (#393), admission and concurrency (#104), request structural validation (#395), telemetry sink caps, catalog and discovery ceilings, and the MCP option caps.
