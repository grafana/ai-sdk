## Why

Source entries emitted by the Anthropic provider are missing the `id` field that upstream always sets via `generateId()`. This breaks frontends that depend on stable source IDs from both `DoStream` (web search results) and `DoGenerate` (citations and web search results). Found during review of #82 (citations delta work).

## What Changes

- Streaming web search sources in `emitWebSearchResult` will include `ID: a.generateID()` on every `SourceInfo`.
- Non-streaming `GenerateContentPart` will gain an `ID` field so source entries from `convertResponse` can carry IDs.
- Non-streaming web search source entries and citation source entries in `convertResponse` will populate the new `ID` field using `generateID()`.

## Capabilities

### New Capabilities

### Modified Capabilities

- `v4-source-document`: Source entries now require an `ID` on all source types (url and document) in both streaming and non-streaming paths, aligning with upstream behavior.

## Impact

- `provider/language_model.go` -- `GenerateContentPart` struct gains `ID string` field.
- `anthropic/convert_stream.go` -- `emitWebSearchResult` sets `ID` on `SourceInfo`.
- `anthropic/convert_response.go` -- citation and web search source entries populate `ID` on `GenerateContentPart`.
- Wire format: non-streaming `GenerateResult` content parts of type `source` will now include an `id` JSON field, matching upstream.
