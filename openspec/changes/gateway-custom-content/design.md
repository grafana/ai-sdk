# Design

## Context

See proposal.md for the problem statement. Current state:

- **Schema and request goldens.** They already cover assistant custom parts (`kind` must contain a dot) and custom tool-result entries.
- **Request mapper.** It rejects the first as `custom-content` and the second as `tools`. The private `wirePart` has no `kind` field.
- **Output.** Unary and stream mapping reject custom output with the fixed internal error.
- **Go client.** `providers/grafana` already projects both request shapes. It fails on unary and stream custom output.
- **Native producers and consumers.**
  - OpenAI emits and consumes `openai.compaction`.
  - Anthropic emits and consumes `anthropic.fallback`, backed by imported upstream fixtures `fallback` and `fallback-mid-output`.
  - Logger, Agent Observability and Prometheus already treat custom parts as metadata-only.
- **Fallback.** The default fallback decider (`fallback.defaultDecider`) already returns true for any error that is not an APICallError. Combined failures are joined newest-first with `errors.Join`.
- **Gateway error mapping.** `safeErrorFromProvider` maps errors that are not API call errors to the retryable internal error.
- **Gateway command providers.** The command builds only Anthropic, OpenAI and OpenAI-compatible candidates. It does not configure Bedrock (#118), and it refuses requests that enable Anthropic server-side fallback (`internal/nativeoptions/options.go`). Request-scoped BYOK (#368) constructs only Anthropic and OpenAI models. So through the command, `anthropic.fallback` reaches callers only as caller-supplied history, and no served adapter produces the new rejection.
- **Native assistant custom handling.** OpenAI-compatible returns an error for assistant custom parts, and Bedrock Converse warns. Upstream skips them in both.
- **Baseline.** The registered upstream is `eb77f09e` (provider 4.0.21, gateway 4.0.103, anthropic 4.0.71, amazon-bedrock 5.0.105, openai 4.0.83, openai-compatible 3.0.62). The pinned Vercel Gateway client parses responses with `z.any()` and spreads unary content, so server output is the compatibility surface.

## Goals / Non-Goals

**Goals:**
- One generic custom codec per surface that any well-formed kind passes through, so the target upgrade's `anthropic.message_start` needs no Gateway change.
- Native adapters match registered upstream custom semantics, except where a decision below says otherwise.

**Non-Goals:**
- Implementing the target-only Anthropic `message_start` emitter or the server-side fallback beta header. Those belong to the follow-up upgrade PR.
- Converting other upstream throw sites from warnings to errors, or changing the AGENTS.md warning convention.
- Per-attempt failure transport (#322) or nested options on text tool-result entries (#108).
- Anthropic streaming `container_upload`, which upstream does not emit.
- openai-compatible multipart tool content (#362). The current JSON-stringify behavior already matches the upstream default.
- Enabling Anthropic server-side fallback through the Gateway. The existing refusal stays.
- Pre-existing native gaps found while planning, each tracked in `upstream-sync` issues instead of fixed here:
  - Anthropic tool_result cache control taken from the first options-bearing content entry;
  - Anthropic warnings for dropped tool-result file entries;
  - Anthropic request-wide MCP tool-result routing;
  - Bedrock assistant-role tool results.

## Decisions

### Generic custom DTOs, no kind inventory
**Decision.** Request mapping copies `kind` and options. Output DTOs are `{type, kind, providerMetadata?}`. Validation covers only:
- a non-empty, dotted kind of valid UTF-8;
- metadata under the shared gateway-provider-metadata budget and shape rules.

**Rationale.** Kinds are provider-owned; the registered type is `${string}.${string}`. An allowlist would repeat the current blocker on every new native block.

**Alternative rejected:** per-kind codecs. They need a Gateway release for every provider block and conflict with the opaque-metadata principle.

### Stream custom is atomic started content
**Decision.** A custom event is written immediately with its own complete-frame preflight.
- It sets the existing content-started flag, so later response metadata fails, as with sources.
- It does not touch text, reasoning or tool active state.
- It is valid with or without preceding response metadata.

**Rationale.**
- Upstream has no custom lifecycle.
- Target Anthropic emits `message_start` after response metadata, which stays valid.

**Alternative rejected:** allowing custom before response metadata without setting the flag. It would weaken the existing metadata-first ordering without upstream evidence.

### Tool-result custom entries carry only scoped options
**Decision.** Map custom tool-result entries to `ToolResultContentValue{Type: custom, ProviderOptions}` through the existing scoped-options mapper, so the reserved-namespace refusal still applies.

**Rationale.** Options are the entry's only payload; mapping the entry without them would be lossy.

**Ownership boundary.** Text-entry nested options remain refused and stay owned by #108.

### Anthropic alignment with registered upstream
- **Tool references.** A custom tool-result entry whose `anthropic.type` is `tool-reference` becomes a raw `param.Override` block `{type: tool_reference, tool_name}`. `toolName` is copied as raw JSON and omitted when absent, which mirrors upstream's `toolName!` byte for byte. The typed SDK param would force `"tool_name":""`. No tool-name mapping; Anthropic validates the field.
- **Other custom tool-result entries** are dropped with the `other` warning `unsupported custom tool content part`.
- **Fully filtered content.** When non-empty content loses every entry, Go sends `content: []`, as upstream's `filter(isNonNullable)` does, whether the dropped entries were custom or unsupported files. Upstream never pads this case. Go already sends `[]` for explicitly empty content. Upstream also warns for each dropped unsupported file entry, but Go drops them silently. That gap is pre-existing and tracked separately; tests must not claim file-drop warning parity.
- **MCP routing.** Go routes tool results whose IDs appear anywhere in the request's MCP calls to `mcp_tool_result`, which ignores content. Upstream tracks MCP IDs per assistant block. Custom handling applies on the `tool_result` path only; the routing difference is pre-existing and tracked separately.
- **Unknown assistant custom kinds** are skipped silently. This removes the Go-only `customContent` warning.
- **`container_upload`** blocks in unary responses become `anthropic.container_upload` custom content with `anthropic.fileId`, in block order. Streaming ignores them, as upstream does.

### OpenAI compaction replay presence
Go treats an empty compaction `itemId` as absent and requires `encryptedContent`. Upstream checks `id != null` and emits the compaction item without `encrypted_content` when it is absent. The Gateway now makes this path caller-reachable, so Go aligns: presence-aware `itemId`, optional encrypted content. The exported `OpenAIPartOptions.ItemID` string stays unchanged; the compaction branch checks raw presence locally, and the item is built with an override that omits `encrypted_content`, since the SDK constructor forces the field.

### Assistant custom history in other adapters
OpenAI-compatible and Bedrock Converse skip assistant custom parts silently, matching upstream. Without this, OpenAI compaction or Anthropic fallback history sent to a compatible candidate fails with a non-sentinel error. The Gateway maps that to a retryable 500, so clients would retry a deterministic failure.

### Bedrock rejects, upstream-compatible
**Decision.** On a custom tool-result entry, conversion returns `fmt.Errorf("bedrock: unsupported tool content part type: custom: %w", provider.ErrUnsupportedFunctionality)` before any request is built. Both DoGenerate and DoStream convert before I/O.

The rejection applies only on user/tool-role results and only when tools remain active after tool preparation. Upstream's chat model filters tool parts before conversion when prepared tools are empty, so it never throws there. Go now prepares tools first and filters tool content before conversion when `hasActiveTools` is false, as upstream does. So the rejection can sit directly in user/tool-role result conversion: filtered prompts never reach it, and `toolChoice: none` filters rather than rejects. Assistant-role tool results already fall to Go's generic warning; that gap is pre-existing and tracked separately.

**Rationale.** This is nara's decision for this site. Dropping the entry silently changes continuation semantics. The narrow scope keeps every other Bedrock warning unchanged.

**Existing Bedrock rejections.** Converse conversion errors that already exist and whose upstream counterpart throws UnsupportedFunctionalityError also wrap the sentinel: provider-reference files, non-S3 URLs, and unsupported media types. The existing Bedrock spec already calls these unsupported-functionality errors. Messages and timing stay the same; only classification changes. Warning sites stay warnings.

**Deviation recorded.** The new custom tool-result rejection is an explicit exception to the AGENTS.md warning convention. It is not a policy change.

### Root sentinel instead of a typed error
**Decision.** Add `provider.ErrUnsupportedFunctionality = errors.New("provider: unsupported functionality")` and wrap it with context at each use.

**Rationale.**
- The repository uses sentinels and `%w` wrapping, not custom error types.
- Upstream's `UnsupportedFunctionalityError.functionality` is carried in the wrapped message.

**Consequences.**
- Core retry already retries only retryable APICallErrors, so the sentinel is never retried.
- The default fallback decider already advances on non-APICallError errors. No fallback code change is needed; tests pin the behavior.

**Alternatives rejected.**
- A fake APICallError with status 400: there was no HTTP call.
- Stopping fallback on the sentinel: the rejection is specific to one candidate, and a later candidate may serve the content.

**Framing.** Advancing is a Go routing policy, not an upstream parity claim, because there is no public upstream fallback oracle.

### Gateway classification only when every failure is the rejection
**Decision.** In `safeErrorFromProvider`, after the existing cancellation and deadline checks, a recursive predicate walks the error tree:
- a leaf matching the sentinel counts;
- an `Unwrap() error` link recurses;
- an `Unwrap() []error` join requires every branch to qualify;
- anything else is false.

Only a true result maps to the existing fixed invalid-request document. Unary invocation and stream setup share this path.

The predicate runs last, after the existing `catalog.ErrUnsupportedRequest`, cancellation, deadline and APICallError cases, just before the internal-error default. Placement matters because `APICallError.Unwrap` returns its cause: an API error whose cause wraps the sentinel must keep API classification, as retry and the fallback decider do. An empty join does not qualify.

**Rationale.** `errors.Is` on a join matches any branch. Using it alone would turn "rejection plus a retryable 503" into a non-retryable 400 and hide an operational failure.

**Alternative rejected.** Classifying by last attempt only would change existing semantics.

**Reachability.** No command-served adapter produces the sentinel at this baseline, so handler tests use fake models returning the wrapped error. The classification is forward-compatible for #118 Bedrock configuration and for library consumers of `providerwire/v4`.

### Observability disposition
The existing per-capability mappings are the intended behavior for custom content; no new mapping is added.
- Agent Observability omits custom parts, because they have no representation on its wire.
- The logger records no custom kind, options or metadata under metadata-only capture. Stream custom parts keep only their type even with capture enabled.
- Prometheus counts custom as payload-bearing, which affects first-output timing.

Task 6.3 asserts each of these.

### Evidence without invented provider input
**Gateway codecs.** Covered by:
- Go handler/raw tests;
- the test-only `unary_success.json` and `stream_event.json` schemas, extended with a custom arm;
- a request golden family emitted by the real Vercel client and replayed through Go;
- TS workspace consumption and differential tests with fake models;
- authenticated command tests with native fakes.

**Command tests.** Native fakes may produce `anthropic.fallback` output. That evidence covers format conversion only, because the command refuses enabling Anthropic server-side fallback. Compaction round trips are production-reachable.

**Frontend.** No separate test is needed. Gateway output is ProviderWire, not UI chunks, and #112 doesn't change the UI chunk wire. Existing frontend tests already assemble custom UI parts: `test/integration/sse-message-assembly.test.ts`, `sse-wire-format.test.ts` and `simulated-stream.test.ts`, backed by `test/integration/testserver/scenario_anthropic_fallback.go`.

**Anthropic.**
- Fallback reuses the imported upstream fixtures.
- `tool_reference` and `container_upload` have no matching recording or upstream fixture. They get request-snapshot and synthetic HTTP unit tests labeled synthetic, and PARITY.md records the gap.
- No `input*.chunks.txt` is added or edited.

## Risks / Trade-offs

- **[Generic codec admits a future kind carrying sensitive data]** → Metadata stays caller-visible, as the native adapter returns it. Operator capture is metadata-only, and known credential protections still apply.
- **[Breaking change for direct Bedrock Converse callers that relied on the warning and drop]** → Call it out in the PR title/footer and in `docs/providers/bedrock.md`. The error text names the unsupported content type. A squash title marked breaking is attributed to every module the PR touches; in the alpha channel that only advances the prerelease counter.
- **[Caller-supplied compaction `itemId` becomes an item reference in the configured OpenAI account when the adapter stores responses]** → Accepted by nara for internal configured accounts, matching the reasoning `itemId` precedent (#280). Record it in the consumption audit. Under request-scoped BYOK (#368) the reference resolves in the caller's own OpenAI account, so the shared-account exposure applies only to configured accounts; the audit records both cases.
- **[Prometheus treats custom as payload-bearing]** → The upgrade's `message_start` will set time-to-first-output. Flagged to the upgrade PR; no change here.
- **[Mixed-failure predicate depends on the fallback error shape]** → It relies only on the standard `Unwrap` contracts. A test covers both orders and unknown-error mixes, and fallback's error shape stays unchanged.
- **[The Bedrock module consumes a new root symbol while its `go.mod` pin lags]** → Source CI builds the workspace candidate, which matches repo policy. Report the standalone gap and bump the pin before any independent Bedrock publication.
- **[Anthropic warning removal changes warnings returned to direct callers]** → This matches upstream. Tests assert the absence.

## Migration Plan

Ship as one PR, ordered by commit: root sentinel and fallback tests; Anthropic and Bedrock; Grafana client; Gateway codec and classification; docs and PARITY. The Gateway is built from the same-revision workspace (`go.gateway.work`). Rollback is a revert. No persisted state or configuration changes. The follow-up upgrade PR rebases onto this change and adds the native `message_start` emitter without touching Gateway codecs.
