## Purpose

Define the cross-language integration test infrastructure for validating Go-generated SSE and text streams with the upstream TypeScript frontend tooling.

## Requirements

### Requirement: Go test server with scenario-based routing
The system SHALL provide a Go HTTP server binary (`test/integration/testserver/`) that serves deterministic, mock-model-backed responses at `/scenario/:name` endpoints. Each scenario SHALL produce a reproducible stream (SSE or text) without calling any external LLM provider. The server SHALL expose a `GET /health` endpoint that returns HTTP 200 when ready.

#### Scenario: Server starts and responds to health check
- **WHEN** the test server binary is executed
- **THEN** it starts an HTTP server on a dynamically-assigned port, prints the port to stdout, and responds with HTTP 200 to `GET /health`

#### Scenario: SSE scenario returns valid stream
- **WHEN** a client sends `POST /scenario/simple-text` with a JSON body
- **THEN** the server responds with `Content-Type: text/event-stream`, header `x-vercel-ai-ui-message-stream: v1`, a sequence of `data: {JSON}\n\n` lines, and a final `data: [DONE]\n\n`

#### Scenario: Text stream scenario returns plain text
- **WHEN** a client sends `POST /scenario/text-stream` with a JSON body
- **THEN** the server responds with `Content-Type: text/plain; charset=utf-8` and raw text chunks

#### Scenario: Unknown scenario returns 404
- **WHEN** a client sends a request to `/scenario/nonexistent`
- **THEN** the server responds with HTTP 404

### Requirement: Vitest harness with Go server lifecycle management
The system SHALL provide a Vitest global setup that builds the Go test server binary, spawns it as a child process, waits for the health endpoint to respond (with timeout), exposes the server base URL to tests, and kills the process on teardown.

#### Scenario: Global setup starts server before tests
- **WHEN** vitest runs the global setup
- **THEN** the Go test server binary is compiled (if not cached), spawned as a child process, and the base URL is written to a `.test-server-url` file readable by all test files via the `getServerUrl()` helper

#### Scenario: Global teardown stops server after tests
- **WHEN** vitest completes all tests
- **THEN** the Go test server process is terminated and cleaned up

#### Scenario: Setup fails gracefully if Go build fails
- **WHEN** the Go test server fails to compile
- **THEN** vitest reports a clear error and does not attempt to run tests

### Requirement: SSE wire format validation tests
The system SHALL include Vitest tests that fetch SSE responses from the Go test server and validate them using the upstream `parseJsonEventStream` function with `uiMessageChunkSchema` from the `ai` package.

#### Scenario: Simple text SSE stream parses without errors
- **WHEN** the test fetches `/scenario/simple-text` and pipes the response body through `parseJsonEventStream({ stream, schema: uiMessageChunkSchema })`
- **THEN** all chunks parse successfully and the stream contains at least `start`, `text-delta`, and `finish` chunk types

#### Scenario: SSE chunks produce valid UIMessage via readUIMessageStream
- **WHEN** the parsed chunk stream is fed to `readUIMessageStream`
- **THEN** the resulting messages array contains an assistant message with a text part matching the expected content

### Requirement: Text stream validation tests
The system SHALL include Vitest tests that fetch text stream responses from the Go test server and validate that they produce valid partial JSON consumable by `useObject`'s parsing approach.

#### Scenario: Text stream produces valid accumulated text
- **WHEN** the test fetches `/scenario/text-stream` and accumulates the response body as text
- **THEN** the accumulated text is valid JSON matching the expected object shape

### Requirement: TypeScript CLI for ad-hoc stream testing
The system SHALL provide a CLI tool (`test/cli/`) that connects to a given URL, parses the response as either SSE or text stream (auto-detected from Content-Type), and prints each parsed chunk to stdout in a human-readable format.

#### Scenario: CLI parses SSE stream
- **WHEN** the user runs `pnpm --filter @ai-sdk/test-cli start -- --url http://localhost:PORT/scenario/simple-text`
- **THEN** the CLI connects, parses the SSE stream using `parseJsonEventStream`, and prints each `UIMessageChunk` to stdout as formatted JSON

#### Scenario: CLI parses text stream
- **WHEN** the user runs the CLI with a URL that returns `Content-Type: text/plain`
- **THEN** the CLI prints each text chunk as it arrives

#### Scenario: CLI reports connection errors
- **WHEN** the target URL is unreachable
- **THEN** the CLI prints a clear error message and exits with a non-zero code

### Requirement: CI integration as separate job
The system SHALL add a GitHub Actions job `integration-test` in `.github/workflows/ci.yml` that sets up Go and Node.js, builds the test server, installs TS dependencies, and runs the Vitest integration suite.

