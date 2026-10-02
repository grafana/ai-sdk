## Context

This is #280, stacked on `nrbrd/gw-fallback`/PR #328 at `44968b44`, which contains #318 native option forwarding and #319 mapped fallback. The implementation starting point is this active stack, not stopped #303/#309. Reconfirm the base and registered reference before applying; issue closure is not the test of whether prerequisite code is present.

The current server drops metadata on text, function calls, tool inputs/basic results, unary results and finish. `reasoning.go` projects a known continuation-key inventory; `sources.go` synthesizes numeric-only `citation` metadata. Independent Grafana decoding repeats reasoning projection and ignores other supported scopes. The root SDK already forwards much metadata into response history, but `chunk.go` preserves explicit empty metadata only on reasoning, and affected UI parts use `omitempty` maps. These are concrete presence risks to reproduce, not authority for a broad core refactor.

### Registered authority and evidence layers

Use `test/conformance/upstream.yaml`: ai `7.0.116`, Gateway `4.0.94`, Provider `4.0.18`, Anthropic `4.0.65`, OpenAI `4.0.78`, compatible `3.0.57`, React `4.0.119`, commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`. The local `/home/nara/src/ai` HEAD is different; source inspection uses `git show` at the registered commit, not its working tree. No baseline upgrade belongs here.

Inspected upstream sources establish:

- `packages/provider/src/shared/v4/shared-v4-provider-metadata.ts`: `Record<string, JSONObject>`; arbitrary namespace objects, not arbitrary namespace JSON primitives.
- `packages/provider/src/language-model/v4/`: result, text/reasoning, reasoning-file, source, tool-call/basic-result, tool-input start/delta/end and finish metadata positions. `response-metadata` and `stream-start` do not define an ordinary providerMetadata member.
- `packages/gateway/src/gateway-language-model.ts` and tests: permissive success parsing, unary spread with Gateway-hop request/response replacement, ordinary metadata independent from raw-event opt-in.
- `packages/ai/src/generate-text/stream-text.ts` and `to-response-messages.ts`: latest non-nullish object replacement; assembled content metadata becomes scoped providerOptions in subsequent history. Sources remain response-only and empty text is skipped by response-message conversion.
- `packages/ai/src/ui-message-stream/ui-message-chunks.ts`, `types/provider-metadata.ts`, and `ui/process-ui-message-stream.ts`: represented UI metadata positions and frontend replacement. ProviderWire tool-input delta/end metadata does not justify inventing UI fields for those chunks.
- Native Anthropic and OpenAI Responses prompt converters and tests: tool-call caller, thinking signature/redacted continuation, text item/phase and encrypted reasoning consumption. Pinned compatible chat conversion reads `[providerOptionsKey].thoughtSignature ?? google.thoughtSignature`; current Go production emits under the resolved configured namespace but consumes only `google`. The selected existing-supported witness uses configured `providerName: "google"`, not arbitrary compatible names.

`PARITY.md` classifies this as mixed Gateway runtime/client plus provider-boundary request proof and conditional core/UI/frontend work. Shape checks and supplied history are not proof of output-derived continuation. Existing reasoning history witnesses and #201's historical failures inform tests but do not replace the new baseline's acceptance evidence.

### Authentic fixture inventory

Gateway replay coverage uses unchanged Anthropic recorded simple-text, tool-call and thinking-tool-signature-roundtrip and OpenAI recorded reasoning-text inputs and direct UI/native-request expectations. Request comparisons use the existing conformance snapshot canonicalization, including equivalent Anthropic JSON-string and single-text-block tool results; metadata is not removed or projected for comparison. Other recorded thinking-tool-use/reasoning-tool-use cases remain candidates. The imported OpenAI upstream reasoning-encrypted-content fixture contains four complete Responses sequences concatenated into one mocked HTTP stream. It remains unchanged native-adapter decoding evidence, not an eligible single-response Gateway replay or proof of actual multi-request continuation. Authentic Gateway encrypted-reasoning continuation needs separately captured request/response steps; the focused synthetic native-boundary matrix proves continuation without claiming that authentic recording. Provider-executed web-search/tool-search/programmatic cases do not become supported because their inputs contain metadata. Compatible recorded simple-text, tool-call-delta/tool-call-parsable-prefix and upstream xai/placeholder cases exist, but none establishes a returned Google thought signature. The google continuation witness is therefore a focused synthetic native-boundary test, not authentic replay evidence. No authentic phase/caller/compatible-signature full matrix is claimed.

## Goals / Non-Goals

**Goals:**

- One service metadata policy/codec used at all currently supported registered response scopes, with independent Go decoding and no namespace/key inventory.
- Preserve absence, explicit empty objects, nested JSON values and event position; assemble replacement rather than merge.
- Bounded original-byte/count/UTF-8/JSON validation and complete response/frame encoding, with explicit adaptation failure and no silent omission.
- Real two-client first-output-to-next-native-request continuation, including configured fallback already delivered in this stack.
- Keep operator capture, consumer capture, authorization and caller response semantics separate.

**Non-Goals:**

- Native diagnostics, attempts/failures/discovery carriers, request routing, BYOK, optional raw parts or new tool/MCP/content codecs.
- #320 warning/source-display/response-identity restoration, provider bug fixes unrelated to demonstrated core/UI metadata loss, deep merging, streaming read-ahead, alternate fallback execution, retries after commitment or termination redesign.
- Full #201 provider matrix completion, live provider acceptance claims, or replication of Vercel's private service.

## Decisions

### 1. Preserve metadata only at registered supported positions

| Boundary | Metadata positions delivered here |
| --- | --- |
| Unary | GenerateResult.providerMetadata; supported text, reasoning, reasoning-file, source and client-executed function-call content |
| Streaming | Text and reasoning start/delta/end; reasoning-file; source; tool-input start/delta/end; supported function call and correlated basic result; finish |
| Core/UI | Existing represented text/reasoning, tool-call/result, reasoning-file and source fields, only where continuation/presence evidence demonstrates loss |

Add optional metadata fields to existing private DTOs. Keep nil absent and non-nil empty maps present using presence-aware representation. Transport maps as opaque namespace JSON objects; no renamed namespaces or per-native-field scalar parsing. Nested null, false, zero, empty strings, arrays and objects survive semantically. JSON key ordering and insignificant whitespace are not a wire guarantee.

A present top-level null, null namespace, array or scalar is not registered object metadata. Reject these explicitly in strict server mapping and the independent client. Pinned Gateway permissiveness for malformed responses is not the server's shape contract; document the Go client's strict malformed-input adaptation rather than calling it identical parsing behavior.

Do not add metadata to response-metadata/start/error/raw or enable unsupported output unions. Scalar identity/display and warnings remain owned by #320/PR #325. This base currently rewrites stream model identity, source IDs and file_path display; those facts are not renewed product policy. Leave scalar code unchanged while working on this base, but on rebase/merge preserve #325's native response/source identity, display/filename and warning changes and reconcile copied requirement blocks and tests rather than restoring suppression. The pinned client combines server warnings with client warnings; it replaces request/response, not warnings. If the current file_path display rule needs a narrow discriminator inspection, keep it separate from metadata transport and remove that temporary inspection when #320 supersedes the rule. No merge-first prerequisite or reciprocal dependency is introduced.

Alternative rejected: more continuation keys in the existing allowlists. That still loses future native semantics and duplicates policy across families and clients.

### 2. One bounded server transport seam; independent client validation

Use `provider.ProviderMetadata` directly in service-owned unary, stream, reasoning-file, source and tool DTOs, with `omitzero` preserving nil versus explicit empty maps. Keep original-byte accounting and UTF-8/object-shape validation in the existing response/event/source preflight paths, without metadata mapping helpers or recursive projection. Standard JSON encoding validates namespace JSON syntax before any response/frame is written.

Before JSON scanning or copies, perform subtraction-based aggregate accounting for namespace key bytes, original raw namespace bytes (including whitespace), one work-budget unit per namespace and other output values. Unary accounting is shared across result metadata and every content part, not a fresh budget for each part. Streaming accounting covers the complete event, including metadata and existing content/result fields. Reject excessive content/map cardinality before allocating mapped slices/maps; nested validation work is bounded by already-accepted raw bytes. After aggregate accounting, preflight checks namespace names and raw values for valid UTF-8 and object shape. Standard JSON encoding rejects malformed namespace syntax. Final encoded-size checks account for JSON punctuation and escaping, rather than estimating punctuation during preflight. Valid JSON surrogate escapes remain valid; do not silently repair invalid original bytes.

Use the existing UnaryResponseBytes and StreamFrameBytes budgets, not family-specific known-key byte caps or an invented transport protocol. Remove the source projection's standalone recognized-namespace cap in favor of unified accounting. Existing client UnaryBytes, StreamEventBytes, StreamBytes and StreamEvents remain the enclosing limits; independently bound decoded metadata counts/allocations by the original accepted document. Keep `usage.raw`'s separate delivered cap/semantics unchanged.

After preflight/validation, standard Go JSON encoding of private DTOs checks final unary bytes and complete SSE bytes including `data: ` and `\n\n`. Escaping can increase size; only complete fitting documents/events are written. Constant-factor bounded allocations remain the existing encoding trade-off. Provider marshalers do not control public DTOs.

The Apache client implements the object/presence checks locally without importing Gateway implementation helpers, validators or schemas. It retains metadata across all supported decoder branches and top-level unary results. No new dependency or public configuration API is needed.

Alternatives rejected: one per-family projection, sharing AGPL code into the Apache client, recursive generic sanitization, or relying on permissive TS parsing as resource/shape validation.

### 3. Adaptation failures use existing commitment and ownership

Invalid/oversized unary metadata fails the entire response before HTTP 200 using the existing fixed adaptation error. After streaming commitment, reject the invalid event before writing it, cancel work, and use the existing bounded synthetic terminal error when writable. Never fall back to metadata-free success, selectively drop bad namespaces or replay a selected candidate. Metadata validation does not change stream-start/error commitment, IDs, part counts, idle deadlines, authoritative finish, cancellation or single-owner bounded drain.

Alternative rejected: restart fallback after metadata adaptation fails. A generation may already be paid or have effects; the selected result/stream is already committed under the delivered contract.

### 4. Actual continuation is the acceptance gate

First add failing focused/differential witnesses, then implement. Use real handler and authenticated-command calls backed by fake native HTTP endpoints, and assert the actual second native request. Separate API paths explicitly:

| Client/path | Execution and continuation proof |
| --- | --- |
| TypeScript unary | High-level `generateText` uses Gateway `doGenerate`; reuse its returned response messages. |
| TypeScript streaming | High-level `streamText` uses Gateway `doStream`; reuse its assembled history/local tool loop. |
| Go unary | Call Grafana `DoGenerate` directly. A test-only adaptation maps actual returned supported GenerateContentPart fields into provider.ContentPart, wraps each returned namespace unchanged as RawProviderOption, and calls the existing public ToResponseMessages. Append only the ordinary consumer-owned tool result when needed, then issue the second DoGenerate. Test this adapter's absence/empty/call/result semantics independently and label it as a unary consumer adaptation, not native Go high-level orchestration. |
| Go high-level | StreamText and GenerateText both use DoStream; GenerateText collects StreamText in generatetext.go. Reuse actual Response.Messages or the automatic local-tool loop and count this only as streaming HTTP evidence. |

Assert observed HTTP streaming selectors/native stream flags and invocation counts in each path. The unary test adaptation must derive all metadata from the decoded first output; it must not consult expected-value constants, rewrite namespaces, fill missing metadata or add a public Go orchestration API. Expected values belong only in assertions against the producer output/second request.

Cover direct and selected configured-fallback paths in each applicable row, including an eligible pre-commit failure and no replay after a selected response fails encoding. Reuse command harness and existing #318/#319 native request chains instead of adding another executor. Require candidate-specific namespace consumption and no cross-request mutation. Primary restart on each new continuation request remains the existing stateless behavior.

Representative required witnesses: Anthropic client-executed tool-call caller and thinking signature/redacted continuation; OpenAI text item ID/phase with stored and reconstructed history as supported, and final encrypted reasoning. The compatible witness is a local function call returned with extra_content.google.thought_signature on a configured providerName of google: the producer emits google.thoughtSignature, assembly retains that namespace and the next native call emits extra_content.google.thought_signature. Streaming must supply the signature in the initial tool-call delta before the current Go producer starts the call; late signature support is not claimed.

A temporary synthetic native-adapter planning probe verified that google path in both modes: direct DoGenerate output through the described unary adaptation/ToResponseMessages and a real StreamText automatic local-tool loop each produced two native requests and retained the first response's signature in the second. This is source-workspace native-adapter feasibility evidence, not Gateway acceptance, an authentic recording, a pinned TS execution result or live-provider parity. Both-client handler/command unary/stream/fallback regression tests remain required after the metadata fix.

For other configured compatible namespaces, opaque transport preserves the returned namespace but cannot repair the Go consumer's google-only lookup. That broader producer/consumer parity gap remains separately owned; no Gateway metadata rewriting or native-adapter fix is absorbed here. Azure/future namespaces get opaque transport witnesses even when no configured native continuation route is available. Ordinary tool-role result caller consumption remains a separately documented native-adapter boundary; metadata transport cannot fix that provider bug or activate programmatic provider tools.

Scope inventory tests cover every supported metadata placement even if a native provider does not naturally emit every one. Such synthetic fixtures stay focused tests. Actual continuation must not be reduced to supplied prompt history, a decoded snapshot, hand-assembled metadata or a transport echo.

For a demonstrated UI gap, add deterministic scenarios in `test/integration/testserver/` and Vitest using `parseJsonEventStream` and `uiMessageChunkSchema`, then assert assembled messages and ConvertToModelMessages output. Preserve explicit empty replacement on affected chunks/parts; omit metadata on UI variants that do not represent it. Fix only the serialization/assembly/conversion boundary proven to fail.

### 5. Response data is not an observability or authorization policy

Do not consult server capture flags to project responses. Operator metadata-only telemetry continues to omit provider payloads and high-cardinality continuation fields. Consumer middleware around `providers/grafana` may independently observe returned metadata under its own configuration without enabling operator capture.

The Gateway must not add credentials, configuration dumps, native request/response transports or another tenant's state to providerMetadata. Retain protections at the concrete source/field where credentials and tenancy are known; exercise configured authentication secrets and concurrent tenant/request isolation. Do not introduce arbitrary token-pattern or field-spelling redaction of opaque application metadata. A concrete unexpected credential-bearing metadata producer is a blocker to handle specifically, not a reason to silently strip unknown namespaces or invent a generic redactor.

Returned provider fields, including names resembling gateway controls, do not authorize destinations/accounts and are not parsed as service-owned routing/evidence. #321–#324 can reuse this transport only after approving their own representations; this change defines none.

### 6. Proof and module boundaries ship together

Keep all Gateway codecs, schemas, fake-native command/contract tests under `ai-gateway/` (AGPL). Independent client and generic root/UI tests remain Apache. Reuse unchanged authentic recorded/imported simple-text/tool/reasoning inputs where available and compare Gateway output against existing direct UI/native-request expectations. Synthetic resource/malformed/transport scenarios remain focused tests, never recorded or upstream fixture inputs.

Classify remaining differences as Go adaptation, intentional deviation, implementation bug or coverage gap. Remove superseded metadata-projection deviations from stable coverage documentation without removing unrelated scalar/display boundaries. Report #201 full-matrix and missing native recordings separately. Source-workspace tests and standalone published module tests establish different things and must be reported separately.

## Risks / Trade-offs

- [Opaque metadata increases caller-visible data] → Bound it, preserve concrete credential/tenant boundaries and keep operator exports independent; do not tunnel transport/config into metadata.
- [Whitespace, many entries or JSON escaping defeats per-value checks] → Aggregate original-byte/cardinality preflight plus final complete-document/frame checks and below/at/above witnesses.
- [Explicit empty maps become omission] → Presence-aware DTOs and demonstrated SDK/UI serialization fixes with pinned frontend assembly tests.
- [ProviderWire and UI represent different tool-input scopes] → Test each boundary independently; never invent UI schema fields to claim universal propagation.
- [Malformed TS responses parse permissively] → Test strict raw server/schema output and independent Go decoder failures; record the malformed-input Go adaptation explicitly.
- [Native provider mismatch masquerades as Gateway continuation failure] → Capture first producer output and second actual native request; scope unrelated native gaps separately instead of weakening history assertions.
- [#320/PR #325 parallel work touches source/response DTOs] → Change only metadata semantics; explicitly reconcile full copied requirement blocks, schemas and assertions on rebase/merge, preserving #325's warnings, native identities and source display/filename semantics without restoring current-base rewrites or creating a dependency cycle.
- [Workspace success is mistaken for published consumption or live parity] → Run/report applicable standalone checks separately and retain provenance/coverage limits.

## Migration Plan

1. Add failing tests at the current registered baseline, implement shared server transport and independent decoding, then bounded core/UI fixes justified by evidence.
2. Update schemas, spec deltas and centralized client/operator guides with opaque metadata and malformed-output behavior; no legacy codec or compatibility flag remains.
3. Validate candidate-source server/client/frontend/parity gates and applicable standalone modules. Keep the PR based on the current #319 stack until rebased after prerequisite merges. If #320/PR #325 integrates before or during this work, reconcile the metadata deltas with its warning/native-identity/source-display requirements and tests, retain its implementation and rerun affected gates; do not require #320 to merge first or restore this base's scalar suppression.
4. Deploy client/server changes through normal module and Gateway release paths. Stock pinned TS clients already retain registered metadata. A server rollback restores lossy projection and can break continuation; avoid mixed-version claims that an older Go client preserves newly exposed scopes.

## Open Questions

- The compatible witness is resolved to providerName google with an initial tool-call thought signature. If required Gateway/TS execution exposes another blocker on that exact path, stop and ask before choosing a substitute, rewriting metadata or widening native-adapter scope; arbitrary configured-name and late-signature support remain unclaimed.
- Which authentic existing fixtures exercise supported metadata without deferred provider-tool codecs? Inventory available cases before implementation and report absent provider-boundary evidence; do not manufacture it.
- Are additional UI assembly gaps found beyond empty-map serialization? Each requires a pinned frontend reproduction before adding a core fix; unrelated provider or tool-family gaps remain separately owned.
