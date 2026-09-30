# Local implementation evidence

## Delivered foundation

- Anthropic ordinary supplied function history preserves direct and both code-execution caller variants, including native tool_id conversion, through authenticated pinned TS and independent Go unary/stream requests. This is request-policy evidence, not response-derived continuation.
- Unary/streaming warnings preserve registered fields, order and required empty strings within original/escaped-output bounds. Supplied native response identity survives without canonical fabrication. Both clients overwrite typed unary identity; bounded raw response body retains it.
- Sources preserve native IDs, repetition/cross-variant collisions and URL/document/file-path display. Current numeric source and reasoning metadata codecs are unchanged.
- Trusted configured direct Anthropic/OpenAI/reviewed compatible routes project bounded structured diagnostics using existing handler/PartError paths. Native HTTP status and pinned retry categories survive. Native number/null/string code, type and parameter use existing param/cause/body; GatewayError.Code stays a string. Provider authorization prose is fixed; native transport/cause dumps are excluded.
- Heterogeneous fallback retains the candidate policy union for explicit pre-invocation refusal of consumed active options rather than silently dropping them. Ignored namespaces and empty message namespaces remain eligible; effect guards and first-part commitment are unchanged.

## Regression and review evidence

Caller forwarding and diagnostics were exercised as failing regressions before their implementations. Raw handler schemas/bounds and authenticated command tests are separate from permissive pinned-client parsing. Added provider responses are synthetic unit/integration scenarios, never recorded/upstream fixtures. No registered pins, lockfile, authentic provider inputs or direct conformance expectations changed.

The configured diagnostic seam is server-owned and AGPL-only. Unknown/zero/fallback policies are fixed-safe. Projection neither copies/mutates APICallError nor changes stream channels/readers or core retry/finish authority. Result-plus-error remains precommit with one drain owner; projected-event writer/flush failures cancel without a second write or invocation. Existing setup races, late streams, cancellation/timeout, premature EOF, invalid adaptation and interrupted HTTP success tests were rerun.

The paid-output test distinguishes actual high-level entry points: pinned TS GenerateText uses unary and retries a precommit adaptation 500 three times by default; Go GenerateText collects a committed stream and makes one attempt for a postcommit adaptation error. This is not an exactly-once guarantee.

`TestCallerResponses_MetadataOnlyObservation` captures success/error output separately from logs, labels and AO in unary/streaming internal-team fixture contexts. It confirms contracted warning/source/actual-identity/error/caller markers survive where applicable while telemetry retains canonical route identity and excludes arbitrary markers/native transport. It waits for asynchronous export completion before shutdown; 20 repeats and race tests pass. These are authorized fixture contexts, not BYOK provisioning or deployed tenant-storage evidence.

The restriction inventory now separates real-handler policy forwarding for every current classified field from native consumed-scope tests and output semantics. Compatible native captures verify namespace precedence and call-versus-message/part extension consumption in both modes. Additional producer/continuation/readiness gaps have explicit handoffs, not ignored-field claims.

## Passing checks

- `mise run parity-check`: registered baseline, coverage/provider shape, ProviderWire/schema/typecheck/differential, unchanged direct conformance.
- `mise run test-ai-gateway-source-integration`: candidate-source real handler plus 40 authenticated command tests.
- `mise run test-integration`: schema-parsed frontend source assembly and existing cross-language scenarios.
- `mise run test`: candidate workspace tests for root, Gateway, providers, middleware and examples.
- `mise run build`, `mise run vet`, `mise run lint`: candidate workspaces; no lint issues.
- `mise run lint-docs`, `mise run test-ci-workflow`, `mise run verify-gateway-workspace`.
- `mise run test-ai-gateway-image-source`: actual local-source image build/run and license evidence, not skipped.
- `mise run build-ai-gateway-image`: local image built with honest local-unverified revision metadata.
- Gateway `go test -race ./providerwire/v4 ./cmd/grafana-ai-gateway/internal/service` under go.gateway.work.
- Focused Grafana client error/interrupted-read/cancellation tests, provider diagnostic lifecycle tests and 20 repetitions of caller-response telemetry tests.
- `openspec validate --all --strict --json`: active change and all 80 main specs valid; `git diff --check` and changed Go formatting checks pass.

The repository fmt-check command requires committed changes because it compares the whole worktree with HEAD; it is run after the local implementation commit.

## Not claimed

Published dependencies are not a gate: nara explicitly selected candidate go.work/go.gateway.work source. No pins or releases were changed. No #280 metadata transport/actual-output continuation, #238/#239 provider-tool readiness/runtime, #240 MCP activation or #201 full authentic matrix is claimed. Gateway diagnostic and additional producer follow-up candidates are not registered; GitHub mutation is unauthorized. PR splitting/creation remains outside this local run. The change is not archived and full original refactor acceptance remains open in handoffs.md.
