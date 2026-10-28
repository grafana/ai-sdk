# Proposal

## Why

BYOK callers cannot see which accounts the Gateway actually tried, or why one failed. The runtime already captures each BYOK account attempt in order, but it publishes overviews only for configured catalog routes. So BYOK unary failures, stream setup and preflight failures, and successful responses after an earlier account failed carry no native summary or attempt history ([#420](https://github.com/grafana/ai-sdk/issues/420)). That gap was previously attributed to #317, which is now closed.

## What Changes

- Publish the existing `gateway.execution` overview for BYOK calls through the same carriers as configured calls:
  - unary result metadata;
  - stream finish metadata;
  - top-level `providerMetadata` on unary and setup error documents;
  - `error.data.providerMetadata` on committed stream errors.
- BYOK overviews use honest request identity:
  - `requestedModelId` is the `provider/model` selector.
  - Each attempt's `provider`/`modelId` is the native destination actually invoked, in account-array order.
  - `canonicalModelId` and `providerInstance` are omitted, because no catalog resolution or configured instance exists.
  - Account position is implied only by attempt order. No credential or account identifier is emitted.
- `canonicalModelId` becomes optional in the overview schema. Configured overviews still always carry it.
- Earlier failed accounts keep their compact native summaries (message, type, code, status) when a later account succeeds, when fallback is not eligible, and when every account fails. This includes OpenAI HTTP-200 preflight errors.
- Incomplete, mixed or inconsistent observation omits the overview rather than inventing attempts. Rejections before any account is tried (for example, Anthropic native-option validation) publish no overview.
- Summary protection stays as it is. Exact configured provider/exporter key matches are redacted, and BYOK-only echoes remain.
- Fallback order, eligibility, classification, retryability, cancellation, first-part commitment and no-replay behavior are unchanged. No new collector, stream reader, event or error protocol is added.
- Docs replace the "BYOK calls don't return overviews yet" note.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-execution-overview`: Requested route identity is required, but canonical identity is optional. Request-account overviews carry no canonical or instance identity, and the owned schema reflects that.
- `gateway-attempt-failure-evidence`: Request-local projection and delivery cover BYOK request-account invocations with native destination identity. This replaces the "configured only / capability gap" wording. The evidence requirement adds BYOK both-client scenarios.
- `gateway-request-byok`: BYOK attempts are now published rather than captured-only. The capture-policy scenarios change accordingly.
- `gateway-execution-runtime`: Request-local attempt capture publishes BYOK overviews, as it does for configured calls.
- `providerwire-v4-unary-runtime`: BYOK unary/setup error documents can carry the overview with native summaries. The "#317 missing capability" wording is removed.

## Impact

- `ai-gateway/internal/execution`: selection attribution for request accounts, attempt capture/projection, `Overview` JSON shape (`canonicalModelId` omitempty).
- `ai-gateway/cmd/grafana-ai-gateway/internal/service/selection.go`: the BYOK selector supplies request-account attribution.
- `ai-gateway/providerwire/v4/schema/gateway_execution.json` and its TypeScript schema tests.
- Tests: execution unit and race tests, ProviderWire runtime tests, authenticated command tests through both registered clients (`gateway-command.test.ts`, with per-key fake native failures).
- Docs: `docs/providers/grafana-gateway.md`, `docs/concepts/gateway-glossary.md` and `docs/concepts/gateway-architecture.md` where they describe overviews as configured-only.
- Client-visible: BYOK responses and errors gain optional `providerMetadata.gateway.execution`. A native `gateway` metadata value on BYOK results is now relocated under `gateway.nativeMetadata`, as it already is for configured calls. No client decoder or upstream baseline change is needed, because the Gateway owns `gateway.execution` and both clients treat it as opaque metadata.
