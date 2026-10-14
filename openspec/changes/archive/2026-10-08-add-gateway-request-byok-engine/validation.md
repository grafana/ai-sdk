# Engine evidence

## Scope and reference

PR #368 owns the request-only engine and host-selection extension, independently
of PR #369's authentication/admission and service activation. This slice remains
catalog-only: command/config validation use `CatalogSelector`, not `NewBYOKSelector`.

Registered reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18, Anthropic 4.0.65
and OpenAI 4.0.78 at `5d12eaa6caa193d3901cbab98a734403eb6bf622`.
Gateway projection and its generic account-record shape establish client behavior,
not Vercel hosted-service decoding, destination approval or retry semantics.
Standard Go decoding is an intentional service adaptation. No baseline pins,
module pins or authentic recorded/upstream provider inputs changed.
Synthetic HTTP, local TLS proxy, compile and image checks do not prove live native
acceptance, private Vercel behavior or deployed Cloud/DNS/proxy/network isolation.

## Current contract

- `DecodeRequest` uses nested control/provider maps, plain `nativemodel.Config`,
  standard Go JSON decoding and `DisallowUnknownFields` for account structs.
  Exactly lowercase `byok` is required; account fields follow canonical client
  names without compatibility promises for incidental decoder case matching.
  Null/duplicate members follow Go semantics, including malformed earlier values.
- Every supplied provider array has 1–8 accounts, each with a nonempty key;
  the selected provider must be present. Nonempty organization/project are
  OpenAI-only. Native defaults remain explicit and ambient accounts are excluded.
- Custom destinations require exact provider-scoped host approval. Approvals
  grant permission only, not endpoints/accounts/defaults. Requests cannot supply
  approval policy, retries, arbitrary account headers, transport/TLS or deadlines.
  Approval syntax validation is host-owned; engine requests check membership.
  No operator approval configuration is implemented in this slice.
- Removed schemas, duplicate normalization/JSON round trips, opaque validated
  requests and engine-specific key/map/selector byte or header/URL grammar.
  Whole ProviderWire request limits apply before selection (command default 1 MiB);
  real native HTTP transport validates outgoing headers.
- Explicit native constructors have no catalog/configuration/secret-resolver
  dependency. Ordered accounts reuse default fallback, zero native retries and
  redirect refusal without mutating clients. Content and account isolation remain.
- Shared Anthropic consumption guards run outside credential fallback. Valid
  bounded MCP configuration/history and inert local markers survive; invalid
  consumed state, skills and native fallback fail before attempts.
- Selection returns model/identity only; mapped inference options stay with the
  handler. Selection and invocation share a deadline, with panic containment,
  cancellation and late-stream cleanup. Account-access/discovery errors are
  activation-owned. Fixed validation errors never echo private diagnostics.

## Validation gates

Each milestone marked full-gates below reran this inventory on its own slice:

- `mise run test-short`, `build`, `vet`, `lint`, `parity-check`.
- `test-ai-gateway-source-integration` and `test-integration`, including pinned
  client typechecks and real-command tests.
- `lint-docs`, `verify-ai-gateway-boundary`, `verify-sdk-gateway-isolation`,
  `verify-merged-pins`, `test-ci-workflow`.
- `test-ai-gateway-image-source` without skips and `build-ai-gateway-image`.
- SDK races: Grafana, logger, Agent Observability, enrichment and fallback;
  Gateway races: command internals and ProviderWire.
- Explicit shape comparison against installed registered Provider 4.0.18 source;
  all discriminator families match.
- Strict OpenSpec 1.14.0 validation and `git diff --check`, scoped to this change
  or its synchronized specs/archive after archival.

## Milestones and attribution

Historical statements about an active engine change or untouched PR #369 were
true at those revisions, not its current archive/restack status. Original detail
remains in Git history and the unabridged validation at `b6e9015c`.

