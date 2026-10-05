## Context

The registered baseline is `@ai-sdk/openai@4.0.78`, `@ai-sdk/anthropic@4.0.65` and `@ai-sdk/provider@4.0.18` (`test/conformance/upstream.yaml`), which matches the versions issue #312 assessed.

Upstream `packages/openai/src/responses/openai-responses-language-model.ts` at `@ai-sdk/openai@4.0.78` lines 1084-1151 pushes `{type: 'source', sourceType: 'url', title: annotation.title}` for URL citations, and `{sourceType: 'document', title: annotation.filename, filename: annotation.filename}` for file and container-file citations, with `title: annotation.file_id` for `file_path`. The streaming branch at lines 2831-2856 is identical. Upstream source parts have no text field at all.

Upstream `packages/anthropic/src/anthropic-language-model.ts` at `@ai-sdk/anthropic@4.0.65` lines 129-167 sets `title: citation.title ?? undefined` for web-search citations and `title: citation.document_title ?? documentInfo.title` for document citations, and lines 1519-1533 set the web-search result title the same way.

In Go at `43527ef`, both streaming adapters already set `SourceInfo.Title`. The unary producers set `Text` and leave `Title` empty: `providers/openai/sources.go:14-60` and `providers/anthropic/convert_response.go:47-57` and `:218-227`. `middleware/simulate_streaming.go:123-130` reads `part.Title`, which is how the empty field becomes a visible defect.

`test/conformance/PARITY.md` classifies provider adapters as mixed coverage and distinguishes synthetic inputs from provider recordings, so this change adds focused Go tests rather than new provider fixtures.

## Goals / Non-Goals

**Goals:** Populate `Title` from the native values in both unary producers; keep source ids, media types, filenames, provider metadata and the URL/document split; prove the simulated-streaming path keeps titles.

**Non-Goals:** The Gateway source transport, which #113 and #246 completed; the middleware projection that #241 fixed; the Grafana provider, which already sets `Title`; any public API change; adding a legacy alias in place of the one being removed.

## Decisions

### 1. Stop writing the title into Text

The producers now set `Title` only. Keeping both fields would preserve the legacy alias this change exists to remove, and the V4 source contract has no text field to carry a title. The Gateway's `unarySource` fallback from `Title` to `Text` stays as it is, since it also serves the Grafana provider and older callers; it simply stops being the only reason unary titles reached the wire.

### 2. Document titles come from the filename

A document source needs a title, and the native annotations supply none, so the filename becomes the title and the file id does so for `file_path`, exactly as the registered upstream conversion and the Go streaming adapter already do.

### 3. One public-API probe

`TestSourceTitles_SurviveSimulatedStreaming` drives `NewResponses` over a fake transport, wraps it with `middleware.SimulateStreaming()` and asserts the four titles arrive in order. That is the path the issue's probe exercised, and it fails if either the producer or the middleware projection regresses.
