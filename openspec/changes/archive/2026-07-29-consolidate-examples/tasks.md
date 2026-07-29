## 1. Agent chat example

- [x] 1.1 Create the self-contained `examples/agent-chat` module with Anthropic runtime setup, a reusable bounded agent, a deterministic typed tool, and a `useChat`-compatible HTTP handler.
- [x] 1.2 Add credential-free handler and orchestration tests covering invalid input, local tool execution, final text, UI-message SSE chunks, and stream termination.

## 2. Structured extraction example

- [x] 2.1 Create the self-contained `examples/structured-extraction` module around the alert-triage outcome with provider construction isolated from testable workflow logic.
- [x] 2.2 Add credential-free tests covering response-format configuration, valid typed output, validation failure, and rendered results.

## 3. Continuous integration coverage

- [x] 3.1 Add a `test-examples` mise task that discovers and tests every example module, and invoke it from the blocking short-test task.
- [x] 3.2 Add a deterministic agent-tool scenario to the integration test server and assert progressive tool state plus final text through the pinned `@ai-sdk/react` `useChat` hook.

## 4. Remove superseded examples and update documentation

- [x] 4.1 Remove `generate-text`, `streaming-cli`, `tools-agent`, `structured-output`, and `chat-server` after their replacement coverage exists.
- [x] 4.2 Rewrite `examples/README.md` as an outcome-oriented two-example index and update all README, docs, contributor, agent-guidance, and run-command references.

## 5. Validation

- [x] 5.1 Run formatting, example tests, example builds, short tests, integration tests, and documentation lint; fix all failures.
- [x] 5.2 Validate the OpenSpec change and inspect the final diff for scope, stale links, generated artifacts, and dependency drift.
