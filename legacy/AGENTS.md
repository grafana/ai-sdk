# AGENTS.md

Go port of Vercel's AI SDK (`github.com/grafana/ai-sdk`), providing streaming
LLM orchestration wire-compatible with `@ai-sdk/react`.

## Upstream Reference

This project is a Go port of [Vercel's AI SDK](https://github.com/vercel/ai).
The upstream TypeScript implementation is the canonical reference for behavior,
naming, and protocol details.

Every implementation, bug fix, refactor, test, fixture, and documentation task
should consider the matching upstream behavior before deciding the Go shape.
Use upstream as normal development context, not only for explicit parity work.
Always compare against the same upstream package versions registered in
`test/conformance/upstream.yaml`; do not compare local Go code against upstream
`main`, latest docs, or an arbitrary package version unless the task is
explicitly upgrading the baseline. When local upstream checkouts or online
source are used, resolve them to the registered version/tag first and note any
version mismatch in the work summary.

- **Wire compatibility with `@ai-sdk/react`**: The SSE wire format
  (`UIMessageChunk`) must stay compatible with `useChat`, `useCompletion`, and
  `useObject`. Any change to chunk types, serialization, or SSE framing must be
  validated against the TypeScript frontend hooks.
  Docs: https://ai-sdk.dev/docs
- **Provider interface mirrors LanguageModelV4**: `provider.LanguageModel`
  follows the upstream LanguageModelV4 spec. Keep the interface aligned.
  Spec: https://github.com/vercel/ai/tree/main/packages/provider/src/language-model/v4
- **Use upstream as reference**: When investigating bugs, designing features, or
  clarifying expected behavior, consult the Vercel AI SDK source and docs first.
  Source: https://github.com/vercel/ai

## Development Workflow

### Upstream Alignment

The upstream TypeScript source is the canonical reference for behavior,
naming, and protocol details. Upstream comparison is required for all normal
development work and at every development phase:

Upstream alignment means matching behavior, semantics, and wire format --
not translating TypeScript patterns into Go. Adapt the design to use Go
idioms (interfaces, channels, error returns, etc.) rather than mirroring
the TypeScript architecture directly.

Before implementing, identify the registered upstream package version from
`test/conformance/upstream.yaml` and use source/tests for that same version as
the reference. If the corresponding upstream source cannot be found, state the
gap and avoid substituting another version silently.

- **Planning**: Read the corresponding upstream implementation before designing.
  Trace the execution flow, understand edge cases and design decisions. Document
  any intentional deviations with rationale.
- **Implementation**: Reference upstream source while writing Go code. Read both
  implementation and tests before porting a feature. Preserve upstream semantics
  even where Go idioms differ from TypeScript patterns.
- **Review**: Compare the Go implementation against upstream to verify
  behavioral alignment. Check for drift in wire format, API surface, error
  handling, and edge cases.

### Parity Governance

The registered upstream baseline lives in `test/conformance/upstream.yaml`; the
coverage map lives in `test/conformance/PARITY.md`. Any parity-sensitive change
must identify the relevant baseline and coverage status before implementation
is considered complete.

Parity-sensitive work includes stream parts, UI chunks, SSE framing, provider
messages, provider request conversion, tool orchestration, output behavior,
provider options, frontend interop, and conformance fixtures.

- **Coverage map first**: Use `test/conformance/PARITY.md` to classify the
  work by layer: core orchestration, provider contract, provider implementation,
  frontend interop, or conformance harness. The required proof differs by layer;
  provider work usually needs request snapshots, while core stream work usually
  needs UI chunk snapshots.
- **Conformance-first TDD**: When a reported bug can be reproduced by recorded
  provider chunks or provider request snapshots, add or update the conformance
  fixture first, confirm the Go replay fails, then implement the fix. For new
  parity-sensitive features, record or import upstream behavior alongside the
  implementation so the fixture becomes the regression contract.

#### Conformance Fixture Provenance

Provider fixture inputs are evidence, not test data to invent. The provenance of
`input.chunks.txt` and `input-*.chunks.txt` determines whether a conformance test
can support a parity claim.

- Files under `test/conformance/<provider>/recorded/` must be captured from a
  real provider API by `mise run record-conformance`. Never hand-author,
  synthesize, or assemble provider events and present them as recorded input.
- Recorded inputs must remain the provider events captured by the recording
  tool. Do not derive, truncate, splice, rewrite, or otherwise modify them.
  Synthetic provider responses and transport-failure scenarios belong in
  focused unit or integration tests, or in provider-independent
  `test/conformance/ui/` fixtures when they exercise core stream behavior.
- Files under `test/conformance/<provider>/upstream/` must be copied from the
  matching registered upstream package fixtures and listed in `INDEX.yaml`.
  Never place locally invented inputs in `upstream/`.
- `mise run generate-conformance` only regenerates expectations from existing
  inputs. It does not record provider responses, establish input provenance, or
  make synthetic inputs valid conformance evidence.
- If a live provider recording or matching upstream fixture is unavailable,
  use focused unit tests or a provider-independent `test/conformance/ui/`
  fixture when appropriate, and document the remaining provider-boundary
  coverage gap. Never substitute invented provider payloads.
- Reviews must verify the provenance of every added or changed provider
  `input*.chunks.txt` file before accepting its snapshots as parity evidence.

- **Baseline checks**: Run `mise run parity-check` when changing committed
  parity behavior. For metadata-only changes, run
  `mise run validate-parity-baseline`.
- **Fixture updates**: Wire-format or provider-boundary behavior changes must
  consider whether `expected.jsonl`, `expected-requests.jsonl`, or
  `expected-object.json` needs to be regenerated.
- **Divergence classification**: Every observed upstream difference must be
  classified as parity-preserving Go adaptation, intentional deviation,
  implementation bug, or coverage gap.
- **Documented gaps**: Intentional deviations and accepted coverage gaps must be
  recorded in `test/conformance/upstream.yaml` or `test/conformance/PARITY.md`.
- **Upstream upgrades**: Package/version bumps must update the baseline
  manifest, conformance dependency pins, generated snapshots, and lockfiles
  together. The selected stable package set must satisfy the
  `minimumReleaseAge` in `test/pnpm-workspace.yaml`; do not bypass that gate.
  Use the `ai-sdk-parity-upgrade` skill for that workflow.

## Build / Lint / Test Commands

```bash
# Trust the project config on first checkout
mise trust

# Install workspace dependencies
mise deps

# Build all modules
mise run build

# Format
mise run fmt           # gofmt -w .

# Vet + lint
mise run vet           # go vet ./... (all modules)
mise run lint          # golangci-lint (all modules)

# All tests (root + providers)
mise run test

# Root module tests only
go test ./...

# Short tests (skip integration/E2E)
mise run test-short    # go test -short ./...

# Single test by name
go test -run TestStreamTextSingleStep ./...

# Single test in anthropic module (must run from providers/anthropic/ directory)
cd providers/anthropic && go test -run TestBuildParams_SystemMessage ./...

# Tidy deps
mise run tidy

# Full check (fmt + vet + test)
mise run check

# Upstream parity checks
mise run validate-parity-baseline
mise run parity-check
```

The Anthropic provider module is a separate `go.mod`. Run its tests from the
`providers/anthropic/` directory or via `mise run test`. The same applies to the
Bedrock provider module under `providers/bedrock/`.

## Project Structure

```text
aisdk/              Root package - orchestration (StreamText, UIMessage, SSE, tools)
  provider/         Leaf package - provider interface and types (zero deps on root)
  output/           Structured output (Object, Array, Choice, JSON) with schema validation
  fallback/         Automatic model failover wrapper
  providers/
    anthropic/      Separate Go module - Anthropic/Vertex provider implementation
    openai/         Separate Go module - OpenAI Responses API provider implementation
    bedrock/        Separate Go module - AWS Bedrock Converse provider implementation
    grafana/        Separate Go module - Grafana Cloud hosted provider implementation
  docs/             Narrative documentation (concepts, guides, providers, best-practices)
  examples/         Runnable example programs - each a self-contained Go module
```

Three event layers: `provider.StreamPart` (raw) -> `TextStreamPart`
(orchestration) -> `UIMessageChunk` (SSE wire format).

## Documentation Strategy

Docs live in three surfaces, each with one job. Keep content in the right place
to avoid README bloat and godoc drift. Full rules: [CONTRIBUTING.md](CONTRIBUTING.md).

- **godoc** (`doc.go` / per-symbol) owns the **API reference** -- signatures,
  option lists, struct fields. Authoritative; never duplicate it elsewhere.
- **Root `README.md`** owns the **landing page** -- pitch, install, ONE quick
  start, and a router into `docs/`. Keep it ~120-150 lines.
- **`docs/`** owns **concepts and guides** -- the "why" and "how do I X". Plain
  GitHub markdown, no frontmatter; navigation via `docs/README.md` index plus
  per-page `Prev/Up/Next` footers.

The drift boundary: "what's the signature / options?" -> godoc; "why/how?" ->
`docs/`; "convince me + run once" -> README. `docs/` pages link to pkg.go.dev
rather than reproducing signatures or option tables.

User-facing docs are centralized in `docs/` -- avoid per-module `README.md`
files for user-facing setup/behavior; fold that content into the right `docs/`
page (`doc.go` stays as the godoc API reference). Non-user-facing tooling
READMEs (e.g. under `test/`) may stay co-located. Runnable examples go under
`examples/` as self-contained, outcome-oriented modules. They must `go build`
and include deterministic credential-free tests
(CI: `mise run build-examples` and `mise run test-examples`).

## Code Style

### Imports

Follow `goimports` convention: stdlib first, blank line, then external packages
sorted alphabetically. Named aliases only when necessary (e.g.
`vertexsdk "..."`).

### Naming

- **Types/functions**: PascalCase exported, camelCase unexported
- **Files**: snake_case (`convert_request.go`, `message_json.go`)
- **Constants**: PascalCase exported (`ChunkTextDelta`, `FinishReasonStop`),
  camelCase unexported (`defaultStreamBuffer`)
- **Sentinel errors**: `Err` prefix (`ErrWriterClosed`, `ErrAlreadyClosed`)
- **Constructors**: `New()`, `NewVertex()`, or descriptive (`CreateIDGenerator`)
- **JSON tags**: camelCase with `omitempty` where appropriate
- **Test functions**: `TestFunctionName_Scenario` with underscores for sub-cases;
  E2E tests prefixed `TestE2E`

### Error Handling

- Standard `(value, error)` returns; never panic — use graceful fallbacks or
  error propagation instead
- Wrap with context: `fmt.Errorf("marshaling chunk: %w", err)`
- Error messages lowercase, prefixed with package/context
- Sentinel errors via `errors.New("aisdk: ...")` with package prefix
- No custom error types; only sentinels and `fmt.Errorf` wrapping
- Stream errors captured and emitted as events, not fatal
- Unsupported features produce `Warning` structs, not errors

### Interfaces

- Sealed via unexported marker method: `textStreamPart()`, `contentPart()`, `message()`
- Consumers use type switches on concrete types, never type assertions
- Compile-time satisfaction checks in `*_test.go`: `var _ Interface = Concrete{}`

### Types and JSON

- `json.RawMessage` for opaque/passthrough JSON
- Custom `MarshalJSON`/`UnmarshalJSON` for polymorphic serialization
- Pointer types for optional fields (`*int`, `*float64`)
- Maps for extensibility: `map[string]json.RawMessage`
- **Typed string enums for discriminator fields**: Fields that take a fixed
  set of string values (e.g., `Type`, `State`, `SourceType`) use a named
  `string` type with typed constants. Never use bare `string` for these.
  Example: `type ToolChoiceType string` with `ToolChoiceAuto`, `ToolChoiceNone`,
  etc. This preserves JSON wire compatibility while adding compile-time safety.
  Use the typed constants at all usage sites -- never inline string literals.

### Functions

- Functional options pattern: `type Option func(*model)`
- Variadic optional params: `func Foo(required, opts ...OptionalConfig)`
- Value receivers for small immutable types; pointer receivers for mutable/large
- One-liner methods on single line: `func (m *model) Provider() string { return m.providerName }`

### Concurrency

- Goroutines + buffered channels (`make(chan T, 64)`)
- `sync.Mutex` for shared state, `sync/atomic.Bool` for flags
- `defer close(ch)` in goroutines to signal completion
- `select` with `ctx.Done()` for cancellation
- Channel drain pattern: `for range result.FullStream() {}`

### Documentation

- Doc comments on all exported symbols; `doc.go` for package-level overview
- Minimal inline comments, only for non-obvious logic
- No code comments unless explicitly requested

## Testing Conventions

- Tests in same package (white-box): `package aisdk`, not `package aisdk_test`
- Use `testify/assert` and `testify/require` for assertions
- `require` for preconditions that must pass (replaces `t.Fatal`):
  `require.NoError(t, err)`, `require.Len(t, x, 2)`, `require.NotNil(t, x)`
- `assert` for value checks (replaces `t.Errorf`):
  `assert.Equal(t, expected, got)`, `assert.Contains(t, slice, item)`
- Table-driven tests with anonymous struct slices and `t.Run()`
- Loop variable naming: `tc` or `tt`
- Group related cases under one top-level `Test*` function with subtests,
  rather than many individual `Test*` functions; name: `TestFunctionName_Scenario`
- Extract shared setup into small test helpers to keep table entries focused
  on what varies; use a `check func(t *testing.T, ...)` field for complex
  per-case assertions
- Hand-written mocks implementing interfaces (no mocking framework)
- Test helpers defined in `*_test.go` files within the same package
- Integration tests in `integration_test.go`, named `TestE2E*`

### Cross-Language Integration Coverage

Changes to SSE chunks, text streams, HTTP headers, response formats, or provider
stream parts must include a cross-language scenario in `test/integration/` when
they affect frontend wire behavior. Add a deterministic Go scenario under
`test/integration/testserver/` and a matching Vitest test under
`test/integration/`. SSE tests must parse with `parseJsonEventStream` and
`uiMessageChunkSchema`, then assert chunk fields and assembled messages. Run
`mise run test-integration`.

### Mock Pattern

```go
type mockModel struct {
    streamFunc func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error)
    callCount  int
}
// implement all LanguageModel methods, delegate DoStream to streamFunc
```

### Compile-Time Interface Checks

```go
var (
    _ TextStreamPart = StreamStart{}
    _ TextStreamPart = StreamFinishStep{}
)
```

## Dependencies

- Root module: `invopop/jsonschema` (schema generation),
  `santhosh-tekuri/jsonschema` (schema validation)
- Anthropic provider module: `anthropic-sdk-go`, GCP auth libraries
- Both modules: `stretchr/testify` (test assertions)
