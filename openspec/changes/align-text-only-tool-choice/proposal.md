## Why

Go's shared text orchestration omits the upstream-default `toolChoice: {type: "auto"}` when no tools are active, masking the Gateway's rejection of ordinary text-only calls from the registered TypeScript SDK. Issue #202 requires correcting both boundaries so Gateway conformance compares equivalent high-level requests rather than accidental Go-only successes.

## What Changes

- Default omitted core tool choice to `auto` independently of the active tool set, preserving explicit and per-step choices across `StreamText`, `GenerateText`, and Agent entry points.
- Retain #186/#187's shared unary/streaming function-tool mapper, including schema-valid `auto` with absent or empty tools and explicit none/required/named choices. Add regression coverage without narrowing direct-route tool support.
- Apply the same narrow exception to the fallback-route text guard; do not alter failover or effect-replay policy.
- Add red-first core call-options, production-handler HTTP, fallback-guard, and equivalent high-level Go/TypeScript streaming regression coverage. Reuse the existing ProviderWire contract/command workspace.
- Require focused regressions, actual high-level cross-client HTTP evidence, and direct conformance to pass without modifying existing expectations or provider recordings. Ship before #201; defer its paired `anthropic/upstream/text-generation` replay and broader matrix checks to #201 integration follow-up.
- Update durable compatibility documentation to distinguish supported text-only high-level streaming from the separate TypeScript `generateText` body-header and default unary-token-limit gaps.

Extending the direct function-tool support already provided by #186/#187, effectful fallback, other Gateway capabilities, upstream upgrades, client-side field stripping, and UI/SSE protocol changes are out of scope. Public signatures do not change; custom models and middleware will now observe an automatic choice instead of nil on text-only high-level calls.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `functional-options`: Specify tools-independent automatic choice preparation and explicit/per-step precedence for shared text orchestration.
- `providerwire-v4-unary-runtime`: Permit and preserve text-only automatic choice while retaining unsupported-family validation; require high-level cross-client request evidence and direct conformance, with paired fixture verification tracked as #201 follow-up.
- `providerwire-v4-streaming-runtime`: Apply the same text-only automatic-choice support and pre-invocation rejection guarantees in streaming mode.
- `gateway-ordered-text-fallback`: Exempt only the harmless no-tools automatic choice from the effect guard, preserving the choice across candidate calls.
- `gateway-unary-function-tools`: Reconcile its fallback prohibition with the text-only automatic-choice exception while preserving direct-route function tools.
- `ai-gateway-cloud-authentication`: Replace the blanket high-level-streaming incompatibility claim with proven text-only streaming support through the existing authenticated composition, retaining unrelated gaps and credential protections.

## Impact

Production changes are limited to `streamtext.go` and `ai-gateway/cmd/grafana-ai-gateway/internal/service/fallback_route.go`; the stack's shared mapper is retained unchanged. Tests span root shared-callers, Gateway handler/service tests, and `ai-gateway/test/providerwire-v4/` with `providers/grafana/internal/capture/` test tooling. The contract workspace reuses #187's exact `ai@7.0.65` dependency and unchanged lockfile; its Go high-level probe uses the changed local core rather than the capture helper's existing published dependency.

The registered baseline remains commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` (`ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, provider `4.0.7`). No production dependency upgrade, protocol-schema relaxation, serializer workaround, or provider-fixture regeneration is planned. Update `test/conformance/PARITY.md` to record the new request-boundary evidence and retained limitations. Paired fixture validation remains pending follow-up when #201 integrates this fix and its harness runs on Linux/local Docker. It is not a completion or shipping gate for this change; no replacement harness or unexecuted matrix result is claimed.
