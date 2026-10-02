## Why

Gateway developer evidence currently loses meaning or accessibility when native transport is replaced, errors are normalized, or discovery fields are discarded. Issue [#321](https://github.com/grafana/ai-sdk/issues/321) and work package 34 of `~/ai-sdk-ai-gateway-plan.md` require one reviewed access contract before #322–#324 implement attempts, native debugging, and configured discovery.

## What Changes

- Add small deterministic injected-response probes against the registered TypeScript packages and independent Go client, covering unary success, stream setup/success/committed errors, exhausted-candidate envelopes, discovery, relevant high-level normalization, and consumer middleware access.
- Produce one compact decision recording concrete field placement and access expressions, requested/canonical/selected/provider-reported identities, selection versus completion, native versus Gateway retry/status facts, and opaque-metadata collision/provenance rules.
- Decide source-specific credential exclusions, numeric input/retention/encoded/count budgets, and absent/unavailable/redacted/malformed/over-limit behavior without trimming ordinary supported content or metadata.
- Record an acyclic ownership map for #280, #322, #323, #324 and #316, and obtain explicit owner approval of any minimal extension/public API before downstream implementation.
- Keep this delivery design/probes only. Existing production wire behavior, SDK error types, stream lifecycle, discovery APIs, routing, BYOK and capture configuration remain unchanged.

## Capabilities

### New Capabilities

- `gateway-developer-evidence-decision`: Exact-client evidence and approval requirements for the Gateway developer-evidence/discovery contract and downstream seam ownership.

### Modified Capabilities

None. Existing client/catalog/protocol requirements are inspected as the implementation baseline, not rewritten to claim delivery of #322–#324. Their production changes and superseded concealment requirements belong to the downstream feature changes.

## Impact

- Planning artifacts and a final reviewed decision under this change; focused TS probes in `ai-gateway/test/providerwire-v4`, with independent Go fake-HTTP probes in `providers/grafana` and only necessary high-level/middleware test coverage.
- Reference: canonical main/worktree `c0299776`; registered upstream commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`, `ai@7.0.116`, `@ai-sdk/gateway@4.0.94`, `@ai-sdk/provider@4.0.18`, `@ai-sdk/provider-utils@5.0.49`. Reconfirm before executing probes; no baseline upgrade is proposed.
- Provider-contract, client normalization, middleware and conformance-harness evidence are relevant layers. Synthetic injected responses are access witnesses, not provider recordings or proof of Vercel's private service.
- No new runtime dependencies, server/client codec changes, exported APIs, metadata mapper, collector framework, extra stream reader, credential store, or resumed #303/#309 implementation. Known BYOK capture handling remains #317; producer/core error semantics remain #299.
