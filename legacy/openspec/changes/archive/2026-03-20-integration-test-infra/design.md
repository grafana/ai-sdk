## Context

The Go AI SDK (`grafana/ai-sdk`) is a port of Vercel's TypeScript AI SDK. It produces two wire formats:

1. **SSE stream** (`PipeUIMessageStreamToResponse`): `data: {JSON}\n\n` chunks terminated by `data: [DONE]\n\n`, with headers `Content-Type: text/event-stream` and `x-vercel-ai-ui-message-stream: v1`. Consumed by `useChat` and `useCompletion` (data mode).
2. **Text stream** (`WriteTextStream`): Raw UTF-8 text chunks with `Content-Type: text/plain; charset=utf-8`. Consumed by `useObject` and `useCompletion` (text mode).

Today, wire compatibility is enforced by code review against the upstream TypeScript implementation. The existing Go integration tests (`integration_test.go`) use `httptest.Recorder` to validate SSE output structure, but never feed the output through the actual TypeScript parsing stack.

The upstream TS SDK has two key parsing layers:
- `parseJsonEventStream` (`@ai-sdk/provider-utils`): SSE bytes → parsed JSON chunks validated against `uiMessageChunkSchema` (Zod).
- `processUIMessageStream` (`ai`): `UIMessageChunk` stream → assembled `UIMessage` objects with parts, tool calls, etc.

The CI runs on GitHub Actions (`ubuntu-latest` VM, not Docker). Currently Go-only — Node.js would be new.

## Goals / Non-Goals

**Goals:**
- Prove that Go SSE output is parseable by the upstream TypeScript `parseJsonEventStream` + `uiMessageChunkSchema` validation.
- Prove that parsed chunks produce correct `UIMessage` objects when processed by `processUIMessageStream`.
- Prove that Go text stream output is consumable as partial JSON (matching `useObject` expectations).
- Provide a CLI tool for ad-hoc manual testing and debugging of Go server output.
- Keep the test infrastructure minimal and easy to extend with new scenarios over time.
- Run in CI as a separate job alongside the existing Go-only job.

**Non-Goals:**
- React component rendering or DOM testing (no jsdom, no `@testing-library/react`).
- Testing the upstream hooks themselves — we trust they work; we only test that our wire output is compatible.
- Testing against real LLM providers — all scenarios use deterministic mock models.
- Achieving full scenario coverage in the first PR — infrastructure first, scenarios expand later.
- Browser-based E2E tests.

## Decisions

### D1: Test at the transport/parsing layer, not React hooks

**Decision:** Import and call `parseJsonEventStream` and `processUIMessageStream` directly in Vitest tests against an HTTP response from the Go server. No React rendering.

**Rationale:** The hooks (`useChat`, etc.) are thin wrappers around these parsing functions. Testing at the transport layer verifies wire compatibility without the complexity of jsdom, React lifecycle, and UI rendering. This is faster, more deterministic, and tests exactly the contract boundary.

**Alternative considered:** Full React hook rendering with `@testing-library/react` against the Go server — rejected because it adds significant complexity and tests upstream behavior we don't own.

### D2: Go test server as a standalone binary spawned by Vitest

**Decision:** Build a small Go `main.go` in `test/integration/testserver/` that starts an HTTP server with deterministic mock responses for each scenario. Vitest `globalSetup` spawns this binary, waits for a health check endpoint, and tears it down after tests.

**Rationale:** Process spawn is simpler than Docker (no docker-in-docker risk in CI), and the Go test server reuses existing SDK patterns (`mockModel`, `StreamText`, `PipeUIMessageStreamToResponse`, `WriteTextStream`).

**Alternative considered:** Docker container for the Go server — rejected because CI already runs on a bare VM and Docker adds unnecessary complexity.

### D3: Scenario-based routing in the Go test server

**Decision:** The Go server exposes `/scenario/:name` routes. Each scenario is a named function that produces a deterministic stream (e.g., `simple-text`, `tool-call`, `multi-step`, `text-stream-object`). Adding a new scenario means adding one function and registering a route.

**Rationale:** Makes it trivial to add new test cases. Each scenario is self-contained. The TS test can reference scenarios by name without coupling to Go internals.

### D4: Vitest as the test runner

**Decision:** Use Vitest (not Jest) for the TypeScript integration tests.

**Rationale:** Upstream AI SDK uses Vitest. Same tooling, same configuration patterns, easier to reference upstream test code. Vitest also has native ESM support which `ai` and `@ai-sdk/provider-utils` require.

### D5: Separate CI job with both Go and Node.js

**Decision:** Add a new job `integration-test` in `.github/workflows/ci.yml` that runs after the existing `ci` job. It sets up both Go and Node.js, builds the test server, installs TS dependencies, and runs vitest.

**Rationale:** Keeps the fast Go-only CI loop unchanged. Integration tests are slower and have different dependencies — separating them lets the Go job stay fast.

### D6: TS CLI for ad-hoc testing

**Decision:** A small TypeScript CLI in `test/cli/` that takes a URL, connects, parses the SSE/text stream using the same upstream functions, and pretty-prints each chunk. Usable against any running Go server (local dev, staging, etc.).

**Rationale:** Invaluable for debugging during development. Also serves as a living example of how the TS parsing works.

### D7: pnpm workspace for test packages

**Decision:** `test/` directory has its own `package.json` with pnpm as the package manager, matching the upstream AI SDK repo. Both `test/integration/` and `test/cli/` are pnpm workspaces within it.

**Rationale:** Keeps Node.js dependencies isolated from the Go module. Single `pnpm install` from `test/`.

## Risks / Trade-offs

**[Process orchestration complexity]** → Vitest globalSetup must spawn, health-check, and tear down the Go binary. Mitigated by a simple health endpoint (`GET /health` → 200) with a retry loop and timeout. The Go binary writes its port to stdout so vitest knows where to connect.

**[Upstream breaking changes]** → If `@ai-sdk/provider-utils` or `ai` change their parsing APIs, tests break. Mitigated by pinning dependency versions and updating intentionally. This is actually a feature — it alerts us to upstream changes that affect wire compatibility.

**[CI time increase]** → The integration job adds build + install + test time. Mitigated by running it in parallel with the Go-only job (not sequentially), and keeping the initial scenario count minimal.

**[Two package ecosystems in one repo]** → Go + Node.js in the same repo adds cognitive load. Mitigated by clear separation (`test/` is self-contained) and Makefile targets that abstract the commands.
