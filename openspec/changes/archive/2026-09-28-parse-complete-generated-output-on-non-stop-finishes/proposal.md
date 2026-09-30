## Why

`GenerateText` and `ToolLoopAgent.Generate` currently skip complete structured-output parsing on every non-`stop` finish, losing valid output and hiding invalid output. The registered upstream `ai@7.0.109` (`4e8c387622ee1bb0d55841664416d38754d5c9a3`) instead parses `stop` finishes and nonempty text for every finish except `tool-calls`; the issue's `7.0.107` citation is an older reference, not this project's baseline.

## What Changes

- Align final-step complete-output parsing for both generated operations with the registered upstream finish/text condition: parse on `stop` even with empty text, or on non-`tool-calls` finishes with nonempty text; otherwise leave output and parse error unset.
- Preserve the Go result/error contract: valid content becomes `Output`, parse/schema failures become `OutputError` wrapping `ErrNoObjectGenerated` while raw text and finish reason remain available. `output.Value[T]` continues to report missing or invalid output through an `OutputAccessor` wrapper of the generated result; raw JSON `null` still parses successfully to nil but is reported as missing by that accessor.
- Leave `StreamText`/agent streaming completion and partial/array-element delivery intact; update the obsolete stop-only structured-output spec and length-finish generation test.
- Cover generated operations across `stop`, `length`, `content-filter`, `error`, `other`, and `tool-calls` with valid (non-null for raw JSON), invalid, and empty object, array, choice, and raw JSON responses, using focused synthetic provider streams. Retain the recorded length-finish conformance fixture unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `structured-output`: Correct the generated-operation final-output parsing requirement and its scenarios while retaining streaming semantics.

## Impact

Core orchestration in `options.go`, `agent.go`, and `streamtext.go` plus focused root tests and the `structured-output` specification. No new API, dependencies, provider changes, frontend wire changes, or migration. This is parity-sensitive core structured output and the agent entry point; both are classified `mixed` in `test/conformance/PARITY.md`. Provider recorded input is evidence and must not be fabricated or modified. No legacy `generateObject`/`streamObject` or `repairText` API is part of this change.
