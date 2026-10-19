## 1. Freeze parity decisions and failing evidence

- [x] 1.1 Apply independent plan review corrections against issue #108, original WP14/ownership and exact upstream `eb77f09e`; settle presence-aware Reason representation and document compatibility impact before source edits.
- [x] 1.2 Add failing provider JSON/native absence-empty-text denial tests, Gateway missing/false decision and role tests, and both-client registered approval request/response fixtures. Keep synthetic inputs out of recorded/upstream fixture directories.

## 2. Apache domain and native semantics

- [x] 2.1 Implement canonical `ToolResultOutput.Reason *string` in provider/types.go and JSON codecs; migrate root/native/client/capture readers and writers, default only on nil, synthesize nil from normalized empty approval reasons, and document exported source compatibility impact.
- [x] 2.2 Audit/fix OpenAI Responses approved/denied/mixed conversion against pinned approval tests, including store true/false, duplicate IDs and execution-denied approvalId options; preserve native metadata/options. Verify existing Anthropic/Bedrock/compatible denial default behavior against their registered source.

## 3. Gateway transport and independent Go client

- [x] 3.1 Map tool-role approval-response after schema requiredness validation, preserve false and options, and map execution-denied plus nested supported result-content options without widening unrelated unions.
- [x] 3.2 Extend strict unary and streaming approval output encoding with bounded metadata/IDs and available-state correlation; cover unknown/completed/conflicting calls, ordering, finish/error/cancellation and no replay.
- [x] 3.3 Extend independent providers/grafana unary/stream decoding and continuation encoding; keep assistant approval-request prompt rejection because it is not a registered V4 input arm.
- [x] 3.4 Normalize application-only approval-response bookkeeping in core sanitizePromptForProvider on a copy after ownership/correlation handling; preserve registered fields and caller-history immutability.

## 4. Both-client continuation and execution evidence

- [x] 4.1 Add Gateway-owned exact-pinned TS and Go unary/stream tests that consume actual output and submit approved, denied and mixed continuations, checking actual next native HTTP requests and local versus provider execution ownership.
- [x] 4.2 Cover direct and configured fallback continuation, candidate-specific namespaces, eligible precommit failover, first-part commitment and no replay after approval output/effects; include cancellation and bounded cleanup.
- [x] 4.3 Prove opaque approval/tool metadata and output/nested options survive; independently toggle operator capture and consumer middleware/capture without response changes. If root frontend behavior changes, add schema-parsed test/integration evidence.
- [x] 4.4 Prove actual high-level Go GenerateText/StreamText response-history resume and pinned TypeScript high-level normalization. Keep direct V4 option-scope evidence distinct from high-level TS approval-response options that pinned core drops; preserve the unsupported content-wrapper options boundary.

## 5. Validation and review handoff

- [x] 5.1 Update Gateway capability docs and affected godoc, synchronize accepted spec changes, and update PARITY.md only for changed stable coverage/evidence boundaries.
- [x] 5.2 Run focused tests, affected modules' tests through go.gateway.work where required, both-client contract/runtime tests, race tests for affected state, and `mise run parity-check`; preserve exact commands/results and distinguish pre-existing failures.
- [x] 5.3 Run OpenSpec validation and final independent implementation review; fix in-scope findings and record unresolved gaps. Check staged diff excludes `.codex/implementation-reports/` before any local commit; do not push, open PRs or mutate external systems.
