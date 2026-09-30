## Context

The original design reference was `ai@7.0.109`, `@ai-sdk/react@4.0.112`, commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`. The issue cites the older 7.0.107 reference. The initial evidence table below refers to `packages/ai/src/` at the original design commit.

Merging `origin/main` advances the registered reference in `test/conformance/upstream.yaml` to `ai@7.0.116`, `@ai-sdk/react@4.0.119`, commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`. Matching source/tests were compared and the represented contracts revalidated against those packages. Newly added approval `inputSchemaInput` and schema-transform/refinement paths remain outside the represented Go API and this change's proof; passing the existing differentials does not establish those paths.

| Seam | Current evidence | Pinned behavioral reference |
| --- | --- | --- |
| Persisted tool shape | `message.go:75-134`, `message_json.go:38-50` lack title/toolMetadata/preliminary/rawInput and approval descriptor/requestReason | `ui/ui-messages.ts:289-513`, `ui/validate-ui-messages.ts:121-399` |
| Reader transitions | `ui_message_reader.go:242-349` ignores those fields and replaces approval response data; snapshots clone only old fields | `ui/process-ui-message-stream.ts:183-389,589-840` and matching tests |
| Conversion | `convert.go:124-212,338-430` includes streaming input by default, loses raw-input fallback, uses wrong denied default and local-result metadata | `ui/convert-to-model-messages.ts:35-419` and matching tests |
| Validation | `agent.go:727-849` already validates some required fields but not configured schemas or normalization | `agent/create-agent-ui-stream.ts`, `ui/validate-ui-messages.ts:483-713` and matching tests |
| Resume | `ui_message_reader.go:10-76` starts empty and only offers an ID option | `ui-message-stream/read-ui-message-stream.ts:43-125` and matching tests |

Existing `ToModelOutput` support, provider-executed result placement, custom content, approvals, partial JSON repair, provider file references and filename presence are not missing. In particular `prompt/create-tool-model-output.ts` bypasses `toModelOutput` in both error modes; the current Go error bypass is correct and will receive regression coverage, not a callback behavior change. UI/SSE, React and core/tool coverage is currently mixed (`test/conformance/PARITY.md`); passing a chunk schema does not prove resumed messages.

## Goals / Non-Goals

**Goals:**
- Preserve supported tool/approval state through chunks, immutable snapshots, persistence and resumed conversion.
- Make field presence, filtering and validation deterministic against the pin, with concrete Go API shapes.
- Validate before `Agent.Stream`/provider calls and prove actual hook resume, not just initial SSE parsing.

