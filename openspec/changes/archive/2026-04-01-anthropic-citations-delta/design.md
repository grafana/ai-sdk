## Context

When using Anthropic's provider-executed tools like `web_search`, the API sends inline citation references as `citations_delta` events interleaved with `text_delta` events during streaming. In non-streaming responses, citations appear as a `citations` array on text content blocks. The Go port's stream adapter (`convert_stream.go`) and response converter (`convert_response.go`) both ignore these citations entirely.

The Anthropic Go SDK (v1.17.0) has full support for citation types: `BetaCitationsDelta` (streaming) and `BetaTextCitationUnion` (non-streaming), with 5 citation variants: `web_search_result_location`, `char_location`, `page_location`, `content_block_location`, `search_result_location`.

The provider package already has all needed types: `PartSource`, `SourceInfo` (with `SourceType`, `URL`, `Title`, `MediaType`, `Filename`, `ProviderMetadata`). The `emitWebSearchResult` function already emits `PartSource` with `SourceType: "url"` for batch search results — so the plumbing is proven.

Current state:
- `convert_stream.go` delta handler switches on 5 delta types, missing `citations_delta`
- `convert_response.go` reads `block.Text` for text blocks but ignores `block.Citations`
- `streamAdapter` struct has no citation document tracking
- `web_search_tool_result` emits batch `PartSource` for search result URLs — but inline citations pointing to those same results are dropped

## Goals / Non-Goals

**Goals:**
- Handle `citations_delta` events in streaming, converting them to `PartSource` stream parts
- Handle `citations` arrays on text blocks in non-streaming responses, converting them to `GenerateContentPart` source entries
- Track citation documents from prompt file parts to resolve `page_location`/`char_location` citations by document index
- Match upstream TypeScript behavior for all citation types the upstream handles

**Non-Goals:**
- `content_block_location` citation type — the upstream TypeScript SDK doesn't handle it (`createCitationSource` returns undefined for it). We skip it too.
- `search_result_location` citation type — the upstream doesn't handle it either. We skip it.
- Prompt-side citation configuration (`citations.enabled` on file parts) — this is about request building, not response handling. Out of scope.
- Deduplication between batch `web_search_tool_result` sources and inline `citations_delta` sources — upstream doesn't deduplicate either; they serve different purposes (batch = all results, inline = specific citations in text).

## Decisions

### 1. Citation-to-source conversion as shared helper

**Decision**: Create a `createCitationSource` function usable by both streaming and non-streaming paths. Both `BetaCitationsDeltaCitationUnion` (streaming) and `BetaTextCitationUnion` (non-streaming) resolve to the same concrete types via `.AsAny()`, so the same switch-on-concrete-type logic works for both.

The function takes the citation variant (via `.AsAny()` result) and the `citationDocuments` slice, and returns `(*provider.SourceInfo, bool)`.

**Rationale**: Avoids duplicating the citation type mapping logic. Mirrors upstream's shared `createCitationSource`.

### 2. Citation document tracking on streamAdapter and convertResponse

**Decision**: Add a `citationDocuments` field to `streamAdapter` (slice of `{title, filename, mediaType}` structs). For non-streaming, pass the same slice to `convertResponse`. Both are populated from the prompt's user message file parts at init time, before the stream/request starts.

The extraction function scans user messages for `FileContentPart` entries with `MediaType` of `application/pdf` or `text/plain` that have `providerOptions.anthropic.citations.enabled = true` in their provider metadata.

**Rationale**: `page_location` and `char_location` citations reference documents by index. Without this tracking, those citation types can't produce meaningful source info. Matches upstream's `extractCitationDocuments`.

**Alternative**: Skip document-based citations entirely and only handle `web_search_result_location`. Rejected — this would be a known behavioral divergence from upstream, and the extraction logic is straightforward.

### 3. Citation types handled

**Decision**: Handle 3 citation types matching upstream:
- `web_search_result_location` → `SourceInfo{SourceType: "url", URL, Title}` + provider metadata with `citedText` and `encryptedIndex`
- `page_location` → `SourceInfo{SourceType: "document", MediaType, Title, Filename}` + provider metadata with `citedText`, `startPageNumber`, `endPageNumber`
- `char_location` → `SourceInfo{SourceType: "document", MediaType, Title, Filename}` + provider metadata with `citedText`, `startCharIndex`, `endCharIndex`

Unknown types (including `content_block_location`, `search_result_location`) are silently skipped.

**Rationale**: Exact match with upstream's `createCitationSource` behavior.

### 4. ID generation for citation sources

**Decision**: Use the existing `idGenerator` from the model options to generate unique IDs for each `SourceInfo`. The `streamAdapter` needs access to an ID generator, which can be passed during construction.

**Rationale**: Upstream uses `generateId()` for each citation source. Source IDs are used by frontends to track and display citation references.

### 5. Non-streaming text block citation extraction

**Decision**: In `convertResponse`, after appending a text `GenerateContentPart`, iterate over the block's `Citations` array and append source `GenerateContentPart` entries. This directly mirrors the upstream's non-streaming handler which pushes `content.push(source)` after `content.push({ type: 'text', text: part.text })`.

**Rationale**: Straightforward port of upstream behavior. The `BetaContentBlockUnion` struct already exposes `Citations []BetaTextCitationUnion` for text blocks.

## Risks / Trade-offs

- **Citation document index mismatch**: If the prompt file part extraction doesn't match the order Anthropic uses internally, document-indexed citations will resolve to wrong documents. Mitigation: the extraction follows the exact same algorithm as upstream (user messages, file parts, PDF/text only, citations.enabled).

- **ID generator threading**: The `streamAdapter` currently doesn't receive an ID generator. Adding it changes the constructor signature. Mitigation: this is an internal API, not exposed to users. The model already has `idGenerator` available.

- **Future citation types**: `content_block_location` and `search_result_location` are silently dropped. If these become important, we'll need to revisit. Mitigation: upstream doesn't handle them either, so we're aligned.
