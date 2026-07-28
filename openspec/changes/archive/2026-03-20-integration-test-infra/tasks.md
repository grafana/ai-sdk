## 1. Project scaffolding

- [x] 1.1 Create `test/` directory structure: `test/integration/`, `test/integration/testserver/`, `test/cli/`
- [x] 1.2 Create root `test/package.json` with pnpm workspaces pointing to `integration` and `cli`
- [x] 1.3 Create `test/integration/package.json` with dependencies: `ai`, `@ai-sdk/provider-utils`, `vitest`, `typescript`
- [x] 1.4 Create `test/cli/package.json` with dependencies: `ai`, `@ai-sdk/provider-utils`, `typescript`
- [x] 1.5 Create `test/integration/tsconfig.json` and `test/cli/tsconfig.json`
- [x] 1.6 Create `test/integration/vitest.config.ts` with global setup file reference and a generous test timeout (server spawn)

## 2. Go test server

- [x] 2.1 Create `test/integration/testserver/main.go` with HTTP server, dynamic port allocation, port printed to stdout, and `GET /health` endpoint
- [x] 2.2 Add scenario router: `POST /scenario/:name` dispatches to registered scenario handlers, returns 404 for unknown names
- [x] 2.3 Implement `simple-text` scenario: mock model returns a single text response via `StreamText` + `PipeUIMessageStreamToResponse`
- [x] 2.4 Implement `text-stream` scenario: mock model returns structured JSON text via `StreamText` + `WriteTextStream` (for `useObject`)
- [x] 2.5 Verify Go test server builds and runs: `go build ./test/integration/testserver && ./testserver`

## 3. Vitest harness (global setup/teardown)

- [x] 3.1 Create `test/integration/global-setup.ts`: build Go binary (`go build`), spawn process, parse port from stdout, poll `/health` with retry+timeout, set `globalThis.__TEST_SERVER_URL__`
- [x] 3.2 Create `test/integration/global-teardown.ts`: kill spawned Go process, clean up binary (combined into global-setup.ts teardown export)
- [x] 3.3 Create `test/integration/helpers.ts`: shared utilities — `fetchScenario(name)` that sends POST to the test server scenario endpoint and returns the raw Response

## 4. Integration tests

- [x] 4.1 Create `test/integration/sse-wire-format.test.ts`: fetch `simple-text` scenario, pipe response through `parseJsonEventStream` with `uiMessageChunkSchema`, assert all chunks parse and expected types (`start`, `text-delta`, `finish`) are present
- [x] 4.2 Create `test/integration/sse-message-assembly.test.ts`: feed parsed chunks into `readUIMessageStream`, assert resulting UIMessage has assistant role and text part with expected content
- [x] 4.3 Create `test/integration/text-stream.test.ts`: fetch `text-stream` scenario, accumulate body text, assert valid JSON matching expected shape

## 5. TypeScript CLI

- [x] 5.1 Create `test/cli/src/index.ts`: parse CLI args (`--url`, `--method`), fetch the URL, auto-detect Content-Type, parse SSE via `parseJsonEventStream` or read text chunks, print each chunk to stdout
- [x] 5.2 Add `bin` entry in `test/cli/package.json` so it can be invoked via `pnpm --filter @ai-sdk/test-cli start`
- [x] 5.3 Test CLI manually against the Go test server

## 6. Makefile and CI

- [x] 6.1 Add Makefile target `test-integration`: installs TS deps (`cd test && pnpm install`), builds Go server, runs vitest
- [x] 6.2 Add Makefile target `test-cli`: runs the CLI with a `URL` variable
- [x] 6.3 Add `integration-test` job to `.github/workflows/ci.yml`: setup Go + Node.js + pnpm, run `make test-integration`
- [x] 6.4 Verify CI workflow runs both jobs in parallel (no `needs: ci` dependency)
