## Why

Issue #108 / work package 14 requires clients to approve or deny tools and continue through the Gateway. Canonical main has function/provider tools, opaque metadata and mapped fallback, but rejects approval requests/results and loses the distinction between an absent and empty execution-denied reason.

## What Changes

- Support registered provider approval requests in unary and streaming responses and tool-role approval responses in requests, preserving required false decisions.
- Preserve approval correlation, provider/client execution ownership, opaque continuation metadata and nested tool-result options through actual subsequent native requests.
- Make execution-denied reason presence representable while retaining string normalization for approval-response reasons.
- Prove approved, denied and mixed continuation using the registered Vercel client and independent Go Gateway client, including configured fallback and independent capture.
- Update capability docs and evidence without extending routing, BYOK, diagnostics, or general custom-content support.

## Capabilities

### New Capabilities

- `gateway-tool-approvals`: Registered approval transport, lifecycle, continuation and both-client acceptance.

### Modified Capabilities

- `gateway-provider-tools`: Enable nested output/content options currently explicitly deferred.
- `v4-tool-result-alignment`: Presence-aware execution-denied reasons.
- `grafana-gateway-client`: Consume provider approval output and preserve continuation inputs.

## Impact

AGPL Gateway mapping/encoding/tests/docs stay under `ai-gateway/`; Apache provider domain, native conversion, core continuation and independent Grafana client stay outside. No reverse module dependency or published artifacts. Baseline is `ai` 7.0.127, gateway 4.0.103, provider 4.0.21 at upstream commit `eb77f09e3c06c28e860d92e0de941b143c2eecec`; previous issue version literals are historical. Existing #280/#318/#319 foundations on canonical main are consumed rather than reimplemented. No stopped #303/#309 code is used.
