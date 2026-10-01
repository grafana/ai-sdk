## 1. Establish pinned failing contracts

- [x] 1.1 Compare matching upstream UI/Agent implementation and tests; enumerate states and presence boundaries without changing pins.
- [x] 1.2 Establish failing JSON, reader and conversion regressions for seven static/dynamic states and optional values.
- [x] 1.3 Add provider-independent fixtures/differentials without fabricating or editing recorded provider input.

## 2. Persisted types and scoped chunk presence

- [x] 2.1 Extend tool/approval fields and migrate pointer callers, preserving unrelated scalar normalization.
- [x] 2.2 Preserve empty metadata/raw JSON; reject malformed non-null optional fields before presence is erased.
- [x] 2.3 Preserve decoded title/reason/preliminary/provider-execution presence and registered approval fields without scalar API or framing changes.
- [x] 2.4 Deep-clone new pointers/maps/raw approval data; test caller and snapshot isolation.

## 3. Reader lifecycle and resume

- [x] 3.1 Match static/dynamic transitions, raw-input arms, preliminary replacement and false-vs-omitted provider execution/result placement.
- [x] 3.2 Merge approval responses without losing request data; verify denied/output transitions.
- [x] 3.3 Add isolated initial-message seeding, ID/data matching, option reuse and empty/invalid-delta cases.
- [x] 3.4 Preserve progressive/blocking error contracts and no synthetic final snapshots; exclude #181/#180.

## 4. Conversion behavior and data hook

- [x] 4.1 Filter streaming/preliminary tools before output callbacks for both tool kinds.
- [x] 4.2 Match nullish input and presence-based call/result metadata selection.
- [x] 4.3 Preserve denied text/empty reason and distinct negative-approval behavior; protect output callbacks and existing content ordering.
- [x] 4.4 Add text/file-only data conversion with nil/error handling; retain required empty text without changing Text string.
- [x] 4.5 Bound differential normalization to documented optional provider representations, not required text or metadata selection.

## 5. Agent validation and terminal normalization

- [x] 5.1 Use one normalized clone for conversion/assembly; assert caller isolation and zero calls on failure.
- [x] 5.2 Validate represented roles/parts, required/forbidden state fields and approvals without separate prior-call requirements.
- [x] 5.3 Apply configured static schemas and pinned terminal normalization; check output before dynamic normalization.
- [x] 5.4 Document application/unrepresented provider-schema boundaries; do not invoke history-time ValidateInput callbacks.

## 6. Differential and hook-level proof

- [x] 6.1 Compare persistence/assembly/conversion with pinned TypeScript APIs across tool states and optional values.
- [x] 6.2 Parse SSE with parseJsonEventStream/uiMessageChunkSchema and assert fields plus assembled history.
- [x] 6.3 Prove real useChat persist/remount/resume, final output and provider prompt; verify ordered, isolated tool-message coalescing.
- [x] 6.4 Compare Agent-specific normalization/shared schemas and classify representation/proof boundaries.

## 7. Documentation and independently green delivery

- [x] 7.1 Document API/pointer migration, persistence, resume and validation; keep lifecycle gaps in #181.
- [x] 7.2 Update stable parity evidence/boundaries only; preserve fixture provenance.
- [x] 7.3 Pass all-module checks, example builds/tests, integration, parity and candidate-source CI; inspect skips.
- [x] 7.4 Verify/sync/archive deltas; before release run `MODULE=. mise run verify-published-module`. Consumer adoption needs published-dependency evidence.

Validation passed against 7.0.116 / React 4.0.119, including exact-source provider
shape checks and integration typechecking. Candidate-source Build & Test
[run 36770987472](https://github.com/grafana/ai-sdk/actions/runs/36770987472) passed;
the separate OpenSpec gate required archiving. Approval input-schema provenance
and schema-transform/refinement paths remain unrepresented.