#### Scenario: Integration tests run in CI on pull requests
- **WHEN** a pull request is opened or updated
- **THEN** the `integration-test` job runs in parallel with the existing `ci` job

#### Scenario: Integration tests run in CI on push to main
- **WHEN** code is pushed to `main`
- **THEN** the `integration-test` job runs

### Requirement: mise tasks for local development
The system SHALL add mise tasks for running the integration tests locally.

#### Scenario: Run integration tests via mise
- **WHEN** a developer runs `mise run test-integration`
- **THEN** the Go test server is built, TS dependencies are installed (if needed), and vitest runs the integration suite

#### Scenario: Run CLI via mise
- **WHEN** a developer runs `URL=http://localhost:8080/scenario/simple-text mise run test-cli`
- **THEN** the CLI tool runs against the given URL

### Requirement: Controlled and abortable integration streams
The Go integration test server SHALL provide deterministic request-scoped stream scenarios that expose an observable partial response and honor request cancellation. Controlled stream handlers and mock-model producers MUST stop blocked work when the request context is canceled and MUST NOT depend on mutable state shared between tests.

#### Scenario: Controlled UI stream exposes lifecycle phases
- **WHEN** a React chat hook requests the controlled UI-stream scenario
- **THEN** the server flushes an initial response phase before holding the stream open
- **AND** the server closes the stream in a separate bounded phase when it has not been canceled

#### Scenario: Controlled text stream exposes partial completion
- **WHEN** a React completion hook requests the controlled text-stream scenario
- **THEN** the server flushes a deterministic partial completion before holding the response open
- **AND** content scheduled after the hold is not part of the partial completion

#### Scenario: Cancellation releases server work
- **WHEN** a hook aborts a controlled stream after observing its partial response
- **THEN** the handler and any mock-model producer stop waiting when the request context is canceled
- **AND** the scenario does not require a global controller or cross-request execution flag

### Requirement: React hook lifecycle and failure integration coverage
The integration suite SHALL exercise Go HTTP responses through the public hooks from the registered `@ai-sdk/react` package and assert intermediate state, terminal state, callback arguments, and retained partial data for the covered lifecycle and failure contracts.

#### Scenario: Chat status ordering succeeds
- **WHEN** `useChat` consumes a successful phased Go UI stream
- **THEN** its observed status history contains `submitted`, `streaming`, and `ready` in that order

#### Scenario: Chat HTTP and stream errors surface through the hook
- **WHEN** `useChat` receives either a non-success HTTP response or a Go UI stream error chunk
- **THEN** the hook exposes the expected error
- **AND** its terminal status is `error`

#### Scenario: Stopped chat retains partial output
- **WHEN** `stop()` is called after `useChat` has rendered partial assistant text
- **THEN** the hook returns to `ready`
- **AND** the partial text remains present without later server text

#### Scenario: Chat history exposes a step boundary
- **WHEN** `useChat` consumes the deterministic multi-step tool scenario
- **THEN** immutable message snapshots expose the completed first-step state before the next-step output
- **AND** the final message preserves the expected tool output and final text

#### Scenario: Approved tool response resumes chat
- **WHEN** a pending tool part is approved through `addToolApprovalResponse` and automatically resubmitted
- **THEN** message history transitions that part from `approval-requested` to `approval-responded`
- **AND** the response preserves `approved: true` and its reason
- **AND** the resumed Go flow produces the expected `output-available` tool state

#### Scenario: Denied tool response resumes without execution
- **WHEN** a pending tool part is denied through `addToolApprovalResponse` and automatically resubmitted
- **THEN** message history transitions that part from `approval-requested` to `approval-responded`
- **AND** the response preserves `approved: false` and its reason
- **AND** the resumed Go flow exposes `output-denied` without a successful tool output

#### Scenario: Completion error resets lifecycle state
- **WHEN** `useCompletion` receives a non-success HTTP response
- **THEN** `onError` is called exactly once with the expected `Error`
- **AND** `onFinish` is not called
- **AND** the hook exposes the error and resets `isLoading` to false

#### Scenario: Stopped completion retains partial output
- **WHEN** `stop()` is called after `useCompletion` has rendered a partial completion
- **THEN** the abort does not invoke `onError`
- **AND** `isLoading` becomes false
- **AND** the partial completion remains present without later server text

#### Scenario: Final object fails schema validation
- **WHEN** `useObject` finishes consuming valid JSON that does not match its configured schema
- **THEN** `onFinish` is called exactly once with `object: undefined`
- **AND** the same callback result contains an `Error`
