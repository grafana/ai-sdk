## Why

Configured Gateway fallback rejects function tools/history/choices, files, reasoning content and active options/headers that the strict ProviderWire mapper already supports on direct routes. Issue [#319](https://github.com/grafana/ai-sdk/issues/319) removes that additional Gateway dialect so configured candidates reuse the SDK fallback contract without discarding native semantics.

## What Changes

- Remove the text-only/semantic-emptiness fallback route guard and compose configured candidates directly through reusable `fallback.Model` beneath the existing single logical observation chain.
- Forward the same supported mapped call options to every attempted candidate, retaining original scopes, selected file arms, presence and native namespaces. Do not intersect candidate policies, translate namespaces or silently omit options.
- Preserve configured ordering, native failure eligibility, cancellation and commitment at unary success or the first provider stream part, including stream-start and error. No read-ahead or replay after selection.
- Keep local function execution in consumer applications. Preserve strict mapping, concrete credential/account/protocol/execution protections and existing response/lifecycle bounds.
- Deliver both-client direct/fallback unary/streaming evidence, fake native request assertions, local-tool-loop ownership tests, request isolation and observation checks with revised specs/docs.

## Capabilities

### New Capabilities

None. This expands existing configured fallback eligibility, not the ProviderWire capability union.

### Modified Capabilities

- `gateway-ordered-text-fallback`: Replace text-only and semantic-emptiness restrictions with mapped-capability eligibility, unchanged selection/cleanup and bounded regression evidence. Retain the existing spec identifier to avoid an unrelated rename.
- `gateway-unary-function-tools`: Allow supported definitions, choices and continuation on configured fallback while keeping execution consumer-owned and stateless.
- `gateway-streaming-function-tools`: Allow supported streaming requests and local loops on configured fallback with no replay after the first provider part.
- `gateway-file-inputs`: Remove categorical fallback refusal for supported files/options while retaining validation, presence and bounds.
- `ai-gateway-cloud-authentication`: Distinguish mapped local-tool/file/reasoning fallback evidence from missing provider-executed/MCP codecs and actual Cloud/BYOK support, without changing authentication or credential boundaries.

## Impact

- Gateway service catalog composition and fallback tests under `ai-gateway/cmd/grafana-ai-gateway/internal/service/`; authenticated command and ProviderWire tests under `ai-gateway/test/providerwire-v4/`; centralized Gateway/client guides and the stable parity coverage map.
- Existing SDK `fallback/` tests are regression authority. No reusable API, decider, executor, channel ownership or dependency change is planned. Gateway implementation/tests stay AGPL-owned; reusable SDK code and the independent Grafana client remain Apache-owned.
- Prerequisite: #318 native-option forwarding is already included in `nrbrd/gw-fallback` as `85975180`/`02c59bc2` (ancestry verified). Implement and review on this explicitly stacked prerequisite; no branch restructuring or wait for its merge is needed. While PR [#326](https://github.com/grafana/ai-sdk/pull/326) is open, the child PR targets `nrbrd/opt-forwarding`, merges after the parent, and rebases/retargets to main after the parent merges. Reconfirm parent changes and validate cumulative source; do not claim the parent is already merged. Fresh-from-main intent excludes stopped #303/#309 extraction, not this authorized #318 stack.
- Registered authority: `ai` 7.0.116, `@ai-sdk/gateway` 4.0.94 and `@ai-sdk/provider` 4.0.18 at `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`; reconfirm before implementation. The corresponding pinned source was inspected, not the newer local upstream HEAD.
- Non-goals: new models/order/only routing (#316), BYOK (#317), response metadata/actual output-derived continuation (#280), attempt/failure transport (#322), native diagnostics, discovery, missing provider-tool/approval/raw/custom/generated/structured codecs or tool/MCP activation (#107). No live-backend acceptance, private Vercel-service parity or exactly-once provider guarantee is claimed.
