# Service activation evidence

## Prior validation

The evidence below predates this restack. Its blanket MCP rejection claims are
superseded by the reviewed engine and the current validation recorded below.

Validated on this branch after rebasing onto main at `d863b86e` (#326/#328).
All listed gates were rerun on each stack branch; the final command suite passes
60 cases, including main's mapped fallback and high-level default-token coverage:

- `mise run test-short`, `build`, `vet`, `lint`, `parity-check`.
- `mise run test-ai-gateway-source-integration` and `test-integration`,
  including pinned-client typechecks and real-command tests.
- `mise run lint-docs`, `verify-ai-gateway-boundary`,
  `verify-sdk-gateway-isolation`, `verify-merged-pins`, `test-ci-workflow`.
- `mise run test-ai-gateway-image-source` (not skipped) and
  `build-ai-gateway-image`.
- SDK race tests for Grafana, logger, Agent Observability, enrichment and fallback;
  Gateway race tests for command internals and ProviderWire.
- Explicit provider-shape comparison against installed registered
  `@ai-sdk/provider@4.0.18` source; all discriminator families match.
- Strict OpenSpec validation and `git diff --check`.

No module pins, upstream baseline or authentic provider fixture inputs changed.
Local fake services and image tests do not establish live native acceptance,
Vercel hosted-service behavior or deployed Cloud/network authorization.

## Stack preservation

- Original split base: `69bdfa02503c96256c0f23594ad396542de3c374`.
- Rebased stack base: `d863b86e` (main, including #326/#328).
- Backup: `nrbrd/byok-before-stack` (`19cb6a09`), including the public
  test-only TLS certificate/key that were previously hidden by the PEM ignore rule.
- Stack: `nrbrd/byok-client-safety` → `nrbrd/byok-runtime` → `nrbrd/byok`.
- Before rebasing, the final non-OpenSpec tree matched the backup exactly.
  The rebase preserves main's opaque native options and mapped fallback behavior
  rather than restoring deleted policy inventories or text-only wrappers.
- Original requirements are accounted for across the three changes. Caller-owned
  capture guidance moves to grafana-gateway-client; the transport selector bound
  is explicit. The obsolete selected-backend policy delta is dropped in favor of
  main's gateway-native-provider-options contract.
- Client and engine changes are complete. Deployment task 4.1 remains externally
  owned; no environment activation or deployed network proof is claimed.

## Rebase evidence

The operator authorized rebasing before draft PR publication. Conflict resolution
preserves main's native-option/fallback tests, documentation and coverage map,
including unwrapped high-level TypeScript calls and native token defaults.

BYOK Anthropic composition reuses the existing consumption guard for MCP
servers/history, container skills and provider-side fallback. A failing regression
first demonstrated native I/O without that guard; unary/streaming tests now prove
zero attempts for consumed bypasses and retain ordinary-field negative controls.
Configured guard implementations and native converters remain unchanged.

Local gates passed; GitHub CI is separate evidence and is not claimed here.

CI resolved the floating OpenSpec major to 1.14.0, while local mise had 1.6.0.
All three changes also pass strict validation with exact OpenSpec 1.14.0 after
preserving inherited scenario identities, explicitly replacing the obsolete
mode-specific dependency requirement and expressing authenticated-service BYOK
requirements as additions rather than modifications of an unsynchronized spec.
The workflow's archive-before-merge gate remains pending while changes are active.

## Current main restack validation

Restacked in order onto main `ddaa841a` after preserving all three pre-restack
tips under `nrbrd/*-before-restack-20261008`. Only the three activation commits
after `8cae0573` were replayed onto the reviewed and archived engine; obsolete
engine ancestry was not restored.

Service selection now uses `DecodeRequest` and explicit typed-account construction,
without `Selection.Options` or a second native guard. Account-access permission
and unsupported-discovery documents are introduced in this activation slice.
OpenAI organization/project headers survive service selection. No custom
endpoint-approval configuration or deployment changes were added.

Main's additive HTTP/SSE errors, opaque metadata, provider tools and bounded MCP
support remain intact. Focused service tests cover valid MCP configuration/history
and inert markers, while invalid consumed MCP state, skills and native fallback
fail before attempts. Legacy guard expectations failed before reconciliation.
The two Go high-level command cases also failed until deferred provider-option
forwarding was installed in the credential-free client capture harness.

Each stack slice passed `test-short`, `build`, `vet`, `lint`, `parity-check`, source
and frontend integrations, docs, Gateway boundary/SDK isolation, CI workflow and
merged-pin checks, image-source tests and image build, SDK/Gateway race tests and
explicit shape comparison against registered Provider 4.0.18. Source integration
passed 44 runtime tests plus 74 command tests on slices 1/2 and 77 command tests on
slice 3, with no skips. Strict OpenSpec 1.14.0 activation validation and diff checks
passed. Every modified requirement retains its current baseline scenarios,
including text metadata presence/position. Local logs are under
`/tmp/byok-stack-pr{1,2,3}-restack-20261008/`.

No baseline pins or authentic provider inputs changed. Client/capture and engine
archives remain intact. Task 4.1 still requires the external deployment owner,
rendered policy and deployed isolation evidence. This run does not claim live
provider acceptance, hosted Vercel semantics, GitHub CI or environment activation.

## Combined execution stack integration

The owner approved #367 → #368 → #373 → #370 → #369, coordinated with the
execution-stack agent. `gh stack` v0.1.0 tracks branches per worktree. The peer
validated/pushed #373 at `7c492dc9` and #370 at `ae617a13` on repaired #368
`1c2eae83`. Native groups 371/376 were snapshotted and explicitly unstacked as
metadata only; merged #372 remains merged. Four live PRs were regrouped as native
stack 379 before adding this activation slice. No lower branch/status changed.

Adopted the peer's verified tracking into this worktree, recording #369's exact
old boundary `e5b33095` and backup `nrbrd/byok-before-execution-stack-20261008`.
`gh stack rebase --upstack --no-trunk` replayed only the four activation commits.
Conflict resolution retained both PARITY rows and reconciled account-access and
unsupported-discovery errors into #370's typed fixed definitions. No obsolete
engine ancestry, byte-document implementation or execution collector was restored.

Configured selectors return `CatalogSelector`'s private attribution unchanged.
Request-only selections remain catalog-independent and unmarked: they register
no Gateway request collector and publish neither configured overviews nor current
native summaries. Independently registered SDK observers remain effective, and
one logical BYOK log/metric/export still surrounds ordered accounts. Original
native output/metadata remains unchanged; public BYOK attribution stays out of scope.

Six authenticated service cases cover Anthropic/OpenAI unary, stream setup and
post-commit native failures echoing keys and account identifiers. They prove one
attempt, inherited SDK observation, fixed precommit error bytes and no public
execution/nativeError additions or echoed values. Successful/fallback logical
observation cases also assert omission. A temporary mutation enabling current
summaries without configured provenance failed both committed cases with exposed
dummy values; restoration passes races. Mutation never remains in source.
Both-client command rejection tests assert the exact plain error body, including
retained Go/TypeScript error data rather than checking only outer messages.

All full local gates passed on this combined head: source integration 44 runtime
and 77 command tests with no skips, frontend integration, root/Gateway races,
parity/provider-shape, build/vet/lint, docs, boundary/isolation/pins/workflows and
image checks. Focused privacy races passed ten repetitions; no registered pins or
provider input recordings changed. Strict activation validation and inherited
scenario inventory checks retain the current #370 baseline, including typed host
error encoding and configured optional attribution. Logs:
`/tmp/byok-stack-pr3-execution-stack-20261008/`,
`/tmp/byok-execution-stack-privacy-{mutation-red,race,final-green}.log`.
Deployment task 4.1 remains externally gated; no listener redesign or live proof
is included.
