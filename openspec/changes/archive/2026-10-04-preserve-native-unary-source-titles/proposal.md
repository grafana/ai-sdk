## Why

[Issue #312](https://github.com/grafana/ai-sdk/issues/312) reports that native unary conversions put citation and source titles in `GenerateContentPart.Text` and leave the canonical V4 `Title` field empty. Every consumer reads `Title`: `middleware.SimulateStreaming()` copies `part.Title` into the stream part, and the Gateway maps `Title` for its wire sources. So a simulated stream of a unary OpenAI or Anthropic response emits sources with no title at all, while the streaming conversions in the same providers set `Title` correctly.

For Anthropic the `server-tools` spec already requires `Title` on non-streaming citation sources, so the code contradicted its own spec. The OpenAI requirement named `source` parts without saying which field carries the title.

## What Changes

- `providers/openai`: annotation-derived source parts set `Title` (URL citation title, document filename, file id for `file_path`) instead of `Text`.
- `providers/anthropic`: citation and web-search-result source parts in `convertResponse` set `Title` instead of `Text`.
- Point the existing unary source assertions at `Title` and add a probe proving a simulated stream of a unary response keeps every title.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Say that annotation-derived source parts carry their display text in `Title`, including the document filename and file-path cases, and not in `Text`.

## Impact

The implementation is confined to `providers/openai/sources.go`, `providers/anthropic/convert_response.go` and their tests. No public Go API, dependency, baseline or conformance fixture change. A consumer that read a unary source title from `Text` now reads it from `Title`, which is the field the V4 contract, the streaming path and every in-repo consumer already use. The Gateway keeps its `Title`-then-`Text` fallback and the Grafana provider keeps writing both, so neither layer changes.
