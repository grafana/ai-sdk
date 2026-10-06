## Why

Grafana AI Gateway currently drops unary warnings and substitutes generic warning prose, sequential source IDs, file-path display and canonical route identity for values already represented by native providers. Issue [#320](https://github.com/grafana/ai-sdk/issues/320) and work package 33 of `~/ai-sdk-ai-gateway-plan.md` require restoring those values without conflating developer responses with metadata-only operator capture.

## What Changes

- Preserve the four registered warning variants and their active fields/order in unary responses and normalized stream starts, including required empty strings; document optional presence that the Go API cannot represent.
- Preserve URL/document source IDs, titles and filenames, including OpenAI/Azure `file_path` display. Remove sequential identity rewriting and its retained map; bound these strings by their containing response/frame rather than the old 1024-byte identity-map cap. Preserve required document titles, optional-empty normalization and unary Title/legacy Text behavior.
- Emit available native `id`, `modelId` and `timestamp` at registered unary `response` and stream `response-metadata` positions. Never fabricate missing identity from the requested/canonical route.
- Preserve independent Go-client stream identity without applying public route-ID validation. Prove the pinned TS and Go clients' unary transport replacement: native unary identity remains in the raw body, not typed `Response` identity fields.
- Extend explicit DTOs, preflight/final bounds, raw HTTP/schema checks, independent-client tests, authenticated command tests and frontend source/identity evidence only for these represented values.
- Keep canonical routing/operator metric identity and metadata-only operator capture unchanged; prove independently configured consumer middleware receives the contracted values.
- **BREAKING**: retire the deliberate source-ID/display and stream-model substitutions; consumers must not rely on `source-N`, `Document`, generic warning text or canonical stream `modelId`.

## Capabilities

### New Capabilities

None; this restores fidelity in existing supported output families.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: include bounded native warnings and registered response identity instead of dropping them; distinguish raw server identity from client-owned transport replacement.
- `providerwire-v4-streaming-runtime`: preserve native warning fields and optional native response identity while retaining start, placement, finish, cancellation and framing rules.
- `gateway-sources`: preserve native IDs/display/order with containing-document bounds; remove `file_path` substitution and classify existing metadata loss as an outstanding implementation gap.
- `grafana-gateway-client`: consume optional native stream identity independently of public route syntax, preserve source/warning values, and explicitly document/prove unary replacement.
- `gateway-text-observability`: distinguish operator capture from verified consumer middleware hooks and caller-visible output without expanding server capture.
- `gateway-ordered-text-fallback`: reconcile normal ProviderWire native-value clauses with configured topology/operator exclusions; leave fallback eligibility, selection, commitment and attempt transport unchanged.
- `providerwire-v4-http-contract`: correct unary consumption evidence to warning combination versus request/response replacement.
- `gateway-provider-configuration`: clarify that existing discovery/error/operator configuration exclusions do not censor registered normal response identity; add no topology/discovery fields.

## Impact

Production work is scoped to `ai-gateway/providerwire/v4/` and any necessary command composition correction under `ai-gateway/cmd/grafana-ai-gateway/`, with independent Apache client changes under `providers/grafana/`. Contract evidence belongs in `ai-gateway/test/providerwire-v4/`; provider-neutral frontend evidence belongs in `test/integration/` and, where appropriate, `test/conformance/ui/`. Applicable Gateway/client docs and `test/conformance/PARITY.md` must stop describing these substitutions as accepted privacy behavior.

Reference: `test/conformance/upstream.yaml` registers `ai` 7.0.116, `@ai-sdk/gateway` 4.0.94, `@ai-sdk/provider` 4.0.18, `@ai-sdk/openai` 4.0.78 and `@ai-sdk/react` 4.0.119 at commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`. No baseline upgrade or dependency issue is required. Start fresh from canonical main; do not reuse stopped #303/#309 implementation patches.

Opaque metadata/continuation (#280), evidence/access design (#321), attempts/failures (#322), native diagnostic bodies/headers (#323), discovery (#324), request-option forwarding (#318), fallback eligibility (#319), routing (#316), BYOK (#317) and missing capability activation are explicitly excluded. Existing metadata projection remains a gap, not an approved concealment policy; this change neither implements a metadata codec nor claims complete native output parity.
