## Why

The Go AI SDK produces SSE streams (`UIMessageChunk`) and raw text streams that must be wire-compatible with `@ai-sdk/react` hooks (`useChat`, `useCompletion`, `useObject`). Today, wire compatibility is enforced by convention and code review, not by automated tests. A single field name typo, missing header, or chunk ordering bug in the Go SDK would silently break the TypeScript frontend. We need cross-language integration tests that prove the Go server's output is consumable by the real upstream TypeScript parsing and message-processing code.

## What Changes

- Add a **Go test server** (`test/integration/testserver/`) that uses mock models to produce deterministic SSE and text stream responses for predefined scenarios.
- Add a **Vitest test suite** (`test/integration/`) that starts the Go server, sends requests, parses responses using upstream `@ai-sdk/provider-utils` parsing functions and `ai` message processing, and asserts correctness at the transport/parsing layer (no React rendering).
- Add a **TypeScript CLI** (`test/cli/`) for ad-hoc manual testing against any Go server endpoint — displays parsed stream chunks in real time.
- Add a **separate CI job** in `.github/workflows/ci.yml` that installs both Go and Node.js and runs the integration test suite.
- Add a **Makefile target** `test-integration` to run the full cross-language suite locally.

## Capabilities

### New Capabilities
- `integration-testing`: Cross-language integration test infrastructure — Go test server, Vitest harness, CI job, and ad-hoc CLI for verifying wire compatibility between the Go AI SDK Core and TypeScript AI SDK UI.

### Modified Capabilities

_None. This is pure test infrastructure; no existing spec-level behavior changes._

## Impact

- **New files**: `test/integration/` (Go server + TS tests), `test/cli/` (TS CLI), Makefile additions, CI workflow additions.
- **Dependencies**: Adds Node.js/pnpm as a CI dependency (new `actions/setup-node` step in a separate job). Adds `@ai-sdk/provider-utils`, `ai`, `vitest` as dev dependencies in the test workspace.
- **No production code changes**. No changes to the Go SDK's public API or wire format.
