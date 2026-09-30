## Context

This is a PLAN-only proposal for #259, inspected against the authorized Go base `ec204b9164e9c7bdc74cb0cc416d55c0dedaffe1`. The issue's older Go links refer to `cf61a18d`; their line numbers are not used as current implementation evidence.

### Registered upstream assessment

`test/conformance/upstream.yaml` registers `ai@7.0.109`, `@ai-sdk/react@4.0.112`, and commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`. The issue's assessed version matches the current baseline. Source/tests were read with `git -C /home/nara/src/ai show <registered-commit>:<path>`; the shared checkout was not changed and its working-tree version was not used.

Exact upstream seams (all paths below are beneath `packages/ai/src/` at that commit):

| Evidence | Contract |
| --- | --- |
| `generate-text/stream-language-model-call.ts:643-688` | On completed model-call finish, required/named absence creates `ToolChoiceViolationError`, changes unified finish to error, retains raw finish, usage, provider metadata and performance, then surfaces error. |
| `generate-text/stream-language-model-call.ts:694-708` | Parsed calls enter the call-ID map before the invalid-call branch: invalid and provider-executed calls count toward the existence predicate. Named choice is not an allowlist. |
| `generate-text/stream-text.test.ts:1793-1987` | Required/no-call errors once and retains usage; named miss never invokes unrelated executor; PrepareStep overrides are enforced; violation does not retry. Valid required control precedes this block. |
| `generate-text/generate-text.ts:1151-1225`, `generate-text/generate-text.test.ts:1777-1977` | Equivalent unary predicate before tool input callbacks/approval; completed model call notification precedes rejection. |
| `generate-text/stream-text.ts:2607-2625` | Neither automatic nor callback-requested stream retry applies to a choice violation. |
| `generate-text/execute-tools-from-stream.ts:111-143,224-226` | Streaming approval policy/ID effects occur on tool-call arrival; dispatch is gated by model-call-end finish. Thus upstream does NOT prove absence of streaming approval effects for a named miss. |

### Current Go seams

- `streamtext.go:498,549-550,626-645` correctly resolves configured/default choice and per-step override before the provider call. At `683-685` only execution tools are copied into `stepCfg`, not the effective choice.
- `processStep` captures completed status, usage, finish and provider metadata at `1194-1203`, then unconditionally visits `executeTools` for completed/partial steps at `1330-1349`. There is no choice satisfaction check.
- `executeTools` resolves approval and can allocate IDs, sign/emits approvals or create denials at `1615-1677`; its execution-finish gate is later at `1691-1694`. Changing the finish alone does not meet the no-new-local-approval requirement.
- The processStep error exit at `689-692` precedes completed-step recording at `730-742` and usage aggregation/final finish at `776-792`. `emitError` at `2005-2007` emits an empty finish. Neither is sufficient for a completed semantic failure.
- `generatetext.go:32-46` and `agent.go:251-292` collect the same streaming engine and return `nil, error` on failure. No new partial-result API is needed or approved.

This confirms an existing core orchestration implementation bug. `test/conformance/PARITY.md` marks core orchestration/tools, UI/SSE and Agent as **mixed**. Root tests, provider-independent UI snapshots and schema-parsed frontend tests are the relevant proof layers; provider request snapshots alone cannot establish response enforcement. The map already documents Go's batched local approval handling and the generate-to-stream delegation boundary.

## Goals / Non-Goals

**Goals:**

- Enforce the effective required/named choice on a completed call before any new local executor or approval work for that call.
- Record and finalize the failed completed step, retaining usage, raw finish, response/provider metadata, original content and prior successful steps.
- Surface one semantic error through existing StreamError/Err/OnError paths, final reason error, with no retry/continuation; exercise all four public entry points and callback data on generate failures.
- Prove valid controls, step precedence/reset and unchanged incomplete/error/cancellation behavior, plus the pinned UI contract.

**Non-Goals:**

- Named-choice allowlisting; rejection merely because an extra tool appears alongside the selected tool; new enforcement for auto/none.
- Provider request preparation/allowedTools (#218), hosted shell choice (#32), caller-aware routing (#211), tool repair/refinement (#210), output redesign, or undoing provider effects.
- New exported error/result/callback API, custom TypeScript-shaped error class, provider interface changes, pin/dependency upgrades or live provider recordings.
- Changing approval execution from prior input messages, hiding already emitted provider calls/results/approval events, or moving all approval timing to upstream's eager streaming model.

## Decisions

### 1. Use the effective orchestration choice and pinned existence predicate

Carry the resolved per-step choice into response processing as a value in the step-local configuration (or equivalently an explicit internal argument). Do not consult only the global choice or infer it from filtered tools/provider wire transformations. Preserve step-local override isolation: later nil overrides revert to configured choice/default auto.

For a completed provider finish, required is satisfied by any parsed returned call; named is satisfied by at least one parsed call whose name matches. Calls marked invalid or provider-executed count for presence but retain their existing validation/execution semantics. Matching-plus-other calls are accepted. Auto and none are unchanged. Tool-call-shaped text is not a parsed call. Align duplicate call-ID/repaired identity handling with the pinned parsed-call collection without introducing repair behavior.

Alternative rejected: named allowlisting or checking only executable/valid client calls would tighten behavior beyond upstream and the approved scope. Request validation alone is insufficient because providers may ignore choice.

### 2. Separate semantic completion failure from ordinary processing failure

After a completed model call has captured usage/finish/metadata, validate before visiting the executor. A violation uses a narrowly scoped internal terminal outcome distinct from the existing ordinary processStep error return. Keep the provider's raw finish and replace only unified reason with error; retain content and response messages and emit finish-step. The outer engine must record the failed completed step and update last response/provider metadata before terminal finalization. Aggregate prior steps plus this completed failed call, invoke existing completion callbacks once, and emit final StreamFinish(error) with retained total usage. Surface exactly one StreamError and OnError notification and store the result error through existing machinery.

No ordinary processStep error early return or empty `emitError` finish may discard the completed call. A dedicated terminal condition must prevent retries and continuation even if there are no client calls, only provider-executed calls, or all returned client calls already have provider results. Do not rely solely on unresolved-call detection or user stop conditions. Existing provider errors, partial EOF and canceled unfinished calls are not reclassified as choice violations.

Alternative rejected: simply return `error` from processStep loses completed data; simply change finish reason still permits local approvals and may continue. Public API remains unchanged: use existing Go `fmt.Errorf` conventions, not a new exported sentinel/type. Exact UI error wording is normalized with existing UI error hooks in snapshots, rather than requiring TypeScript error class serialization.

### 3. Skip the whole new local approval/execution phase on violation

Do not invoke call-level approval policies, dynamic NeedsApproval, approval ID generation/signing, automatic approve/deny logic, Execute/ExecuteStream, or execution callbacks for the violating completed step. No new local approval request/response or synthetic denial/result may be created. Previously streamed input/call and provider-originated output/approval content remains observable; provider effects cannot be undone. Existing provider-event handling (including its signing behavior) is not moved into the new local gate. Prior-message approval resolution occurs before the model call and is unchanged.

This is an issue-required safety adaptation within Go's already documented batching boundary. It is not exact parity with upstream's eager streaming approvals. Other disallowed finishes retain existing approval observation semantics; the lifecycle spec explicitly distinguishes this exception.

Alternative rejected: suppressing every provider event or changing prior-message approvals would widen scope; matching TypeScript's early approval timing would violate the accepted issue safety goal.

### 4. Preserve entry-point contracts, prove retained callback data

GenerateText and Agent.Generate remain `nil, error` on violation. Retained completed-call usage and metadata are proven through their existing OnStepFinish/OnFinish callbacks, not a newly returned partial result. StreamText and Agent.Stream expose the failed completed step and totals through existing accessors plus callbacks. Agent settings, per-call overrides and PrepareStep precedence remain unchanged.

### 5. Conformance-first proof, not invented provider provenance

Implementation starts after `mise deps` with red unit cases and provider-independent UI fixtures under `test/conformance/ui/effective-tool-choice/{required-no-call,named-miss}/`, adapting the existing `ui/invalid-provider-tool-input/generate.mts` and `ui_conformance_test.go` patterns. Synthetic `input.jsonl` contains core model parts derived from the pinned test scenarios; it is NOT a provider recording/import. Generate `expected.jsonl` via installed pinned `ai` in `test/conformance/tools`, including deterministic IDs and normalized UI error text in both runtimes. Record source commit/test identity in fixture-local provenance notes. Keep approval-specific safety tests separate from upstream-equivalence snapshots because timing differs.

Capture the pinned runtime's exact error/finish-step/final-finish sequence first, compare Go replay, and document any observed ordering difference before accepting it; do not guess snapshots or silently bless Go output. Loader extensions for represented usage/finish fields must be focused if existing decoding cannot express the fixture. Register fixture generation in the existing generator if needed. No changes to provider `recorded/` or `upstream/` inputs or INDEX provenance are required.

Add `test/integration/testserver/scenario_effective_tool_choice.go` and `test/integration/effective-tool-choice.test.ts` using existing registered scenarios. Parse SSE with `parseJsonEventStream`/`uiMessageChunkSchema`, assert tool-input visibility on named miss, terminal error and finish(error), no newly local outputs/approvals, and assembled message content/state using `readUIMessageStream` with intentional error handling. Do not claim an error stops assembly automatically or assert successful output state. Include valid controls and required/no-call content. This proves the pinned frontend wire/assembly boundary, not a live provider or every React-hook lifecycle.

Existing request tests requiring concrete updates: `TestStreamText_ToolChoice`, `TestStreamText_ToolChoiceStepPrecedence`, `TestGenerateText_ToolChoice`, `TestToolLoopAgent_ToolChoice`, and `TestToolCallers_ProviderOnlyAndNamedChoice` (`tool_caller_test.go:222-237`). Their text-only required/named responses currently assert success. Preserve request/default/empty-tool assertions while supplying satisfying responses for success controls or explicitly expecting semantic error; the configured-required final text-only step in the reset test must no longer assert unconditional success. For the provider-only named-choice test, preserve the exact routing/model-visible tool list (`lookup`, `programmatic`), provider-options (`allowedCallers: ["programmatic"]`) and forwarded named-choice (`lookup`) request assertions; return a matching `lookup` call or explicitly assert semantic failure for the text-only response.

## Risks / Trade-offs

- [Semantic error and finalization event ordering] → Capture pinned runtime snapshots before implementation; review the exact stream/UI sequence and prevent duplicate errors/finishes or discarded usage.
- [Approval safety stronger than upstream streaming timing] → Test Go local policy/NeedsApproval/IDs/signing/execution suppression explicitly, preserve provider events, and retain the existing PARITY batching boundary rather than claiming identical timing.
- [Duplicate IDs or repaired call identity differ between slice and upstream map] → Inspect pinned parsed-call-set behavior and add a focused identity regression if reachable; escalate any necessary repair/scope change rather than guessing.
- [Unfinished calls mistakenly treated as semantic violations] → Tests for provider error, empty/partial EOF and cancellation before finish with required/named choice preserve existing lifecycle outcomes.
- [Delegation mistaken for proof] → Invoke StreamText, GenerateText and both Agent methods directly, including retained callbacks on nil-result failures.
- [Existing tests rely on missing validation] → Update the named request-only tests without weakening choice forwarding assertions; run the root suite and parity/frontend gates.

## Migration Plan

No data migration, dependency upgrade or request schema change. Land fixtures/red focused tests, then the shared-engine fix and compatible request-test updates, then frontend proof and required validation. Treat a provider response that previously succeeded despite an unsatisfied required/named choice now failing as the intended correctness change. Implementation is deferred; this proposal changes no runtime behavior. Rollback of a later implementation would restore the known unsafe behavior and requires explicit review, not a provider-specific bypass.

## Open Questions

No unresolved product/API decision remains after supervisor approval. Exact pinned UI ordering and duplicate-ID behavior are implementation evidence gates, not permission to invent behavior. Any additional architecture/API/scope choice discovered while implementing must be escalated before changing this contract.
