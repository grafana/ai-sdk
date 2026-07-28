## Why

As a Go port of Vercel's AI SDK, our primary correctness risk is behavioral divergence from the upstream TypeScript implementation. Current unit tests validate internal logic against hand-written mocks, but they only cover what we explicitly thought to test. Real bugs -- missing switch cases for block types, dropped events, wrong field mappings -- share a common cause: incomplete port coverage that hand-written assertions can't catch. We need the upstream TypeScript SDK as an oracle, so any divergence is surfaced automatically without requiring us to anticipate what to check for.

## What Changes

- Introduce a fixture-replay conformance test infrastructure that validates Go SDK output against TypeScript SDK output for the same recorded provider API responses
- Organize conformance tests per-provider (matching Go SDK module structure), with each provider having `upstream/` fixtures copied from the Vercel SDK and `recorded/` fixtures captured from real APIs
- Add a TypeScript recording tool that captures real provider HTTP responses and generates expected UIMessageChunk output using the upstream SDK as the reference (operates on `recorded/` fixtures only)
- Add a TypeScript generation tool that regenerates expected output from existing fixtures (no API keys needed, operates on both `upstream/` and `recorded/` fixtures) when the upstream SDK is updated
- Add Go conformance tests that replay provider response fixtures through our full pipeline (provider -> StreamText -> ToUIMessageStream) and compare output exactly against TypeScript-generated expected output
- Support multi-step tool calling scenarios with ordered per-step fixtures and declarative mock tool results
- Adopt upstream's `.chunks.txt` fixture format (one JSON object per line, SSE framing added at serve time) enabling direct copy of upstream fixtures

## Capabilities

### New Capabilities

- `conformance-testing`: Fixture-replay conformance test suite that validates Go SDK output against TypeScript SDK output, covering fixture recording, expected output generation, per-test configuration, Go replay infrastructure, per-provider test organization, upstream fixture importing, and exact output comparison

### Modified Capabilities

(none)

## Impact

- New `test/conformance/` directory with per-provider subdirectories, TypeScript tooling, Go tests, fixtures, and a separate Go module (needs to import both `aisdk` and provider modules)
- New dev dependencies: TypeScript tooling needs `ai`, `@ai-sdk/anthropic`, YAML parser
- CI: conformance tests run as a separate job gated behind a build tag; no API keys needed (uses committed fixtures)
- Existing test infrastructure is not modified -- conformance tests complement existing unit, E2E, and cross-language integration tests
- Adding a new provider requires: create provider directory, copy/record fixtures, write a `conformance_test.go` that wires the provider to the shared runner
