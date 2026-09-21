## 1. Confirm scope and evidence before implementation

- [x] 1.1 Reconfirm registered and fixed-target package manifests/tags from `design.md`; review matching implementations and tests and record the initial PR delta ledger with source, current Go status, classification, regression proof, snapshot impact, and owner. Validation/provenance summary: `evidence.md`; detailed temporary evidence is preserved externally.
- [x] 1.2 Incorporate Nara's approved nine-correction scope and validate updated artifacts. Unrelated modelFamily, explicit output-mode, Anthropic updates/binding/service-tier, clearAt/effort, and guard-content decisions remain planner-owned PENDING, not accepted deferrals or PR 7 work; they do not block these nine absent a concrete dependency.
- [x] 1.3 Inventory authentic fixture inputs and affected request goldens for the nine approved corrections. Select conformance scenarios where provenance permits; identify focused unit/HTTP tests and explicit boundary gaps elsewhere. Do not modify recorded provider inputs or invent upstream fixtures.
- [x] 1.4 Establish a disposable integration worktree with the plan's exact coherent target package set and preserve temporary candidate evidence outside the repository. Capture current-baseline check status separately; do not alter canonical pins, lockfile, or baseline provenance in this PR.

## 2. Correct Vertex model selection

- [x] 2.1 Add failing capability and generate/stream request regressions for dated Sonnet 4 and Opus 4: default token limits, known-model warnings, explicit limits, thinking-budget clamping, sampling behavior, and unchanged more-specific/legacy/future controls. Replace existing assertions that encode the dated-ID bug.
- [x] 2.2 Recognize dated Vertex IDs in the existing capability lookup while preserving transport model IDs and specificity order; run the focused Anthropic tests and exact-target request comparison for selected scenarios.

## 3. Correct OpenAI allowed-tool request selection

- [x] 3.1 Add failing exact-request tests for function/custom names, all supported built-in entry shapes, MCP server labels, canonical aliases, mode override, duplicate order, and full declaration retention.
- [x] 3.2 Add failing selection-policy tests for direct-name collisions, equivalent versus ambiguous aliases, unknown names, deferred/namespaced/tool-search selections, explicitly empty and fully dropped lists, and the no-tools early return; assert warning details and no HTTP call on local rejection.
- [x] 3.3 Resolve allow-list entries from prepared declarations and propagate conversion warnings/errors through both generate and stream paths without changing ordinary named choices or response/history conversion; rerun focused tests and target comparisons.

## 4. Correct Bedrock document names

- [x] 4.1 Add failing request tests for invalid punctuation, non-ASCII and Unicode whitespace, repeated whitespace, extension-only/empty names, 200-character boundaries, input bytes/text and tool-result documents, request-wide fallback numbering, and unchanged valid names/content/citations.
- [x] 4.2 Apply target-ordered sanitation in the shared document builder with post-sanitation fallback; run focused Bedrock tests and target request comparisons at both document sites.

## 5. Gate Bedrock strict mode by schema compatibility

- [x] 5.1 Add failing request/warning tests for top-level and nested open objects, object union types, $defs/definitions, schema dependencies, pattern properties, array/tuple items, conditionals and composition branches; include closed, boolean, false/absent strict, unsupported-model precedence, and non-schema literal controls.
- [x] 5.2 Implement the private target-compatible schema check without mutation or automatic closure; preserve the original schema and drop only incompatible strict true. Verify warning details, unchanged caller data, and exact serialized request controls.

## 6. Normalize existing OpenAI request schemas

- [x] 6.1 Add failing response-format and function input/output schema tests, including namespaced tools, covering recursive string propertyNames removal, warning cardinality/details, boolean/missing-type/non-string rejection, immutability, and unchanged ordinary data/valid schemas.
- [x] 6.2 Add one private non-mutating normalizer and use it at the existing request-schema boundaries; propagate errors before HTTP and retain local validation semantics. Run focused tests and target request comparisons.

## 7. Correct existing Bedrock OpenAI reasoning requests

