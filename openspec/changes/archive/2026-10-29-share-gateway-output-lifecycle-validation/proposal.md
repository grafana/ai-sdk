# Proposal

## Why

Every provider-output lifecycle rule lives inside ProviderWire V4's stream state machine and unary mapper, mixed with its encoding. A second adapter, such as the test adapter or Chat (#248), gets none of these checks unless it copies them. The test adapter already does: it repeats ProviderWire's "clean close only after a finish" decision. This change moves the rules into the shared execution runtime, which applies them for every adapter. No rule changes (#483, part of #394, after #482).

**#393 landed first.** #483 was meant to give #393 one place to change a rule. #393 was instead delivered by #583 before this change, which rewrote the stream rules for sequential response segments: each `response-metadata` starts a segment, each `finish` ends one, and the stream ends at provider close. This change moves the rules as #583 left them. Where the issue text describes the earlier single-response rules ("parts after a finish are suppressed"), the current specs win: after a `finish`, only `response-metadata`, raw or provider error parts may follow, and any other part is invalid output.

## What Changes

- **Tool history is rebuilt when a call begins.** The runtime rebuilds provider-executed tool history from the prompt as the first step of a call:
  - this happens before host policy, the context check and selection, so error precedence stays as it is today;
  - inconsistent history fails as an invalid request with zero selections and zero model calls. ProviderWire writes the same 400 document as today;
  - the rebuilt history is not mutated afterwards. Each validator copies it when seeded, and `Call.ToolHistory()` returns a copy.
- **Shared stream validation.** The runtime validates every part before the adapter receives it, with the segment rules of #583:
  - the start position, which every part consumes, error and raw parts included;
  - response segments: `response-metadata` and `finish` only while quiescent (no open text, reasoning or tool input, no pending preview), and after a `finish` only `response-metadata`, raw or provider error parts;
  - text and reasoning IDs unique per segment; tool and approval IDs unique per stream, including calls seeded from history;
  - the tool input, call, preview, result and approval phases;
  - the `finish` preconditions and a registered finish reason.

  A violation ends the stream with a new **invalid-output** wait outcome: the runtime cancels and drains the stream. Invalid output is sticky.
- **The runtime decides how a stream ends.** A provider close after a valid `finish` returns a new **finished** outcome, a terminal success. Any other close stays the existing closed outcome, a premature close. Idle timeout, deadline and cancellation after a `finish` keep their error outcomes, as #583 decided. ProviderWire and the test adapter stop tracking the phase themselves.
- **Shared unary validation.** `Generate` validates the result before returning it: call uniqueness, including history re-emission; result correlation, including history; approvals; previews still pending at the end; a registered finish reason; and non-tool parts that are not provider-executed, dynamic or preliminary. A violation returns the invalid-output error. #583 did not change unary rules.
- **Invalid output keeps today's documents.** Invalid output is classified as internal. Its attempt view is the selected-attempt view, so ProviderWire's SSE terminal error, its unary 500 document and its execution metadata stay byte-identical, including the `finish` frame already written before a post-finish violation.
- **Scope reading: only the lifecycle rules move.** The issue says ProviderWire keeps "only" UTF-8, metadata shape and the safe-integer bound. This change moves the lifecycle rules the issue enumerates. These non-lifecycle content-validity checks stay in ProviderWire unchanged:
  - source validity, including a nil source;
  - the custom kind rule;
  - file data `Validate()`;
  - the non-negative token count;
  - response identity and timestamp range;
  - the recovered-panic error-part refusal.

  Consequence: the test adapter would forward a recovered panic as a provider error part today. Whether these checks move to the runtime is an Open Question for nara (design).
- **ProviderWire also keeps its representation checks and plumbing:** UTF-8, metadata, citation and raw-usage shape, the JavaScript safe-integer bound, raw-part projection and the `includeRaw` drop, the synthetic `stream-start` and its warnings, the execution overview on every `finish`, and encoding failures. Its internal state machine and mapper delegate every lifecycle rule to the same execution validators, so ProviderWire holds no copy of any rule. Its `phase` field stays only as a projection of its validator, because existing white-box tests read it.
- **Test adapter.** It consumes the invalid-output and finished outcomes and drops its own phase tracking. Parity scenarios show that it receives invalid output at the same part as ProviderWire. They leave out the content-validity classes above on purpose, because the runtime does not check them.
- **Docs.** The execution package godoc, the architecture walkthrough and the glossary name the runtime as the owner of output lifecycle validation and of the clean-close decision, and no longer give adapters a "protocol state machine".

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-execution-runtime`:
  - new requirements: prompt tool-history reconstruction, shared stream and unary output validation, stream end decided by the runtime, the invalid-output outcome, and representation checks that stay with each adapter;
  - modified requirements: shared adapter runtime (adapters no longer own lifecycle validation), stream wait outcomes (adds invalid output and finished), closed failure classification (adds inconsistent history and invalid output), test-only adapter evidence, and ownership documentation.

## Impact

- **Code:** `ai-gateway/internal/execution` (history, validators, `Runtime.Call`, `Call.Generate`, `Stream.Next`, `View` and classification), `ai-gateway/providerwire/v4` (`handler.go`, `request.go`, `stream.go`, `stream_tools.go`, `reasoning.go` and `response.go` delegate to it), and `ai-gateway/internal/testadapter`.
- **Go API:** internal only. The `execution` package gains a tool-history type and sentinel, stream and unary validators, `ErrInvalidOutput`, `WaitInvalidOutput`, `WaitFinished` and `Call` access to the history. Nothing outside the `ai-gateway` module consumes them.
- **Wire:** no change. Every ProviderWire status, document, SSE frame and execution overview stays byte-identical, including error precedence, multi-segment streams, the `finish` followed by one `internal_error` for content after a `finish`, and the error after a written `finish` on idle, deadline or cancellation.
- **Tests:** "unchanged" means the ProviderWire lifecycle test files the issue names, as they are on main after #583: no existing `providerwire/v4` `*_test.go` file is edited, including `stream_segments_test.go` and `stream_segments_replay_test.go`. Existing `internal/execution` fixtures that feed lifecycle-invalid parts or results are runtime-test inputs; they are made lifecycle-valid without changing what they assert. New tests are:
  - one unit test per validator rule in `execution`, plus the history cases (deferred result, orphan result, conflicting duplicates, approval re-emission, independent validators over one history) and the segment cases;
  - runtime outcome tests;
  - test-adapter parity rows;
  - an agreement test showing that the runtime and ProviderWire reject at the same part for each rule class.
- **Parity:** this is a Gateway-internal refactor with no counterpart in the `@ai-sdk/*` packages pinned in `test/conformance/upstream.yaml`. The LanguageModelV4 stream-part contract is unchanged. `upstream.yaml`, `knownGaps`, recorded and upstream fixtures, and conformance expectations do not change. The segment replay of the seven upstream fixtures from #583 keeps passing unchanged. The PARITY.md "AI Gateway" row keeps its status and evidence location, so it does not change.
- **Delivery:** branch `nrbrd/gateway-lifecycle-validation`, based on `main` after #583. The issue names `nrbrd/gateway-host-contract` as the base, but #482 has merged as #581. Draft PR title: `refactor(ai-gateway): share provider output lifecycle validation`.
- **Out of scope:** changing any rule, including the segment rules of #583; moving the content-validity checks listed above; adapter-specific rules such as Chat's parallel tool and tool-choice rules; size and cardinality accounting (gateway-resource-policy); the Agent Observability first-segment limitation (#582).
