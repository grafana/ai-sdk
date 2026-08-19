## Context

H1 has a complete contract but maintains five response projection files, fully expanded negative and HTTP-envelope corpora, and separate scripted responses in the pinned-client recorder. The existing conformance suite already owns provider-independent LanguageModelV4 stream inputs and pinned TypeScript UI expectations that can serve as an independent oracle for a test-only raw transport lane.

The refactor must preserve the contract-only production boundary, exact registered npm pins, semantic JSON comparison, offline schema validation, and strict fixture provenance. Provider `recorded/` and `upstream/` inputs are immutable evidence and are not part of this change.

## Goals / Non-Goals

**Goals:**

- Keep semantic policy seeds curated while deriving mechanical variants in tests.
- Reuse one unary JSON seed and one clean SSE seed across interop scenarios.
- Reuse selected provider-independent conformance inputs and their existing UI expectations through the pinned Gateway client.
- Make verification non-mutating and artifact replacement explicit and atomic.
- Keep ownership and derivation reviewable in one index.

**Non-Goals:**

- Production ProviderWire decoding, serving, client, DTO, policy, model resolution, or Grafana adoption.
- Authentic-provider unary or streaming derivation, which belongs to H2 and H3.
- Changes to the HTTP or JSON contract, baseline versions, legacy transport, or user-facing documentation.
- Rewriting or relabeling provider conformance inputs.

## Decisions

### Curated seeds with narrow recipes

`positive.json`, `syntax.json`, `unary.json`, and `stream-clean.sse` remain curated semantic sources. Negative schema cases refer to a named positive case and apply only `set` or `remove` JSON-pointer mutations. HTTP envelope cases similarly refer to one of two complete seeds and apply narrow mutations. A small Go test helper expands these recipes in memory.

This keeps the independent policy decision visible while removing repeated complete documents. A general JSON Patch implementation or standalone fixture generator would add more machinery than H1 needs.

### Mechanical response variants stay in memory

The tolerated `[DONE]` stream is the clean SSE seed plus a final sentinel frame. Error responses are selected by name from curated positive error cases. The recorder and consumption tests import shared test-only projection helpers, so scripted request capture responses do not drift from contract projections.

Only `captures/requests.json` remains committed generated evidence. Derived streams and errors are not committed.

### Conformance reuse is a raw test-only transport lane

Selected `test/conformance/ui/**/input.jsonl` files are rendered in memory as ProviderWire `data: <JSON>\n\n` frames. Go contract tests validate every input part against the checked-in stream schema. The pinned Gateway client consumes the SSE using the fixture's existing orchestration context, `ai` assembles UI chunks, and the result is compared semantically with each existing `expected.jsonl`. The invalid-tool-input case therefore supplies the same declared tool schema used by its existing generator; it does not alter the provider input or expected output.

These inputs remain provider-independent curated fixtures, and their expected UI output remains a pinned TypeScript oracle. The lane does not claim provider recording, private-server behavior, host-policy enforcement, or a Go runtime.

### Verification and replacement are separate

`mise run check-providerwire-v4` aggregates baseline, offline contract, and pinned-client evidence and never writes committed files. `mise run update-providerwire-v4-artifacts` is the only replacement command; it generates, privacy-checks, round-trips, and atomically renames the request capture in the destination directory.

The old capture-specific task name is removed so the index and task surface identify one update workflow.

## Risks / Trade-offs

- [Mutation recipes can obscure an invalid document] → Keep operations limited to `set` and `remove`, require named positive bases, and report expanded validation paths in subtests.
- [The TypeScript transport renderer could become an oracle for itself] → Compare only with existing independently generated conformance expectations and validate source parts separately in Go.
- [Selected conformance cases may expose unsupported contract arms] → Start with the three already identified provider-independent fixtures and fail schema validation rather than weakening the contract.
- [Atomic replacement can still accept incorrect generated content] → Run pin, privacy, and semantic checks before rename; normal verification independently compares the committed artifact.
