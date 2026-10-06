# Cross-Language Integration Tests

This directory contains integration tests that verify wire compatibility between the Go AI SDK and the upstream TypeScript AI SDK (`@ai-sdk/react` hooks like `useChat`, `useCompletion`, `useObject`).

The tests work by running a Go HTTP server that produces SSE and text streams using the Go SDK, then parsing those streams with the real upstream TypeScript parsing functions to prove compatibility.

## Prerequisites

- **Go 1.26+**
- **Node.js 22+**
- **pnpm 10+**
- **mise**

## Quick Start

From the repository root:

```bash
# Run the full integration test suite
mise run test-integration

# Inspect a scenario's stream output (auto-starts the Go server)
SCENARIO=simple-text mise run test-cli

# Or point the CLI at an already-running server
URL=http://localhost:8080/scenario/simple-text mise run test-cli
```

## Structure

```
test/
├── integration/
│   ├── testserver/           # Go test server (mock scenarios)
│   │   ├── main.go           # HTTP server with /health and /scenario/:name routing
│   │   ├── scenario_simple_text.go   # SSE stream scenario (StreamText + PipeUIMessageStreamToResponse)
│   │   ├── scenario_text_stream.go   # Plain text stream scenario (StreamText + WriteTextStream)
│   │   └── go.mod
│   ├── global-setup.ts       # Vitest global setup: builds/spawns Go server, teardown on exit
│   ├── helpers.ts            # Shared test utilities (fetchScenario, getServerUrl)
│   ├── vitest.config.ts
│   ├── sse-wire-format.test.ts       # Validates SSE chunks parse via parseJsonEventStream + uiMessageChunkSchema
│   ├── sse-message-assembly.test.ts  # Validates chunks assemble into UIMessage via readUIMessageStream
│   └── text-stream.test.ts          # Validates plain text stream produces valid JSON
├── cli/
│   └── src/
│       └── index.ts          # CLI tool for ad-hoc stream inspection
├── package.json              # pnpm workspace root
└── pnpm-workspace.yaml
```

## How It Works

1. **Vitest global setup** compiles the Go test server binary and spawns it on a random port.
2. The server exposes `/scenario/:name` endpoints, each producing a deterministic stream using mock models.
3. **TypeScript tests** fetch these endpoints and parse responses using the upstream `parseJsonEventStream` and `uiMessageChunkSchema` from `@ai-sdk/provider-utils` and `ai`.
4. Tests assert that all chunks parse successfully, expected chunk types are present, and assembled messages match expected content.

## Adding a New Scenario

1. Create `test/integration/testserver/scenario_<name>.go` with a mock model and handler function.
2. Register the scenario in `init()` via `registerScenario("<name>", handler)`.
3. Create a corresponding `test/integration/<name>.test.ts` that calls `fetchScenario("<name>")` and asserts the response.

## CLI Tool

The CLI auto-detects SSE vs plain text from the `Content-Type` header and pretty-prints each parsed chunk. It can auto-start the Go test server or connect to an existing one:

```bash
# Auto-start server, run scenario, shut down (easiest)
SCENARIO=simple-text mise run test-cli
SCENARIO=text-stream mise run test-cli

# Or connect to an already-running server
URL=http://127.0.0.1:8080/scenario/simple-text mise run test-cli

# Direct pnpm invocation
cd test && pnpm --filter @ai-sdk/test-cli start -- --scenario simple-text
cd test && pnpm --filter @ai-sdk/test-cli start -- --url http://127.0.0.1:8080/scenario/simple-text
```
