## Context

The registered baseline is `vercel/ai` `08ae5ad05bc12496dd1ffcf64e34419e0831300d`, `@ai-sdk/openai` 4.0.71 (`test/conformance/upstream.yaml`). Its `packages/openai/src/responses/convert-to-openai-responses-input.ts` implements generic content conversion at lines 70–222, scalar cache selection at 336–351, ordered parallel child serialization at 1306–1348, custom output at 1480–1594, and generic output at 1624–1644. Its tests cover generic scalar breakpoints (2895–2969), multipart/reference/media (3220–3763), and custom scalar/content (around 6030–6290). The custom branch warns/drops uploaded references, whereas the generic branch resolves them to `file_id`.

Go's `convertProviderToolResult` currently retains hosted dispatch and scalar/schema encoding, but `toolResultOutputString` loses generic content arrays. `customToolCallOutputItem` supports content text, inline bytes, and URLs, but scalar cache hints are lost and reference warnings differ from upstream. `parallelToolResultGroup.output` already groups children by index and handles scalar child breakpoints, but uses the custom converter for content. `provider.ToolResultContentValue.Data` carries URL, inline bytes/base64, or provider reference; `inputConversionContext.providerOptionsName` and `resolveFileReference` already establish namespace-sensitive resolution. The provider-adapter row of `test/conformance/PARITY.md` classifies request snapshots and synthetic provider tests as separate evidence.

## Goals / Non-Goals

**Goals:**
- Match pinned generic function output shapes and cache selection for text, JSON, errors, denial, mixed content, media, and provider references, preserving warnings, call identity, and output-schema scalar quoting.
- Match pinned custom scalar cache and multipart text/data/URL behavior; emit the pinned custom reference warning and omit that part without claiming reference support.
- Match ordered parallel wrapper output serialization and breakpoint placement without changing grouping/hosted tool routing.

**Non-Goals:**
- No new API, tool taxonomy, stream protocol, upstream upgrade, or intentional permanent deviation.
- No custom-tool uploaded-reference support: issue #221 acceptance is explicitly narrowed to generic function results because the registered custom path warns and drops references.
- No invented recorded or upstream provider fixture payloads and no change to provider-independent SSE output.

## Decisions

1. **Keep taxonomy dispatch first.** Preserve the existing `tool_search`, computer, shell, apply-patch, programmatic, hosted, execution-denied approval, and custom branches; replace only the generic output rendering seam with a typed conversion and add scalar cache handling to custom. Reuse the current option namespace and reference/media helpers rather than exposing public types or rerouting hosted results. Preserve `call_id`, `caller`, and function output-schema text/error/denial JSON-string quoting (custom scalar remains unquoted).

2. **Use separate generic and custom content policies.** Generic content yields an ordered typed `function_call_output.output` array: text -> `input_text`; image URL/bytes/reference -> `input_image` with `image_url` (URL/data URI) or `file_id`, and optional `detail`; non-image URL/bytes/reference -> `input_file` with `file_url`, or `filename` (default `data`) plus base64 `file_data`, or `file_id`. Resolve the full inline media type with the same helper used for user files, but do not apply the user-file converter's PDF-only restriction: pinned generic tool results accept non-image inline data of other media types. Attach each content element's own namespaced cache breakpoint. An unsupported generic content/data type warns `unsupported tool content part type: <type>` or `unsupported tool content part type: file with data type: <type>` and is omitted. A missing active namespace in a generic uploaded reference must not silently use another namespace or emit an empty ID; follow the existing file-reference error path. Custom output retains native `custom_tool_call_output` text/data/URL conversion but warns exactly `unsupported custom tool content part type: file with data type: reference` and omits custom reference parts; this is upstream behavior rather than a supported conversion. Do not reuse the generic converter for custom content because its reference policy differs.

3. **Scalar cache only for scalar results.** For generic and custom text/JSON/error/denied output choose output provider-option `promptCacheBreakpoint` first, otherwise tool-result part option, under the active OpenAI/Azure namespace. When present, render an `input_text` array with that breakpoint; otherwise preserve the scalar string. For `content`, ignore result/output-level scalar hints and retain only each content item's own options. Preserve existing scalar JSON and error JSON bytes/string semantics and default denial reason. Do not apply function output-schema quoting to custom results.

4. **Keep parallel child serialization, not native flattening.** Convert each child through generic conversion, stringify any non-string child output as JSON, and join in original child-index order with newlines. Only scalar child breakpoints trigger the wrapper's `input_text` array, with each child's serialized text (newline prefixed after the first) and selected scalar hint. If none, send a single joined string. Multipart per-content cache hints remain inside the JSON-serialized child output; output-level/part-level hints on content do not trigger wrapper text breakpoints. Retain wrapper call ID and existing conversation/previous-response semantics and invalid/incomplete fallback. This matches pinned `Promise.all` indexed serialization without turning the wrapper into multiple native outputs.

5. **Prove the provider request boundary at its available evidence level.** First inspect existing `recorded/` or matching pinned `upstream/` OpenAI continuation fixture inputs and `INDEX.yaml` for an authentic multipart/cache/reference replay that can yield a request snapshot; if one exists, update/regenerate its `expected-requests.jsonl` first, observe Go replay failure, then fix conversion and rerun. Existing `recorded/` input chunks are immutable real API captures; `upstream/` inputs are byte-identical registered imports. `mise run generate-conformance` regenerates expectations only, never input provenance. If no authentic reproducer exists, write failing focused synthetic request-body tests under `providers/openai/` first (no fake provider conformance inputs), document the provider-boundary snapshot gap, and do not claim live acceptance. Test both OpenAI and Azure option namespaces and preserve existing hosted routing regressions. Run OpenAI module tests and `mise run parity-check` after implementation.

## Risks / Trade-offs

- [OpenAI SDK union shape or optional field loss] → Assert marshaled request-body JSON for mixed typed content, detail, IDs, warnings, and breakpoints, including empty unsupported-content arrays.
- [Custom reference acceptance from issue mistaken for implemented parity] → Explicit warning/drop scenario and narrowed acceptance in proposal/spec/tests; a future upstream change needs a separately approved baseline/scope decision.
- [Multipart parallel outputs mistaken for native blocks] → Assert JSON-stringified ordered children and wrapper scalar-only breakpoint behavior against pinned converter tests.
- [Snapshot provenance/coverage gap] → Check existing fixture inventories before writing expectations; use focused synthetic tests if authentic inputs are unavailable, never synthesize recorded/upstream inputs.

## Migration Plan

No data migration or public API change. Land regression tests and request conversion together; rollback by reverting the provider conversion/tests. Validate with OpenAI module tests, existing conformance replay if relevant, and `mise run parity-check`.

## Open Questions

None for this approved strict-parity scope. Authentic request-snapshot feasibility is an evidence check at implementation time, not approval to fabricate provider inputs.
