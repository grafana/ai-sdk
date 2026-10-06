## Why

The repository's five runnable examples expose mostly isolated SDK calls while repeating provider setup, module metadata, and content already taught in the README and guides. Consolidating them around recognizable application outcomes will reduce navigation and maintenance cost, while deterministic CI tests will verify behavior rather than compilation alone.

## What Changes

- Replace the separate chat server and tools agent with one Go `agent-chat` backend that combines a reusable agent, typed local tools, bounded multi-step execution, UI message history, and React-compatible streaming.
- Replace the API-named structured-output sample with a scenario-oriented `structured-extraction` example.
- Remove the standalone generate-text and streaming CLI programs; retain those focused techniques in the README and backend guide.
- Add credential-free behavioral tests for every runnable example using deterministic language-model fakes.
- Add a blocking example-test task to CI while retaining build verification.
- Extend the pinned `@ai-sdk/react` integration suite to cover agent tool states and the final streamed response.
- Update example, getting-started, guide, and contributor navigation to describe examples as complete application outcomes rather than a linear API curriculum.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `docs-structure`: Refine runnable-example requirements so examples are curated application outcomes, have deterministic behavioral tests, and are tested as well as built in CI.

## Impact

- Replaces the five modules under `examples/` with two self-contained Go modules.
- Updates `mise.toml` and the blocking CI workflow to run example tests.
- Updates the cross-language integration test server and React hook tests without changing the SDK's wire protocol.
- Updates links and narrative in `README.md`, `docs/`, `CONTRIBUTING.md`, and `AGENTS.md` where they describe the example set or validation contract.
- Does not change exported Go APIs, provider behavior, conformance fixtures, or the registered upstream parity baseline.