**Non-Goals:**
- Reader error callback, terminate-on-error, cancellation or lifecycle APIs (#181); SSE framing (#180); baseline upgrades; provider producer redesign.
- General exported UI validator or metadata/data schema configuration; full TypeScript generic inference.
- Blanket migration of all scalar booleans/strings or a generic optional-field framework.

## Decisions

### 1. Extend the existing concrete tool parts, not a parallel lifecycle model

Both `ToolInvocationPart` and `DynamicToolUIPart`, and their structurally matching `toolPartFields`, gain:

| Field | Go representation | Presence contract |
| --- | --- | --- |
| `Title` | `*string` | nil absent; pointer to empty string present |
| `ToolMetadata` | `map[string]json.RawMessage` | nil absent; non-nil empty map emits `{}`; object-valued JSON only |
| `RawInput` | `json.RawMessage` | nil absent; arbitrary valid JSON retained, including `null` and JSON strings |
| `Preliminary` | `*bool` | nil absent; explicit false retained; used on output-available |
| `ErrorText` | `*string` (migration) | nil absent; empty string valid when required in output-error |

`ToolApproval` gains `Descriptor json.RawMessage` and `RequestReason *string`; `Reason` migrates to `*string` for nullish denial fallback. Existing ID, Approved, Signature and IsAutomatic remain. Add targeted tool JSON serialization for the empty metadata object, using the existing file-reference presence pattern in `message.go`, rather than letting `omitempty` drop it. Decoding must retain the new fields. Typed tool/approval decoding must reject explicit null or wrong JSON types for optional non-null string/bool/object fields before Go pointer/map decoding can collapse them into absence; RawInput, Input/Output and Descriptor retain their permitted JSON null values. This targeted type check is needed for persisted validation, not a general-purpose JSON schema API. Snapshot/initial-message clones must copy every pointer, map value, raw byte slice and approval object.

Existing persisted `ProviderExecuted`/`IsAutomatic` bool and `Signature` string representations remain unchanged: this proposal does not claim lossless absent-vs-false/empty UI-part serialization for those fields. This bounded representation does not permit sticky-true reader behavior: decoded chunk `providerExecuted: false` must clear a prior true value, while absence inherits it. These representation boundaries must be visible in differential expectations, not silently called full presence parity. Alternatives (new discriminated Go structs for every state or migrating every optional scalar) would cause unrelated public API churn.

### 2. Preserve decoded chunk presence without changing source/document title APIs

`UIMessageChunk.Title`, `Reason`, `Preliminary` and `ProviderExecuted` are scalar fields (`chunk.go:52-110`). Keep their public types and add only narrow private presence bookkeeping in the existing custom JSON codec for decoded tool title, approval reason, preliminary and provider execution. Decode/re-encode and reader transfer must retain explicit empty strings/false, including `providerExecuted: false`; direct Go literals continue existing zero-value normalization (empty/false means omitted for optional fields). This is a construction-boundary tradeoff for review, not a claim of full producer presence parity. Do not create a generic presence framework or alter unrelated source-title behavior.

Add `UIMessageChunk.ApprovalDescriptor json.RawMessage` serialized as the registered `approvalDescriptor` on approval-request; serialize its existing `Reason` as the registered request `reason`. Map those into persisted `Approval.Descriptor` and `Approval.RequestReason`. There is no `requestReason` chunk wire field. Required errorText is present by discriminator even when its scalar string is empty. Keep chunk types, SSE framing and all unrelated field serialization unchanged.

### 3. Follow pinned lifecycle updates and support an isolated starting message

Track title/tool metadata on partial tool calls; input deltas retain them. Update title/tool metadata only when supplied, preserving an explicit empty title/object. Follow the pinned static/dynamic updater rules, including clearing stale output/error/preliminary on transitions and preserving raw input only where the target dynamic updater does so. Static `tool-input-error` uses omitted Input plus RawInput; dynamic uses Input. Preserve call/result metadata association. At static/dynamic tool updaters and approval responses, supplied `providerExecuted` replaces the previous value, including decoded false clearing true; an absent field inherits the previous value. This must produce local tool-role results after explicit false, versus provider-inline results when prior true is inherited; persisted bool serialization and direct-literal zero-value normalization do not excuse incorrect result placement.

Static output-error continuations preserve prior RawInput so rejected/legacy input still projects through conversion; static output-available clears it. Dynamic updates retain RawInput as the pinned updater does.

Approval responses merge the previous approval rather than reconstructing it, retaining descriptor, request reason, signature and automatic status. Approval denied/output transitions retain target data. Successive preliminary/final outputs replace one part, not append results, and explicit false differs from omitted preliminary.

Propose `WithUIMessageReaderInitialMessage(message UIMessage) UIMessageReaderOption`. Clone at option construction and again per reader consumption so later caller mutation or option reuse cannot alias state. Seed contents only from an assistant message. For a non-assistant initial message, ignore its parts/metadata but retain its supplied ID on an empty assistant message, matching pinned `read-ui-message-stream.ts:81-83` and `process-ui-message-stream.ts:65-77`. A later start chunk may replace either seed's ID; use the existing generated-ID fallback only when no ID is supplied. Active text/reasoning/partial-input maps start empty as in upstream; continuation still needs start chunks for active delta sequences. Existing tool lookup and data-ID matching must find persisted parts using target step/ID rules. No initial snapshot is emitted solely because an option was supplied; empty progressive input emits nothing, while blocking assembly returns the cloned initial assistant state or empty assistant carrying a non-assistant seed's ID. No option retains existing empty-stream/generated-ID behavior.

Do not change `StreamUIMessage` ignoring ChunkError and closing on malformed transitions, or `AssembleUIMessage` returning errors. These intentionally split Go contracts remain separate from target read-helper error/cancellation API gaps tracked by #181.

### 4. Correct conversion at the existing common-tool seam

- Always omit input-streaming calls, approvals and results, regardless of `WithIgnoreIncompleteToolCalls`.
- With that option retain approval-responded, output-error, output-denied and output-available only when preliminary is not true. Filtering removes the entire tool part before callback invocation. Without the option preliminary outputs convert like other available outputs.
- For output-error call input use the target nullish rule: non-null Input, otherwise RawInput, otherwise absent. Do not mistake a present JSON `null` for non-null input. Call metadata is call metadata, falling back to result metadata only for output-error.
- Provider-executed inline results use result metadata with call-metadata fallback only when result metadata is absent, not when it is an explicitly empty object. Select nil-versus-present source metadata before the existing provider-options conversion. Local tool-role results (including denied) use call metadata, not result metadata. Preserve approval request/response placement and synthetic execution-denied results for negative approval-responded.
- Output-denied local result is error-text with `Approval.Reason` when present, including empty, otherwise exactly `Tool call execution denied.`. Do not replace it with execution-denied; that is the distinct approval-responded negative path.
- Retain present empty call/result metadata objects in persisted tool JSON. Existing provider-domain codecs can normalize empty provider-options maps and optional empty request/response Reason strings to omitted fields; do not migrate those types here. Differential comparison must list only those existing representation cases explicitly, never drop empty values generically. In particular this cannot normalize a present empty denied error-text value or excuse fallback to populated metadata.
- Keep successful-output `ToModelOutput` callback semantics and error propagation; do not invoke it for error-text/error-json modes, denied or filtered parts. Preserve tool context and plain string-vs-JSON output mapping. Approval request `RequestReason` projects to the existing provider content `Reason` field.

Propose `WithConvertDataPart(fn func(DataPart) (*provider.ContentPart, error)) ConvertOption`. Invoke on user and assistant data parts in original part order, preserving step-block splitting; never on system parts. Nil callback/result skips. Accept only `provider.ContentPartTypeText` or `provider.ContentPartTypeFile`, preserving the returned content directly; reject other discriminators with contextual Go errors. Callback errors abort conversion before Agent streaming where that conversion is used. Using existing provider content types plus a strict guard is smaller than introducing another model-content hierarchy. Agent helper configuration is unchanged: direct callers opt into the hook through `ConvertToModelMessages`; no new Agent data-converter option is implied.

The approved codec extension keeps `provider.ContentPart.Text` as `string`. Its existing `Type` discriminator identifies text parts, whose JSON encoder must always emit the required `text` field, including `""`. All other variants retain their existing encoding. This fixes the data converter's empty-text projection without a public pointer migration, blanket empty-value normalization or broader provider-union redesign.

The approved prompt-preparation extension coalesces consecutive tool-role messages in `sanitizePromptForProvider`, after removing approval bookkeeping and before provider invocation. Preserve part order; move each preceding message's provider options to its last content part using the pinned deep merge, with part options overriding message options, and retain the final message's options on the combined message. Empty tool messages participate in coalescing before final removal. This changes provider prompt preparation, not direct `ConvertToModelMessages` grouping or public message types.

### 5. Validate and normalize once before Agent conversion

Extend the internal `validateAgentUIMessages` seam to return a deep-cloned normalized slice plus error, not introduce a general exported validator. `CreateAgentUIStream` uses that same slice for conversion and its enforced original-message assembly option. No caller mutation and no provider invocation on failure. Reference **`validateUIMessagesForAgent`**, not the public safe-validator default: the Agent variant enables obsolete terminal-tool normalization even without a supplied tool map.

Reject nil/empty histories before Agent invocation, then check represented message roles/parts and tool state field/approval constraints from the pinned schema. Dynamic reader updates may retain RawInput outside output-error; remove that unsupported field only on the normalized Agent clone, matching pinned validation, without changing reader persistence or weakening other state constraints. The required/forbidden lifecycle matrix is:

| State | Input | Output / error | Approval |
| --- | --- | --- | --- |
| input-streaming | optional; no configured input-schema check | neither | forbidden |
| input-available | required | neither | forbidden |
| approval-requested | required | neither | required ID; approved/reason forbidden |
| approval-responded | required | neither | required ID and approved bool |
| output-available | required | required output; error forbidden | optional, approved must be true |
| output-error | optional; RawInput supported | required error string, including empty; output forbidden | optional, approved must be true |
| output-denied | required | neither | required ID and approved false |

Preliminary/result metadata are constrained to target-supported states; metadata must contain valid JSON objects. Use existing nonempty tool ID/name invariants; do not invent separate prior-call cross-reference requirements for whole lifecycle parts. Represented unsupported/unknown parts cannot bypass Agent validation. Opaque message metadata/data payloads get structural JSON handling only, not application schemas.

For configured **static** tools reuse `Tool.InputSchema`/`OutputSchema` and `schema.Schema.Validate` according to exact target gates:
- Streaming input has no schema check. Available input, approval states and output-denied validate input.
- Output-error invalid current-schema Input becomes dynamic rather than rejects; absent Input/legacy RawInput remains loadable without treating RawInput as validated input.
- Output-available validates input; an invalid empty object normalizes to dynamic, but invalid nonempty input rejects. If configured, output schema is still checked **before** normalization.
- Missing terminal static tools (output-available/error/denied) become dynamic; missing nonterminal tools reject. Preserve name, identity and every retained field during normalization.
- Dynamic parts get state/shape validation but no configured tool schemas, matching the target's explicit dynamic-tool support boundary.

Provider-defined Go tools without a represented input schema cannot acquire a fabricated schema; schema checks apply where the Go configuration supplies one. This and absent metadata/data schema configuration must be documented as support boundaries, not claimed exhaustive validation parity. `Tool.ValidateInput` execution callbacks are not a substitute for upstream UI schema validation and are not newly invoked for history. Direct `ConvertToModelMessages` and `StreamText` UI conversion do not acquire automatic Agent validation.

### 6. Prove persistence, model projection and hook resume separately

Create provider-independent core fixtures or focused tests first and confirm current Go failures; never invent recorded provider inputs. Enumerate all seven static/dynamic states and absent/empty/false/null values relevant to this design. Differential tests use pinned `readUIMessageStream`, conversion, validation and Agent validation reference where applicable; normalize only existing documented Go field representations, not new regressions. JSON equivalence rather than map byte ordering is the assertion.

Add deterministic Go testserver cases and Vitest tests akin to `file-input-presence.test.ts` / `upgrade-tool-metadata.test.ts`: parse every SSE event with `parseJsonEventStream` and `uiMessageChunkSchema`, compare assembled messages and converted model messages after persistence. Add an actual `useChat` mount/persist/remount/resume case in the existing React test setup, retaining a preliminary tool/approval history, resolving it, sending resumed history through Go Agent validation/conversion, and asserting the fake provider received the expected final model history. Cover negative/schema-failing requests with provider invocation count zero. Pinned target source/tests, not hand-authored Go output, own expected behavior.

## Risks / Trade-offs

- [Pointer migration breaks Go literals/callers] → migrate repository consumers and document nil versus empty; run all affected modules/examples, not only root compilation.
- [Old persisted data lacks required denied approval or has contradictory fields] → reject before provider with indexed/contextual errors; normalize only the exact target terminal cases, never invent approvals or discard malformed fields silently.
- [Previously discarded title/metadata cannot be recovered] → additive decode remains compatible but migration cannot reconstruct lost history.
- [Shared chunk scalars cannot express every direct-literal zero-value presence] → bounded decode-presence support and explicit evidence boundary; no full producer optional-presence parity claim.
- [Over-validating dynamic/partial/error history prevents resumption] → source-specific state/schema gates and differential valid-legacy tests.
- [Schemas are absent or differ between TS/Go validators] → reuse configured JSON schemas, compare shared supported cases and report uncovered schema-family differences instead of inventing behavior.

## Migration Plan

Deliver one independently green root-module change with tests, docs and scoped codec support. Update pointer call sites in the same change; no new provider-domain types/dependencies or upstream pins are required. Document the proposed public APIs in godoc and persistence/resume migration guidance in the existing UI/tool guides without duplicating signatures. After merge, publish root through normal release workflow; later consumer adoption needs published-dependency checks, not workspace-only proof. Rollback may restore code but must not destructively rewrite stored JSON; older binaries can drop new fields, so avoid read/write downgrades on histories requiring retained state.

Required implementation gates: root tests, affected modules/examples, cross-language integration, parity-check and ordinary candidate-source CI. OpenSpec validation during planning proves artifact structure only, not behavioral parity. Update `PARITY.md` only for stable delivered evidence/support boundaries.

## Open Questions

No unresolved product decision is required to draft this plan. The concrete pointer migrations, additive APIs and decoded-chunk construction tradeoff are proposals for owner/design review, not shipped API approval. Reader error/lifecycle parity remains #181; optional metadata/data schema configuration and unrepresented provider-tool schemas remain explicit support/evidence boundaries, not scope silently added here.
