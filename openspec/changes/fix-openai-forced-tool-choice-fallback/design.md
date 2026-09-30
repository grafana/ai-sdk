## Context

This is a provider-implementation bug and forced-choice evidence gap, not a baseline upgrade or intentional deviation. At Go `ec204b9164e9c7bdc74cb0cc416d55c0dedaffe1`, `providers/openai/prepare_tools.go:490-543` resolves aliases but `hostedToolChoiceType` searches every value in `providerToolNames`. Consequently shell, local-shell, and tool-search selections produce hosted choices; `TestPrepareTools_ProviderToolChoiceVariants` even expects the incorrect shell shape (`prepare_tools_test.go:553-557`).

The registered reference is `@ai-sdk/openai@4.0.72` at aggregate commit `4e8c387622ee1bb0d55841664416d38754d5c9a3` (`test/conformance/upstream.yaml:1-16`). The exact package tag resolves to `fe37eb0f67c1028c4bed0f4640ae17c57145a818`; its `packages/openai/src/responses/openai-responses-prepare-tools.ts`, associated test file, and package manifest are identical to the registered commit. Source lines 539-561 resolve aliases then use a nine-kind hosted allowlist, followed by custom selection and function fallback. All three issue kinds still fall through to functions. The issue's older 4.0.25 target is not used for implementation.

Matching upstream tests cover custom choice (1843-1859), apply-patch (1928-1950), and programmatic alias choice (2877-2886), plus hosted controls elsewhere. They do not directly test the three ordinary forced fallback cases; source establishes their required semantics. Existing `test/conformance/openai/upstream/{shell,local-shell-continuation,tool-search}` requests use auto choice and do not supply forced-path proof. `test/conformance/PARITY.md` classifies provider adapters as mixed coverage and distinguishes synthetic requests from provider recordings.

## Goals / Non-Goals

**Goals:** Match ordinary forced-choice JSON for canonical names and configured aliases of all three tools; preserve other existing choice shapes and independent allowed-tools resolution; add deterministic, pinned-upstream request evidence before fixing the serializer.

**Non-Goals:** Change public APIs, canonical-name mappings, declaration/response conversion, alias collision policy, custom-choice precedence, allowed-tools behavior, unsupported-choice validation, model capabilities, SSE/frontend behavior, other providers, or the registered baseline. Do not create or alter conformance provider inputs, expand the conformance harness, or claim live OpenAI acceptance.

## Decisions

### 1. Separate ordinary forced hosted classification from canonical mapping

Replace the membership scan in `hostedToolChoiceType` with an explicit selection of the nine upstream hosted kinds: `code_interpreter`, `file_search`, `image_generation`, `web_search_preview`, `web_search`, `mcp`, `apply_patch`, `computer`, and `programmatic_tool_calling`. Retain existing typed SDK constructors for apply-patch and programmatic choices and remove the now-unreachable shell-specific forced constructor branch. All other resolved names use the existing `ToolChoiceFunctionParam` path. Preserve the existing custom-provider choice handling; do not refactor its precedence as part of this issue.

`providerToolNames` in `providers/openai/tool_name_mapping.go:5-18` must remain complete: shell, local-shell, and tool-search are still declarations and need bidirectional canonical/alias mapping. Deleting entries there would fix classification at the cost of breaking alias resolution and response reconstruction. An explicit allowlist is preferred to blacklisting only these three names, because recognized declaration kinds are not automatically eligible for ordinary hosted choices.

For each declared provider tool, select either its canonical name or its configured alias. Expected ordinary choices are exactly:

| Provider ID | Example alias | Canonical selection | Serialized choice for either selection |
| --- | --- | --- | --- |
| `openai.shell` | `terminal` | `shell` | `{"type":"function","name":"shell"}` |
| `openai.local_shell` | `localTerminal` | `local_shell` | `{"type":"function","name":"local_shell"}` |
| `openai.tool_search` | `discover` | `tool_search` | `{"type":"function","name":"tool_search"}` |

The provider ID is a declaration identifier, not a new accepted spelling of `ToolChoice.ToolName`. Do not add validation or errors for arbitrary unmapped names; they retain the existing identity/function fallback.

### 2. Keep allowedTools and declaration shapes independent

`prepareTools` returns through `applyAllowedTools` before ordinary `applyToolChoice` (`prepare_tools.go:120-126`). This path resolves emitted declarations and has its own supported kinds. Shell/local-shell remain type-only hosted entries there, and tool-search remains unsupported, warned, and dropped; empty surviving selections still fail before HTTP. The archived `openai-allowed-tools-declaration-resolution` change explicitly reserved ordinary choices for #32. No reuse of the new ordinary allowlist in `allowed_tools.go` is permitted.

