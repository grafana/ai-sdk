## Why

The parity governance workflow now defines the right review posture, but several
coverage signals are still manual or fragmented. Conformance tooling, frontend
integration tests, and CLI tooling can drift to different upstream AI SDK
versions, fixture coverage gaps are not automatically reported, and the
conformance harness cannot yet express several important upstream-visible
options.

## What Changes

- Extend upstream baseline validation to every test package that consumes
  `ai` or `@ai-sdk/*` packages.
- Add an automated parity coverage inventory that checks fixture artifacts,
  provider fixture indexes, and local upstream fixture import drift.
- Expand conformance config support for `toolChoice`, `activeTools`,
  `streamOptions`, tool `providerOptions`, and tool error simulation.
- Add hook-level frontend interop tests for `@ai-sdk/react` surfaces.
- Add an API-shape drift report for provider V4 discriminator values so new
  upstream content, stream, finish, or tool result variants are visible.

## Impact

- Affected docs and specs: conformance testing and upstream parity governance.
- Affected tooling: `mise.toml`, conformance TypeScript tools, integration test
  dependencies, and conformance config schema.
- Affected tests: conformance harness tests and frontend integration tests.
