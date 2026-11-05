## Context

The registered upstream reference is Vercel commit `028a025398782f4938303f48fbeee45c79af544b`, `@ai-sdk/amazon-bedrock` 5.0.130. Its `packages/amazon-bedrock/src/convert-to-amazon-bedrock-chat-messages.ts` keeps a generated-name counter and used-name set inside each conversion. The corresponding tests cover duplicate filenames, extension/sanitization collisions, literal suffixes, generated-name collisions, long names, and user/tool-result boundaries. Both exact source and tests were inspected.

## Goals / Non-Goals

Match that request naming behavior across every existing Go document conversion path. Do not change filename sanitization, content bytes, citations, public API, or unrelated providers.

## Decisions

Replace the existing shared integer pointer with a request-local naming state holding its counter and used-name set. Allocate at the existing shared `buildDocumentBlock` boundary, which serves user files and tool-result files in user, tool, and assistant messages. Starting from the sanitized or generated base, try the base then ` (2)`, ` (3)`, and so on until unused. Truncate and trim the base to reserve suffix space within 200 ASCII characters. Existing sanitization already restricts names to ASCII, so Go byte slicing matches upstream for these names. Named documents continue not to advance the generated-name counter. State is recreated on each conversion, preserving deterministic prompt caching and independent concurrent requests.

Classification: implementation bug corrected using a parity-preserving Go adaptation. The coverage map classifies provider adapters as mixed; focused request tests provide evidence for this surface, not a complete provider parity claim. No recorded or upstream input fixtures are invented.

## Risks / Trade-offs

Previously duplicated emitted names change, intentionally matching the pinned upstream request. Literal suffixes participate in the same set, so `invoice (2)` may become `invoice (2) (2)` if already used. Exact output depends on request order, as upstream does.

## Validation

Run request regressions red before production edits, then all Bedrock tests, race tests, module lint, and available direct parity-check constituents. Archive and sync the completed spec change. No live AWS recording is needed or claimed.