| Milestone | Outcome and retained evidence |
| --- | --- |
| Main `d863b86e`, reviewed client `58487635` | Full gates passed after #326/#328 and client capture/spec cleanup; range-diff retained the three engine commits. |
| Test consolidation `d28bf2cd` | BYOK tests moved into model_test.go and selection tests into handler_test.go without changing bodies; focused races, vet/lint, parity and source integration passed. |
| Shared-guard cleanup `aab792b8` | Full gates passed, 57 command cases; removed redundant unary cancellation checks and `Selection.Options`, deferred activation errors. The initial blanket MCP guard is superseded below. |
| Native-contract rebase `f6520097` on client `6f00c0e8` / main `8d36b82d` | Full gates passed, 74 command tests; retained opaque metadata, provider-tool history and valid MCP. Guard implementation matched main except package/export names; private MCP bounds tests moved unchanged. |
| Typed construction `c0a4e18a` | Full gates passed, 74 command tests; shared explicit native constructors, missing-transport and credential-ownership proof. Opaque request/key-only decoding is superseded. |
| Account configuration `220aa05d` | Full gates passed, 74 command tests; account defaults, exact approvals, OpenAI headers and client/capture witnesses. Schema/normalization/byte/header/URL policies are superseded. |
| Plain decoder `5f7ae150` | Full gates passed, 74 command tests, no skips; final map/typed-account decoder and canonical casing. Logs: `/tmp/byok-stack-pr2-simple-decoder/`. |
| Task-oriented guide `b774043a` | Four Go examples compile; two TypeScript examples strictly typecheck against registered ai/Gateway, including configured-discovery. Sources/build: `/tmp/byok-doc-examples.Bewb9i/`. Docs lint, baseline (109 tests) and strict change validation passed; no provider calls. |
| Fixed diagnostics `00a75a23` | Uncached ProviderWire/BYOK/nativeoptions/service races, lint/vet, parity, source integration (74 command tests, no skips), docs, strict change validation and diff checks passed. Two-round review closed; evidence below. |
| Spec sync/archive `4846b23f` | Seven BYOK, three unary and two streaming requirements synchronized; all 18 tasks complete. Scoped strict archive/spec checks, baseline (109 tests), docs and diff checks passed. |
| Restack `7fb49896` on client `2540ab4c` / main `ddaa841a` | Full gates passed, 74 command tests, no skips. Range-diff changed only inherited parity context/guide reconciliation; main's additive HTTP/SSE diagnostic guidance retained. Logs: `/tmp/byok-stack-pr2-restack-20261008/`; diff: `/tmp/byok-pr2-restack-range-diff.txt`. |
| Terminal-log wait `b6e9015c` | Full gates and uncached short tests passed; targeted race stress and GitHub CI passed. Evidence below. |

## Distinct regression evidence

- Account defaults first failed for explicit native `baseURL`, organization/project
  and empty optional values. Green unary/streaming witnesses retain native headers,
  inference/body separation, ordered per-account endpoint changes, zero retries,
  environment poisoning and concurrent account isolation. Native/default and
  approved endpoint redirects are refused without changing caller clients.
- Policy witnesses validate unused accounts, exact provider-scoped approvals,
  absent/wrong-provider approvals, path/port/prefix mismatches, unknown controls,
  required keys and counts. Historical byte/grammar tests are superseded by tests
  accepting values above deleted limits and preserving ordinary Go null/duplicate
  behavior. Actual TLS transports reject invalid key/organization/project headers
  before server calls; mocked transport assertions are not a substitute.
- Decoder red/green/race logs: `/tmp/byok-decoder-{red,green,race}.log`.
  Separate canonical-control reds showed the initial struct envelope accepting
  uppercase `BYOK`; the map envelope rejects it alone and beside lowercase `byok`
  without changing capture code. Logs: `/tmp/byok-canonical-control-{red,race}.log`.
  Final casing-aligned races/docs: `/tmp/byok-decoder-final-{race,docs}.log`;
  strict change validation also passed. Wire witnesses prove the complete request
  boundary and zero selection/invocation above it.
- MCP witnesses preserve valid configuration/provider-owned history and inert
  markers, reject invalid consumed destinations/history, skills and native fallback
  before attempts, and retain ordinary namespaces and inference host/model/key.
- Registered Gateway 4.0.96 and independent Go captures preserve extended account
  fields/order and caller metadata. Logger sinks redact the entire subtree across
  generate/stream/error/truncation. OpenAI native names/defaults match 4.0.78;
  these are client/native construction witnesses, not hosted-service policy proof.
- Fixed diagnostics: three wrapped selection sentinels across unary/streaming;
  old account/selector wording failed four cases before correction. Green cases
  assert HTTP 400, exact fixed JSON, no private key/account/URL text and zero model
  calls, even when selection also returns a model. Status/type/code/envelope are
  unchanged. Registered Gateway accepts arbitrary messages and classifies the
  unchanged `invalid_request_error`; no private-service message parity is claimed.
  Logs: `/tmp/byok-review-diagnostics-{red,race}.log` and
  `/tmp/byok-review-loop-{lint,parity,source-integration,vet,docs,openspec}.log`.

## Review closure