- [x] 7.1 Add failing generate/stream request tests for native and single-prefix GPT-OSS versus other OpenAI models, root reasoning and provider-config paths, preserved nested fields, custom substring rejection, and unchanged Anthropic/other-provider/default controls.
- [x] 7.2 Tighten OpenAI model recognition and route effective effort to flat or nested target shapes without adding a model-family API; run focused tests and target request comparisons.

## 8. Correct request-aware Bedrock profile and tool routing

- [x] 8.1 Add failing profile generate/stream request tests for explicit budget presence (including raw zero, absent and null), namespace precedence, root overrides, thinking/token/sampling/effort/response-field behavior, tool paths, and unchanged caller data. Preserve the typed zero-as-omitted contract without public pointer migration.
- [x] 8.2 Thread target-stage request-aware Anthropic classification through the relevant request builders, preserving model identity and non-profile controls; run focused profile tests and target comparisons.
- [x] 8.3 Add failing actual-model Sonnet 4.6/Haiku 4.5 generate/stream tests for JSON-tool requests and existing text/finish/metadata collapse, with thinking, strict tools and unchanged other-model defaults.
- [x] 8.4 Add failing parallel-disable request tests across effective choices, raw/typed Anthropic options, inferred profiles, provider tools, synthetic-only/mixed JSON tools, and false/unset/no-tools/none/non-Anthropic controls. Assert exactly one correct choice.
- [x] 8.5 Separate native-output reliability from strict support, select and inject the synthetic JSON tool before common preparation, and forward the existing Anthropic flag via a private option projection. Run focused boundary tests and target comparisons without adding new APIs or response families.

## 9. Complete request evidence and shared-adapter validation

- [x] 9.1 Resolve fixture applicability from the inventory and replay preserved authentic inputs against generated target expectations in the integration worktree. No authentic inputs cover the new request edges: retain red-to-green focused request tests and exact-target probes instead of fabricating configurations/recordings. Record candidate failures separately; keep canonical target expectations for PR 7. See `evidence.md`; raw candidate diffs are preserved externally.
- [x] 9.2 Exercise allowed-tool resolution and schema normalization through Mantle generate/stream HTTP capture tests; verify request shapes, local rejection, warnings, identity, routing, and metadata namespace remain correct without claiming a live Mantle recording.
- [x] 9.3 Run `go test ./...` in `providers/anthropic`, `providers/openai`, and `providers/bedrock` (including Mantle), plus relevant root/schema tests. Run `mise run parity-check`, `mise run build`, `mise run vet`, `mise run lint`, and `git diff --check`; record exact commands/results and investigate every unexplained failure.
- [x] 9.4 Inspect candidate and registered request/output golden deltas, naming affected scenario paths and any unchanged controls. Record expected version-related failures with cause, capability fix owner and PR 7 certification handoff; do not disable comparisons or claim independent merge readiness with failing enforced checks. If UI wire behavior changed, stop and add the required cross-language scenario/checks after scope review.

## 10. Review and handoff

- [x] 10.1 Compare the implementation against the exact upstream versions again, verify provenance for every changed provider fixture, and reconcile the delta ledger so no assessed item is unassigned. Document deliberate ahead-of-baseline behavior and remaining durable gaps in `PARITY.md` or `upstream.yaml` without changing package pins.
- [x] 10.2 Prepare the PR description outside the repository with entry conditions, observable outcome, exclusions, concise validation/provenance, residual risks, and explicit handoffs: PR 2 request helpers/history ownership, PR 3 selection representation/execution ownership, and PR 7 scenario IDs/snapshot deltas/baseline failures.
- [x] 10.3 Verify implementation against these artifacts and archive the single active change before merge. Verification is summarized in `evidence.md`; the detailed report is preserved externally. Main specs were synced and the change archived on 2026-09-21. Land independently only if enforced checks pass with documented ahead-of-baseline behavior; otherwise preserve scoped review for the plan's validated cumulative integration landing.
