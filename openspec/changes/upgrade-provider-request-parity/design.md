## Context

This is PR 1, provider request validity and model selection, from `~/src/ai-sdk/test/conformance/UPGRADE_PLAN.md`. The observable boundary is `provider.CallOptions` to provider HTTP requests and conversion warnings/errors, for both generate and stream calls. Response/history reconstruction belongs to PR 2; enforcement against returned tool calls belongs to PR 3.

The registered baseline remains `test/conformance/upstream.yaml` at `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. Source was inspected using Git objects in `/home/nara/src/ai`, not its working checkout:

| Package | Registered source | Fixed target source |
| --- | --- | --- |
| Anthropic | 4.0.38 at registered commit; matching tag unavailable locally, package manifest verified | `@ai-sdk/anthropic@4.0.57` |
| OpenAI | `@ai-sdk/openai@4.0.41`, manifest verified | `@ai-sdk/openai@4.0.70` |
| Bedrock | `@ai-sdk/amazon-bedrock@5.0.55`, manifest verified | `@ai-sdk/amazon-bedrock@5.0.87` |

All three target tags resolve to `b3033f77f0459bcc68df182c8052f52305467bcc`. The old OpenAI and Bedrock tags resolve to `a7f0d72ab8b64ccf81bcfbc8938a10aa7d4aee26`. This is an explicit upgrade comparison, not evidence that target behavior already belongs to the registered baseline. The plan's full target set is ai 7.0.106, Anthropic 4.0.57, Bedrock 5.0.87, Gateway 4.0.86, OpenAI 4.0.70, OpenAI-compatible 3.0.52, provider 4.0.17, provider-utils 5.0.44, and React 4.0.109. Candidate installation retained the 4320-minute release-age gate. Execution used this fixed set, not a new latest-version selection: target generation passed, but replay retains 14 later-capability failures. The current registered-baseline parity check passed. `evidence.md` summarizes validation and residual ownership; raw candidate artifacts are preserved outside the repository.

### Source comparison and initial delta ledger

| Behavior | Current Go / old behavior | Target evidence and disposition |
| --- | --- | --- |
| Vertex Claude 4 capabilities | `models.go` only catches `claude-*-4-`; tests currently expect future defaults for `@20250514` | `anthropic-language-model.ts` and tests, starting commit `65397d7`: recognize Sonnet 64k and Opus 32k. Required upstream correction. |
| Allowed tools | `applyToolChoice` emits every entry as a function and ignores an explicitly empty list | `responses/openai-responses-prepare-tools.ts` and tests, `a062795`: declaration-aware identities, warnings, rejection when no entry survives. Required upstream correction; empty-list handling is also an existing Go gap. |
| Document names | `buildDocumentBlock` only strips an extension, with fallback only for missing filenames | `convert-to-amazon-bedrock-chat-messages.ts` and tests, `770c214`: sanitation and post-sanitation fallback at both sites. Required upstream correction. |
| Strict tools | `prepareTools` gates on model only | `amazon-bedrock-prepare-tools.ts` and tests, `d746e16`: recursive closed-object check and warning, no schema rewrite. Required upstream correction. |
| OpenAI request schema sanitation | Tool input/output and response schemas are forwarded without target normalization | `normalize-openai-json-schema.ts`, its tests, and request/tool call sites: remove string `propertyNames`, warn, reject other forms, preserve originals. Required correction within existing request settings. |
| Bedrock OpenAI reasoning | `isOpenAIModel` uses a loose substring; `applyNonAnthropicEffort` always emits flat `reasoning_effort` | `amazon-bedrock-chat-language-model.ts` and generate/stream tests: exact native or one-prefix OpenAI identity, GPT-OSS flat versus other OpenAI nested effort. Required correction within existing reasoning settings. |
| Anthropic schema sanitation | Existing shared `internal/anthropicschema` implementation and tests | No `sanitize-json-schema.ts` delta between registered and target source. Preserve; no new sanitizer port justified. |
| Budget-based Bedrock profile inference | Go's ID-only helper misses opaque application profile ARNs despite an explicit reasoning budget | Target `amazon-bedrock-anthropic-model-support.ts`, chat model/tests. Approved existing-setting correction; constructor `modelFamily` remains planner-owned pending. |
| Anthropic thinking/service options | Existing `ThinkingConfig` does not expose target binding controls; target adds display-update beta behavior and service tier | Target `anthropic-language-model.ts` and options/tests. Assess existing-setting corrections separately from new API and recovery-associated behavior; obtain owner decision before adding or deferring. |
| Bedrock default output route | Go shares native-output and strict rejection tables; Sonnet 4.6/Haiku 4.5 still use native output | Target support tables/chat-model tests default these models to JSON tools while preserving strict support. Approved model-selection correction. |
| Bedrock parallel-tool control | Existing Anthropic typed flag is ignored by Bedrock | Target chat-model/prepare-tools tests use Anthropic additional-field choice without duplicate Converse choice. Approved forwarding correction, including synthetic JSON tools. |
| Explicit output selection | Target introduces Bedrock structuredOutputMode and namespace precedence | Planner-owned pending API decision; not a dependency of default routing. |

Other target additions (GPT-6 configuration/async capabilities and new Anthropic web tools) remain explicit separate-capability decisions in the upgrade plan. Response parsing, metadata, replay, and provider-owned stream errors remain PR 2 work. These are the initial source findings; subsequent regression results are summarized in `evidence.md`.

`PARITY.md` classifies provider request conversion as automated for existing request goldens, but unsupported-option warnings as manual/source-and-unit coverage. Mantle has focused adapter tests and no provenance-valid live recording. This change must extend request evidence without overstating those boundaries.

## Goals / Non-Goals

**Goals:**
- Correct the nine request behaviors approved by Nara in the revised upgrade plan, using existing settings and Go error/warning conventions.
- Preserve valid controls, input immutability, request ordering, and model identity.
- Deliver red-first regressions, exact-target candidate request evidence, and a precise PR 7 snapshot handoff.
- Keep unrelated upgrade-wide API allocation planner-owned and pending; escalate only a concrete dependency or a new capability required by these nine corrections.

**Non-Goals:**
- Canonical baseline/pin/lockfile upgrade, final certification, or a new dual-baseline framework.
- Response conversion, history reconstruction, tool execution policy, UI chunks, new provider families, discovery, or recovery scheduling.
- New profile, thinking, structured-output, async, or configuration-update APIs without a revised approved scope.
- Fixing unrelated stale OpenSpec requirements or importing closed PR #66 wholesale.

## Decisions

### 1. Keep policy in existing provider request builders

Extend the current model lookup and common generate/stream conversion paths. For dated Vertex Sonnet/Opus 4, recognize `@` alongside `-` after the family name, after more-specific model checks. Preserve `ModelID` and the Vertex transport mapping. The request tests must replace assertions that encode the current erroneous future-model fallback, not just add lookup tests.

Reject a new generic capability registry: the correction is provider-local, and such a registry would add unrelated API and coordination work.

### 2. Resolve allowed tools from prepared declarations

Build call-local resolution information while preparing the actual OpenAI tools. Track user-declared names separately from canonical provider aliases; compare resolved wire identities to detect real ambiguity. Preserve requested entry order and duplicates and keep the full declaration array for prompt caching.

Function/custom entries carry names, MCP entries carry `server_label`, and supported built-ins carry only their type. Namespaced/deferred functions and native tool search cannot be allow-listed. Unknown names keep the upstream function fallback with a warning; direct-name collisions prefer the direct declaration and warn; ambiguous canonical aliases are dropped with a warning. Provider-option validation rejects an explicitly empty list or invalid mode before tool preparation, even with no declared tools. A list reduced to zero entries also returns a contextual Go error before HTTP, never an unrestricted call. With valid options, no-tools calls retain the target early return without a tool choice. The exact-target probe confirmed that option-schema validation precedes the helper's early return.

Allow conversion to return warnings and errors through the existing build path; do not introduce custom error classes or defer local rejection to the provider. Avoid reconstructing identities solely from `toolNameMapping`, which cannot encode MCP server labels or ambiguity. Ordinary named-tool choice semantics and response-side alias dispatch are not being redesigned.

### 3. Share Bedrock naming and inspect strict schemas without mutation

Use `buildDocumentBlock` as the shared naming boundary for input and tool-result documents. Match target order: strip extension segments starting at the first dot (the target provider-utils helper), collapse ECMAScript whitespace, remove characters outside ASCII letters/digits, spaces, hyphens, parentheses and square brackets, trim, truncate to 200, trim again, then allocate `document-N` only if empty. Retain one request-wide fallback counter. Do not transliterate or add collision suffixes absent upstream.

The strict compatibility check is a private read-only traversal of decoded JSON schemas. Match the target's schema-bearing keywords (properties, pattern properties, definitions/$defs, schema dependencies, propertyNames, contains, not/if/then/else, items/tuple items, and allOf/anyOf/oneOf). Object types, including type arrays containing object, require `additionalProperties: false`. Boolean schemas are compatible; instance values in defaults/examples are not schemas. Do not invent remote-reference resolution or a broader JSON Schema validator. Unsupported-model warnings take precedence; false and absent strict values retain their current semantics.

Reject automatic schema closure: it changes caller meaning. The target drops strict, not schema properties.

### 4. Keep OpenAI schema normalization separate from Bedrock strict checks

Use one private, non-mutating OpenAI normalizer for response-format schemas and function input/output schemas, including namespaced functions. Copy only schema-bearing structures, remove `propertyNames` only when its schema explicitly has `type: string`, and issue the target compatibility warning. Reject boolean, missing-type, and non-string `propertyNames` before transport. Preserve boolean subschemas, dependency name arrays, and non-schema data.

Reject sharing this with Bedrock strict validation or the Anthropic sanitizer: they have different provider semantics and warning contracts. Client-side schema validation remains unchanged.

### 5. Route existing Bedrock effort by actual model family

Recognize `openai.*` and a single dot-delimited prefix followed by `openai.*`, not arbitrary substrings. GPT-OSS retains flat effort; other OpenAI models receive `reasoning.effort`, preserving other caller-provided nested reasoning fields. Keep Anthropic and other-provider request shapes unchanged. Test both existing root reasoning and provider `reasoningConfig` paths. Profile-family overrides are not necessary for this bounded correction and remain gated.

### 6. Apply request-aware Anthropic profile classification

Infer application-inference-profile ARNs from explicit non-null budget presence, not a positive-value test. Preserve raw zero-versus-absent internally while retaining typed Go zero-as-omitted semantics; do not migrate public fields to pointers. The request option schema rejects an explicitly null budget before classification (the lower-level helper's non-null check does not make null a valid provider option). Respect modern/legacy namespace precedence. Match upstream's classification stages: initial explicit budget before root reasoning resolution, effective budget passed to tool preparation afterward. Thread request-aware classification through reasoning derivation, thinking/budget adjustment, sampling removal, output routing, tool conversion, non-Anthropic effort exclusion, and response-field paths. Do not change model identity or infer a family for opaque profiles without budget. New constructor overrides are not needed.

### 7. Compose default JSON-tool routing with parallel-tool disabling

Separate native-output reliability from strict-tool support: Sonnet 4.6 and Haiku 4.5 default to JSON tools even with thinking, while strict support is unchanged. Reuse existing unary/stream synthetic-tool collapse, finish mapping, and metadata; verify actual-model calls rather than only helper tests. Preserve other model defaults.

Determine the output route and append the synthetic JSON tool before common tool preparation. Pass the effective required choice and existing `anthropic.disableParallelToolUse` into preparation so synthetic-only and mixed tool sets receive one correct choice. Decode a private Anthropic option projection in Bedrock without importing its separate module. For eligible Anthropic calls, emit additional-field `tool_choice` with `disable_parallel_tool_use`, omit duplicate Converse choice, and preserve false/unset/no-tools/none/non-Anthropic controls. This does not add an explicit output-mode API or change local execution policy.

### 8. Separate capability proof from baseline certification

Prefer authentic existing or exact-version upstream inputs when a request scenario can be represented in conformance; add the scenario and observe failure first. For unsupported cases and warnings/errors, use focused synthetic unit/HTTP capture tests explicitly labeled as such, never synthetic provider recordings.

Run candidate generation/replay early in a disposable integration worktree using the fixed coherent target set. Preserve temporary probes, raw results and snapshot diffs outside the repository; keep only concise validation/provenance and scenario ownership in the change and PR description. Do not write target expectations beneath old-baseline provenance. PR 1 owns scenario design, generators/configuration where applicable, focused tests, and executed comparisons; PR 7 owns canonical target snapshots and pins.

Inventory affected `expected-requests.jsonl` and any warning/output snapshots, identifying unchanged goldens as well as deltas and missing authentic fixtures. Run current-baseline `mise run parity-check` too. Explain expected version-related failures by scenario, cause, and PR 7 resolution; unexplained failures block review. No check weakening or compatibility switches. No frontend integration addition is expected for request-only changes; if observable UI wire behavior changes, stop and reassess the boundary and cross-language coverage.

## Risks / Trade-offs

- Ahead-of-baseline behavior may fail enforced old goldens → record exact failures; do not merge independently unless checks pass and deviations are documented.
- Ambiguous names could silently broaden tool access → preserve target rejection/drop policy and assert no HTTP request for empty surviving selections.
- Recursive schema traversal could mutate caller data or treat ordinary objects as schemas → non-mutation and schema-location controls, plus unchanged valid requests.
- Source-only warning tests could be mistaken for provider provenance → label focused synthetic tests and document missing live/upstream inputs; never alter recorded chunks.
- Broader remaining delta could require new APIs → unrelated decisions remain planner-owned pending, not accepted deferrals or PR 7 work; escalate only actual dependencies of the approved nine.
- Shared OpenAI adapter changes could break Mantle → capture generate and stream requests through Mantle and assert identity/routing remain intact.

## Migration Plan

No data migration or public API migration is planned. Implement incrementally with tests, then review this scoped PR against its predecessor. Reconcile durable ahead-of-baseline deviations/coverage gaps in `PARITY.md` or `upstream.yaml` without changing the registered package set. Keep the detailed delta ledger and candidate artifact links in the PR description.

Hand off request helper contracts to PR 2 without changing its response/history ownership, and request selection representation to PR 3 without taking execution-policy ownership. Hand PR 7 scenario IDs, target source versions, candidate request deltas, coverage gaps, and current-baseline failure explanations. Archive the single active OpenSpec change before merge. If old-baseline checks cannot pass honestly, preserve scoped review and land the validated cumulative stack atomically under the plan. Rollback reverts behavior and its associated tests/evidence together; never leave stale enforced snapshots.

## Open Decisions and Resolved Checks

- Planner-owned pending decisions: modelFamily, explicit structuredOutputMode, Anthropic display=updates/blockBinding/serviceTier, mid-conversation clearAt/effort, and Bedrock guard-content assessment. Nara approved the nine corrections independently; these decisions do not block them and are not accepted coverage gaps. See `evidence.md` for the approval and dependency analysis.
- Authentic fixture inventory and full candidate replay are complete. New request edges lack authentic inputs, so focused red-to-green tests and 442 exact-target mock-transport probes provide bounded evidence; no provider input was fabricated.
- No old-baseline goldens conflict: registered checks pass unchanged. The complete target replay has 14 unique failures owned at field level by PR 2H, PR 2S and PR 6; PR 7 owns final expectations/certification. See `evidence.md` for the corrected allocation and review limitations.