Initial oracle plus independent correctness/simplicity/validation reviews found
no execution/design/evidence defects; the oracle's stale diagnostic wording was
fixed above. Two fresh follow-up reviewers found no further actionable issues.
The parent confirmed the correction and closed after two rounds; no material
product/API/architecture decision required another oracle.
Reports are retained in managed outputs for workflows `f1708fd5` and `2ad9c708`.
The first report-serialization failure was corrected by same-protocol retry,
reusing completed reports without changing source or discarding evidence.

## Archive integrity and existing findings

Owner-approved sync preserved inherited scenarios, untouched requirements/order
and main-spec preambles. At archival, `.openspec.yaml` and all artifacts were
byte-identical except appended validation evidence; this later owner-approved
consolidation changes only validation narrative, retaining history at `b6e9015c`.
The archive is `2026-10-08-add-gateway-request-byok-engine`; no active engine change
remains. Service activation and operator/deployment evidence stay with PR #369.

Whole-repository strict validation has existing Purpose findings in
`gateway-reasoning-content` and `provider-v4-core-types`. Bulk archive auditing
has two incomplete tasks in each of `2026-05-14-add-sigil-middleware`,
`2026-07-27-complete-agento11y-rename` and `2026-09-24-gateway-reasoning-content`.
Those five files match the PR base/integrated main; scoped engine checks pass.
Archive logs: `/tmp/byok-engine-archive-{specs,tasks}.json` and
`/tmp/byok-engine-archive-{baseline,docs}.log`.

## CI terminal-log synchronization

GitHub run 37796054754 observed zero terminal log events after generation export
in `TestFallbackAcceptance_MetadataFailureDoesNotReplay`. The helper waits up to
its existing one-second bound for exactly one terminal log independently of
export; production behavior, observer ordering and wire contracts are unchanged.
The target passed 900 race-enabled repetitions across CPU settings 1/2/8 and all
fallback acceptance tests passed ten race-enabled repetitions. Full-gate logs:
`/tmp/byok-stack-pr2-ci-log-wait-20261008/`. GitHub checks passed at `b6e9015c`,
including Build & Test run 37798724850.

## Simplification verification

The owner approved consolidating validation history and extracting two small BYOK
test helpers after review `5e59a1a9`. Construction/dispatch alone move into helpers;
all cases/assertions, ownership mutations, concurrent error handling, real HTTP
rejection and stream commitment/cleanup witnesses remain. No production, guide,
spec requirements/scenarios, task inventory, baseline pins or provider inputs change.
Before/after execution retained all 145 test/subtest names and every non-setup
assertion verbatim/in order; ten BYOK race repetitions and uncached short tests
passed. Full gates and strict checks of the three synchronized specs passed;
archive task/evidence inventories and scoped validity are preserved. Logs:
`/tmp/byok-stack-pr2-simplification-20261008/` and `/tmp/pr368-simplification-*`.

## Latest-main rebase

Rebased onto the client slice on `origin/main` at `79be97b0`. Runtime range-diff
retains all behavior; main's stronger fallback-test wait for both terminal logging
and metrics replaces the earlier log-only wait. Full gates above, standalone
readonly Go 1.26.8 Grafana races and 50 fallback metadata-failure race repetitions
passed; 74 command cases ran without skips. Logs:
`/tmp/byok-stack-pr2-latest-main-20261009/`. Main's Azure/security changes and
registered baseline remain intact; generated workspace checksum churn is excluded.

## Approved account decoding revision

Removed DisallowUnknownFields and used a minimal wire-only account wrapper to
identify meaningful unsupported modelMappings; native Config/construction remain
unchanged. Additive/inactive-mapping cases failed before the change. Green tests
retain case-sensitive namespace/provider/discriminator keys, standard account
field matching/duplicates, nil/empty mappings, unsupported/malformed mapping
errors and unchanged explicit native keys/endpoints/headers. Ignored retry/header
fields never alter native construction. The existing eight-account behavior is
retained as temporary execution policy, not a decoder grammar. Ten engine and
SDK capture race repetitions passed. Full gates and current strict specs passed
under exact `eb77f09e`/Gateway4.0.103; 74 command cases ran without skips. One
unchanged React metadata-history test initially missed an asynchronous snapshot;
three isolated runs and the full rerun passed, with no UI source changes. Logs:
`/tmp/byok-stack-pr2-approved-audit-20261009/`, `/tmp/byok-audit-pr368-gates.log`,
`/tmp/byok-audit-pr368-gates-rerun.log` and `/tmp/byok-audit-react-metadata-{1,2,3}.log`.
Native/content/fallback/isolation/redirect/late-owner behavior, registered pins and
provider input recordings are unchanged.
