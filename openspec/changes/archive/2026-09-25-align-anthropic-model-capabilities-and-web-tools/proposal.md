## Why

Issue #217 remains valid against the registered `@ai-sdk/anthropic` 4.0.58 baseline (commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`): Go mistakes dated Vertex Claude 4 base models for unknown models, silently drops invalid web-tool arguments, and cannot request the upstream 20260318 web-tool variants. This leads to incorrect token/thinking/sampling requests or silently different server-tool behavior.

## What Changes

- Recognize both hyphenated and `@`-dated Claude Sonnet/Opus 4 base-family IDs when selecting capabilities, without modifying request model IDs or the existing Vertex resolution catalog. Preserve more-specific Claude 4/5 cases and the unknown-model fallback.
- Add provider-defined `anthropic.web_search_20260318` and `anthropic.web_fetch_20260318` request variants, version-specific argument projection, wire-name aliases and beta-header behavior. Retain the older dated web variants.
- Reject invalid declared web-tool arguments before HTTP rather than silently dropping malformed supported fields. Keep function-tool `strict`, `inputExamples`, and Anthropic provider options separate from provider-defined server tools; retain empty-tool tool-choice behavior.
- Specify focused capability/HTTP request tests and parity verification against the pinned implementation; do not treat synthetic responses as recorded conformance evidence or duplicate the already-fixed caller-metadata work.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `model-capabilities`: Match the pinned Claude 4 base-family `-`/`@` boundary and verify request-level token, reasoning and sampling behavior.
- `server-tools`: Support and validate the full registered web search/fetch version matrix, including 20260318 request fields and beta selection.
- `tool-name-mapping`: Map 20260318 server-tool declarations to their `web_search` and `web_fetch` wire names.

## Impact

Implementation is in the Anthropic module (`providers/anthropic/models.go`, `convert_request.go`, `tool_name_mapping.go`), with focused generate/stream request tests and a deterministic cross-language integration scenario for emitted custom tool names. The installed `anthropic-sdk-go v1.61.0` already contains `BetaWebSearchTool20260318Param` and `BetaWebFetchTool20260318Param`; no module dependency bump or new public Go API is planned. Provider request behavior changes but neither UI chunk framing nor caller-metadata parsing changes. Existing `test/conformance/PARITY.md` provider-adapter evidence boundary remains; add no invented provider recordings.