Retain no-tools omission and string choices (`auto`, `none`, `required`). Test whole `tool_choice` objects, not only discriminators, to catch wrong or extra `name` fields, and assert affected declarations remain their original hosted kinds. Preserve all nine hosted choices, custom named choice, ordinary/unmapped function choices, and existing allowed-tools tests as controls.

### 3. Establish focused request goldens without new provider fixtures

Add table-driven focused tests in `prepare_tools_test.go` and a provider-module HTTP snapshot test (for example `forced_tool_choice_test.go`) using the existing `roundTripFunc` pattern from `TestPrepareTools_AllowedToolsRequestModes` (`allowed_tools_test.go:251-291`). Capture actual encoded request bodies from `NewResponses` through both `DoGenerate` and `DoStream`; return minimal synthetic success responses and drain streams. Cover three kinds × canonical/alias selections × unary/streaming modes. Compare decoded JSON semantically, preserving array order; assert exact choice, unchanged declarations, and the expected streaming-mode field. Snapshot bodies exclude credentials/headers and retain the meaningful streaming-mode distinction.

Store request expectations under `providers/openai/testdata/forced_tool_choice/`. Before changing Go, obtain expectations from the installed, exactly pinned TypeScript provider via low-level `doGenerate`/`doStream` and a fake local transport using the same model (`gpt-4o`), prompt (`hi`), provider declarations/args, selections, and options. Record a reproducible capture recipe alongside the goldens: script or complete executable instructions, package/tag and aggregate commit, exact inputs, normalization policy, regeneration command, and synthetic-evidence designation. Use the existing pinned Node environment at `test/conformance/tools/package.json`; no new dependency pins or general-purpose harness are needed. Ensure package resolution uses that environment even if the capture script is colocated with provider testdata.

The upstream-captured `tools`/`tool_choice` request projection is the mandatory differential contract for this issue; additional full-body fields can be retained in the goldens, but unrelated serializer differences must be documented rather than silently normalized or repaired in this work package. Ordinary JSON key order is immaterial; no normalization may rewrite tool names, types, array order, or choice fields. Do not generate expectations from Go's incorrect outputs. Confirm the new focused and HTTP cases fail with the hosted choice before the fix, then pass after it. Existing conformance fixture inputs and auto-choice expectations remain unchanged; running the conformance/parity suites still guards their behavior.

Using invented SSE inputs in `recorded/` or `upstream/` would violate provenance policy, and changing only a fixture configuration would require additional index/provenance review. Provider-module synthetic transport is the smaller approved route. Request capture proves encoding, not whether a live OpenAI API accepts function-shaped selections for provider tools.

## Risks / Trade-offs

- [Existing tests encode the bug] → Replace the shell expectation only after obtaining independent pinned-TS evidence; extend it to canonical names and both missing kinds, and retain unaffected controls.
- [Conflating hosted declaration/allowed-tools kinds with ordinary choices] → Keep the complete name map and declaration conversion untouched; assert shell/local-shell allowed entries and tool-search rejection remain unchanged.
- [Snapshot provenance or accidental self-confirmation] → Commit reproducible pinned-TS capture inputs/recipe with expectations; keep synthetic provider responses only in focused tests, never in conformance input directories.
- [Synthetic success mistaken for live acceptance] → State the evidence boundary in the testdata provenance and review summary. No credentials or live API call is required.
- [Upstream reference changes before implementation] → Re-read `upstream.yaml`, resolve that exact package, and verify the allowlist/tests again; stop for reassessment if behavior changed rather than updating pins here.

## Migration Plan

No data or API migration is needed. Deliver the serializer fix, regression tests, and request goldens together as one independently green provider change. The observable correction is limited to ordinary forced shell/local-shell/tool-search request JSON; no release ordering or other-module API change is introduced. Roll back the narrow classifier change if regressions occur; do not compensate by removing mapping entries or editing provider fixtures.

Implementation gates: run `mise deps` before building/testing, then focused tests, `cd providers/openai && go test ./...`, `mise run test-conformance`, and `mise run parity-check`. Repeated capture/test runs must produce the same request expectations. Since no SSE or response contract changes, a new frontend integration scenario is unnecessary. Update `PARITY.md` only if the committed synthetic evidence changes a stable coverage/evidence boundary; do not add a dated assessment, copied issue inventory, or baseline upgrade metadata.

## Open Questions

None requiring a product/API decision for the approved 4.0.72 alignment. The missing direct upstream fallback tests and unverified live acceptance are explicit evidence limits, not unresolved serialization semantics.
