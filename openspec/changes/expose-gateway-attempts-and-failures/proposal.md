## Why

Callers need the selected destination and preceding candidate failures through ordinary Gateway results and errors. The shared observation and compact projector in #373 are ready, but production delivery still uses neither; the historical #370 wiring targets a removed collector and mandatory diagnostic budgets.

## What Changes

- Rebuild #370's runtime integration on #373, retaining the PR and this single OpenSpec change rather than carrying obsolete collector code forward.
- Bridge immutable configured destination identity and actual protected sources from service configuration to invocation handling without exposing credentials through discovery.
- Accumulate one call's shared fallback decisions independently of operator capture. Observe direct calls at existing handler ownership boundaries without turning them into fallback models.
- Publish optional gateway.execution on unary results, finish events and classified failures. Preserve original responses when enrichment cannot fit existing complete-envelope limits.
- Keep current committed failures event-local with protected summaries; do not retain stream-error history, raw diagnostics, completion/replay claims or a selected index.
- Replace historical expectations with actual production invocation, both-client, middleware and frontend ordering proof.

## Capabilities

### New Capabilities

- `gateway-attempt-failure-evidence`: Production delivery of compact execution overviews and useful event-local failures.

### Modified Capabilities

- `gateway-execution-overview`: Allow independent foundation reuse by the runtime activation layer.
- `gateway-provider-metadata`: Explicit best-effort result/finish namespace enrichment exception while preserving primary validation and ordinary opaque transport.
- `gateway-ordered-text-fallback`: Consumer capture independent of physical operator capture, without changed fallback policy.
- `providerwire-v4-unary-runtime`: Optional overview enrichment of results and classified failures.
- `providerwire-v4-streaming-runtime`: Optional finish/error enrichment and event-local protected failures without lifecycle changes.

## Impact

AGPL catalog/service and ProviderWire runtime, internal execution helper reuse, schemas, client test capture helpers, frontend scenarios, docs and PARITY evidence. No client decoder redesign, SDK fallback policy change, dependency/pin or minimum Go change. Builds use the same-revision go.gateway.work. Fuller transport diagnostics (#323), routing (#316), BYOK (#317), discovery (#324), client acceptance (#375) and producer/core fixes (#299) remain separate.

The owner-approved compact contract supersedes historical #321 shape/budgets and #370's earlier validation claims. Reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. This is a Grafana extension, not private-service parity.
