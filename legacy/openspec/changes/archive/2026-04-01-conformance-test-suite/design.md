## Context

This is a Go port of Vercel's AI SDK that must be wire-compatible with `@ai-sdk/react`. Current tests validate at three levels: Go unit tests with hand-written mocks, Go E2E tests with `httptest`, and cross-language integration tests (Go server + TypeScript Vitest). These layers prove our SSE output is parseable and our internal logic works, but they cannot catch unknown gaps -- missing event types, dropped blocks, wrong field mappings -- because they only test what we explicitly anticipated. Real bugs have followed this pattern: a provider returns a block type we don't handle, our code silently drops it, and no test catches it because no test knew to look for it.

The upstream TypeScript SDK tests providers using fixture files (`.chunks.txt` with one JSON per line, `.json` for non-streaming) replayed via MSW. There are no cross-language conformance tests upstream -- each provider is tested independently. We need something the upstream doesn't: validation that our Go port produces the same output as the TypeScript SDK for the same inputs.

## Goals / Non-Goals

**Goals:**
- Catch behavioral divergence between Go and TypeScript SDK for the same provider API responses
- Detect missing implementations (unhandled block types, dropped events) automatically
- Make adding new conformance test cases trivial (add fixture + config, run generator, commit)
- Keep Go test execution fast and deterministic (no live API calls at test time)
- Validate the full pipeline: raw provider response -> Go provider -> StreamText -> ToUIMessageStream -> UIMessageChunk sequence
- Support multi-step tool calling scenarios (model calls tool -> mock result -> model continues)
- Enable bulk-importing of upstream Vercel SDK fixtures with minimal overhead

