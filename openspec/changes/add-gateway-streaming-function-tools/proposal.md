## Why

WP12 (#106) lets clients receive incremental function-tool input and complete multi-step loops through the Gateway. It builds on WP11's unary contract and must deliver both Go and registered Vercel client behavior with the same logical observability.

## What Changes

- Enable WP11's supported tool request subset in streaming mode.
- Add bounded tool-input start/delta/end, tool-call and basic tool-result stream projection with explicit ID/order checks.
- Prove stateless multi-step orchestration in client applications through the production handler.
- Extend existing Go stream decoding and normalized logical observation where needed.
- Preserve cancellation, non-terminal provider errors, final finish/EOF and the effectful fallback prohibition.

## Capabilities

### New Capabilities

- `gateway-streaming-function-tools`: Incremental tool lifecycle, basic result transport, stateless client loops, privacy and acceptance evidence.

### Modified Capabilities

- `providerwire-v4-streaming-runtime`: Supported tool request pipeline, block/finish and unsupported-family rules.

## Impact

Depends on `add-gateway-unary-function-tools` (WP11), which depends on integrated WP7/WP8. Apache work is limited to existing Go client, provider conversion and observation gaps; strict codecs, lifecycle and Gateway contract tests stay AGPL under `ai-gateway/`. WP9 remains text-only. Provider-defined execution/dynamic/preliminary behavior stays WP13, approvals WP14; no server tool runner, session store or production activation is added.
