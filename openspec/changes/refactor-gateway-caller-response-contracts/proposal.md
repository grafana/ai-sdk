## Why

Issue #303 corrects Gateway contracts that conflate useful authorized provider responses with telemetry and protection of shared Grafana accounts. Customer-owned provider relationships do not relax credential/tenant/execute/resource guards or implement BYOK provisioning. The foundation must precede—not depend on—metadata and provider-tool work.

## What Changes

- Deliver **#303 → #280 → #238 → #239 → #240** in that order. #303 is independently green/completable from current main; handoffs.md explicitly preserves broader refactor acceptance for successors instead of requiring future output transport or #201's undelivered matrix.
- Classify concrete request/response restrictions now in restriction_inventory.md: keep protections with present reasons; remove obsolete concealment and supported consumed-option loss; defer capabilities with explicit owners/boundaries.
- Preserve unary/stream warnings, native source IDs/display and actual registered response identity. Keep current source-metadata transport unchanged until #280; document the pinned typed unary identity overwrite/raw-body boundary.
- Fix the narrow Anthropic caller request policy for already-supported assistant function history. Both clients' supplied/built history must retain pinned consumed caller semantics in fake native Anthropic requests without needing a prior response, provider execution or future helper. #280 later proves actual response-derived continuation.
- #303 owns minimal reviewed actionable provider errors within existing handler/stream lifetimes and public error API. Define diagnostic/security/status/retry behavior before choosing mechanisms; no prescribed sentinel error tree, new stream reader or breaking GatewayError.Code type. Required invasive work needs a separately registered Gateway follow-up coordinated with #299.
- **BREAKING behavior** for consumers relying on source-N IDs/display censorship, fixed warning prose, canonical actual model substitution or fixed provider-error status/message. No new compatibility dialect or public error type redesign.
- Keep Apache client independent from AGPL server and preserve candidate-source/image versus published-module validation. Sync/archive only behavior actually delivered by each owner.

## Capabilities

### New Capabilities
- `gateway-caller-response-policy`: foundation/target boundary, restriction dispositions, bounded minimal provider diagnostics, debugging and response/telemetry separation.

### Modified Capabilities
- `providerwire-v4-unary-runtime`: request-policy/caller prerequisite, warnings, source display/identity, registered response identity and minimal provider-versus-internal failure mapping.
- `providerwire-v4-streaming-runtime`: warning/actual identity values and minimal existing-path error projection, with unchanged metadata transport/lifecycle.
- `grafana-gateway-client`: warning/identity/source consumption and minimal error envelope/cause-body mapping without public API redesign.
- `gateway-sources`: native identity/display; metadata transport explicitly unchanged pending #280.
- `gateway-unary-function-tools`: supplied assistant function history/caller request evidence and unchanged execution ownership.
- `providerwire-v4-http-contract`: exact-pinned warning/unary identity/minimal error consumption evidence.
- `gateway-ordered-text-fallback`: remove caller identity concealment, refuse rather than silently lose consumed unrouteable options, retain effect/commitment/telemetry guards and safe unresolved diagnostics.
- `gateway-provider-configuration`: distinguish private configuration from caller data, trusted minimal direct-route diagnostic policy and narrow Anthropic caller prerequisite.

Metadata-only reasoning/function-tool/source/client transport deltas are not foundation requirements: #280 owns them and their continuation proof, recorded in handoffs.md.

## Impact

Server: `ai-gateway/providerwire/v4/{request,provider_option_policy,response,stream,errors,sources}.go`, response schemas and command provider-option/route policy. Client: existing private Apache codecs, never AGPL DTO imports or a changed public error API. Tests: exact-pinned TS/Go handler/command/native fake requests, independent guards/telemetry/lifecycle, unchanged direct conformance; full authentic matrix later #201. Central docs and stable parity evidence update only for delivered support.

Baseline stays `4e8c387622ee1bb0d55841664416d38754d5c9a3` (ai 7.0.109, Gateway 4.0.88, Anthropic 4.0.59, provider 4.0.17). #299's newer core-error target is separate. This PLAN neither implements nor proves acceptance of BYOK, metadata roundtrip, new tools/MCP or the full original refactor. Nara's approved minimal-error ownership is explicit; invasive Gateway diagnostic follow-up registration remains a later action, not a GitHub mutation by this run.