**Non-Goals:**
- Replacing existing unit tests, E2E tests, or cross-language integration tests
- Testing the TypeScript SDK itself (it's the oracle, assumed correct)
- Exhaustive coverage of every API edge case on day one (start with core scenarios, expand)
- Validating SSE framing (`data: ...\n\n`) -- already covered by existing integration tests
- Live API testing in CI (recording is offline, Go tests are pure replay)
- Request validation (verifying we send correct requests to providers -- covered by existing unit tests)

## Decisions

### Fixture format: upstream `.chunks.txt` convention

Fixtures use the same format as the upstream Vercel AI SDK: one JSON object per line, no SSE framing. SSE framing (`event: <type>\ndata: <json>\n\n`) is added by the replay server at serve time, matching what the real Anthropic API sends. The event type is extracted from each line's JSON `type` field.

This means we can copy fixtures directly from upstream's provider packages (e.g., `packages/anthropic/src/__fixtures__/`) as a starting point, and the format is familiar to anyone who has worked with the upstream codebase.

**Alternative considered:** Raw HTTP response bytes. More faithful to what the provider actually returns, but harder to inspect, edit, and version-control. The `.chunks.txt` format is human-readable and diff-friendly.

### Fixture categories: upstream and recorded

Conformance fixtures fall into two categories based on their origin:

**Upstream fixtures** are copied verbatim from the Vercel AI SDK test fixtures (e.g., `packages/anthropic/src/__fixtures__/`). These are static -- we cannot regenerate the fixture files since we don't control upstream's test scenarios. They live under `<provider>/upstream/` and each test case directory has a lightweight `config.yaml` with just enough metadata for the generate tool and Go test runner (model, tools, provider options). No `prompt` field since re-recording is not applicable.

**Recorded fixtures** are captured by our `record.mts` tool from real provider APIs. They live under `<provider>/recorded/` and have a full `config.yaml` including the `prompt` field, enabling re-recording when needed. These are our own test scenarios designed to cover specific behaviors.

The `expected.jsonl` is generated the same way for both categories: the `generate.mts` tool replays fixtures through the upstream TypeScript SDK and captures the UIMessageChunk output. The only difference is whether the HTTP fixtures themselves can be regenerated.

This separation makes bulk-copying upstream fixtures trivial (copy into `<provider>/upstream/`, add configs, run generate) and keeps the distinction clear in the directory structure.

Each provider's `upstream/` directory contains an `INDEX.yaml` that maps upstream fixture names to local test case directories (or `null` for fixtures not yet imported). This provides a single place to see import coverage and identify which upstream fixtures are missing.

**Alternative considered:** Single flat directory with a flag in the YAML to mark fixtures as upstream. Rejected because mixing fixture origins obscures which tests can be re-recorded and makes bulk operations harder.

### Exact comparison instead of normalization

When replaying a fixed fixture, the output is deterministic. Text content, tool names, usage numbers, URLs -- all originate from the fixture bytes and are identical in both SDKs. The only non-deterministic values are SDK-generated IDs (message IDs, part IDs).

All IDs in the UIMessageChunk stream are deterministic when replaying a fixture: content block IDs come from the provider (stringified block index), tool call IDs come from the API response (in the fixture), and message-level IDs are controlled via `GenerateMessageID` / `generateMessageId`. This allows exact JSON comparison with no normalization or remapping.

**Alternative considered:** Full normalization contract. Replaces text with `<text>`, URLs with `<url>`, numbers with `0`, IDs with sequential placeholders -- implemented identically in both TypeScript and Go with shared test vectors. Rejected because: (1) most normalized fields come from the fixture and are already deterministic, (2) normalization hides bugs (e.g., emitting a number as a string), (3) dual-language normalization has drift risk.

### Multi-step scenarios with ordered fixtures

Multi-step tool calling involves multiple `DoStream` calls. Each is a separate HTTP request/response, so each step gets its own fixture file:

```
anthropic/recorded/multi-step-weather/
  config.yaml
  input-1.chunks.txt    # step 1: model returns tool_use block
  input-2.chunks.txt    # step 2: model returns text after tool result
  expected.jsonl         # single file: full UIMessageChunk sequence across ALL steps
```

Single-step scenarios use a single `input.chunks.txt` (no number suffix). This naming convention applies to both upstream and recorded fixtures.

The replay server is stateful: it tracks request count per test case and serves fixtures in order. First POST returns step 1, second returns step 2, etc.

The `expected.jsonl` is always a single file because the UIMessageChunk stream is a single continuous stream from the consumer's perspective. Step boundaries are visible as `start-step`/`finish-step` chunks within the sequence.

**Alternative considered:** One expected file per step. Rejected because it tests at the wrong abstraction level -- we care about the wire output, not internal stepping.

### Per-test configuration in `config.yaml`

Each test case directory contains a `config.yaml` with the metadata needed for replay and expected output generation. The schema is the same for upstream and recorded fixtures; the `prompt` field is only present in recorded fixtures (used by the record tool for re-recording).

```yaml
# Upstream fixture config (minimal)
model: claude-sonnet-4-20250514

# Recorded fixture config (full, supports re-recording)
model: claude-sonnet-4-20250514
prompt: "What's the weather in SF?"
stopWhenStepCount: 3
providerOptions:
  anthropic:
    thinking:
      type: enabled
      budgetTokens: 1024
tools:
  get_weather:
    description: "Get weather for a location"
    inputSchema:
      type: object
      properties:
        location: { type: string }
    mockResults:
      - { temperature: 72, conditions: "sunny" }
```

There is no `provider` field in `config.yaml`. The provider is determined by the parent directory structure (`anthropic/`, `openai/`, etc.), avoiding redundancy and keeping provider assignment structural.

This serves both TypeScript tools (recording/generating) and Go tests (replay). It's not a scenario DSL -- it's replay configuration.

**Alternative considered:** Separate YAML schemas for upstream and recorded fixtures. Rejected because the fields are a superset/subset -- a single schema with optional fields is simpler and avoids parser duplication.

**Alternative considered:** Central `config.yaml` per provider. Per-test-case files are better because everything for a test is co-located, no merge conflicts when adding tests in parallel, and directories are trivially auto-discoverable.

### Per-provider directory organization

Conformance tests are organized by provider as the primary axis. Each provider gets its own directory under `test/conformance/` containing a Go test file and fixture subdirectories (`upstream/` and `recorded/`).

This structure mirrors the Go SDK's own organization (each provider is a separate module) and matches how upstream organizes its fixtures per-provider package.

The shared infrastructure (replay server, comparison logic, test helpers) lives at the `test/conformance/` root. Per-provider test files wire their specific provider to the shared runner.

Adding a new provider means: create a directory, copy/record fixtures, write a `conformance_test.go` that instantiates the provider and delegates to the shared runner. The test infrastructure itself doesn't change.

**Alternative considered:** Flat directory with provider as a YAML field. Rejected because it doesn't scale: different providers need different test files to instantiate their provider, and organizing by provider makes bulk-copying upstream fixtures per-provider-package trivial.

### Two TypeScript tools: record and generate

**`record.mts`**: Captures new fixtures from real APIs. Needs API keys. Only works with `recorded/` directories (upstream fixtures cannot be re-recorded). Reads `config.yaml`, sets up a recording proxy, configures upstream SDK, runs `streamText`, saves each HTTP response as `input-N.chunks.txt`, captures UIMessageChunk stream as `expected.jsonl`.

**`generate.mts`**: Regenerates expected output from existing fixtures. No API keys needed. Works with both `upstream/` and `recorded/` fixtures. Reads `config.yaml` + `input-*.chunks.txt` files, serves them through a local test server, runs the upstream TypeScript SDK's full pipeline against the test server, and writes the resulting UIMessageChunk sequence to `expected.jsonl`.

The separation matters because recording is rare (new scenarios, API format investigation) while regeneration is frequent (upstream SDK updates, verifying expected output after changes).

### Go test infrastructure: per-provider auto-discovery + replay

Each provider directory has a `conformance_test.go` that auto-discovers test case directories under both `upstream/` and `recorded/`, and runs the replay-and-compare cycle for each. Adding a new test case requires no Go code changes -- just new fixture files and a `config.yaml`.

Tests are gated behind `//go:build conformance` to keep them separate from the main test suite. The `test/conformance/` directory is a separate Go module since it needs to import both `aisdk` and provider modules.

### Directory structure

```
test/conformance/
  go.mod
  runner.go                 # shared replay, comparison, helpers
  tools/                    # TypeScript generate + record tools
    generate.mts
    record.mts
    package.json
    tsconfig.json
  anthropic/
    conformance_test.go     # wires anthropic provider to shared runner
    upstream/               # copied from Vercel SDK
      INDEX.yaml            # maps upstream fixture names to local directories
      <test-name>/
        config.yaml
        input.chunks.txt
        expected.jsonl
    recorded/               # our own, re-recordable
      multi-step-weather/
        config.yaml
        input-1.chunks.txt
        input-2.chunks.txt
        expected.jsonl
      thinking-enabled/
        config.yaml
        input.chunks.txt
        expected.jsonl
```

### Relationship to existing test layers

| Layer | What it tests | How |
|---|---|---|
| Unit tests (Go) | Internal logic, type conversions, stream processing | Hand-written mocks, testify |
| E2E tests (Go) | Full StreamText-to-SSE pipeline | httptest + mock models |
| Integration tests (Go + TS) | Wire compatibility with @ai-sdk/react | Go server + TypeScript Vitest |
| **Conformance tests (new)** | **Behavioral equivalence with TypeScript SDK** | **Fixture replay + exact comparison** |

Each layer catches a different class of bug. Conformance tests specifically catch "we didn't know to test for this" bugs.

## Risks / Trade-offs

**[Stale snapshots]** Provider API changes, fixtures don't reflect current behavior. -> Mitigation: For recorded fixtures, `record.mts` can re-capture from live APIs. For upstream fixtures, re-copy from updated Vercel SDK fixtures. For both, `generate.mts` can regenerate `expected.jsonl` without API calls.

**[ID generation order mismatch]** TypeScript and Go SDKs generate IDs in different order for some edge cases, breaking exact comparison. -> Resolved: all IDs in the UIMessageChunk stream originate from the fixture (content block indices, tool call IDs) or are explicitly controlled (message IDs via deterministic generators). No remapping needed.

**[Recording proxy complexity]** The `record.mts` proxy needs to intercept multi-step HTTP exchanges. -> Mitigation: Use a simple `http.createServer` proxy that forwards to the real API and tees response bodies to files. This is simpler than hooking into SDK internals. For multi-step, the proxy records responses in order (request counter -> `input-N.chunks.txt`).

**[Scenario expressiveness limits]** `config.yaml` can't express complex conditional tool logic. -> Mitigation: Mock tool results are consumed in order (list, not argument-matching). This covers the common case. If truly complex scenarios arise, they can use a small TypeScript file instead of YAML, without changing the Go test infrastructure.

**[Provider-specific replay]** Each provider has different SSE conventions. -> Mitigation: Start with Anthropic only. The replay server adds Anthropic-style SSE framing. When adding providers (OpenAI, etc.), add a thin adapter per provider. The provider directory drives adapter selection.

**[Upstream fixture staleness]** Upstream Vercel SDK may update its fixture format or reorganize fixture directories. -> Mitigation: Upstream fixtures are point-in-time copies. Re-copy periodically and re-run `generate.mts`. The lightweight `config.yaml` per test case makes re-integration straightforward.
