## Why

Issue #213 remains valid against the registered `ai@7.0.109` / `@ai-sdk/react@4.0.112` baseline: chunks can carry tool information that Go assembly and persistence discard, and resumed model messages differ from upstream. Schema-valid initial SSE is insufficient evidence of correct persisted history, approval resumption, or preliminary-output filtering.

## What Changes

- Retain title, tool metadata, raw-input distinctions, preliminary status, and supported approval data on static and dynamic tool parts through JSON and immutable reader snapshots; decoded providerExecuted false clears prior true instead of inheriting it.
- **BREAKING**: change persisted tool `ErrorText` and `ToolApproval.Reason` to optional string pointers so missing and explicitly empty strings remain distinguishable; migrate repository callers.
- Add a cloned initial-message reader option for resuming assistant tool parts without duplicates. Non-assistant seeds retain their supplied ID but not their parts/metadata. Keep the current progressive/blocking reader error contracts; error/cancellation/lifecycle API work remains in #181.
- Correct conversion defaults, preliminary filtering, denied fallback text, raw-input fallback and source-specific provider metadata selection; add a text/file-only data-part converter. Preserve existing `ToModelOutput`, custom content, approval placement, file references and filename presence.
- Extend existing pre-Agent validation with state constraints, configured static-tool schemas, and the pinned Agent-specific normalization of obsolete terminal static history to dynamic parts.
- Add differential persistence/conversion tests against the pinned TypeScript APIs and a real hook-level persisted-resume scenario.

## Capabilities

### New Capabilities

- `ui-tool-state-persistence`: persisted static/dynamic tool and approval field representation, JSON presence and round-trip contracts.

### Modified Capabilities

- `ui-message-stream-reader`: retain tool fields and resume from an isolated initial assistant message at existing write points, without changing the split error contracts.
- `ui-message-conversion`: match pinned tool-state filtering, raw-input/denial/metadata semantics and add optional data conversion.
- `agent-tool-loop`: validate and normalize persisted UI messages before Agent/provider invocation with configured tool schemas.

## Impact

Root package seams: `message.go`, `message_json.go`, `chunk.go`, `ui_message_reader.go`, `convert.go`, and `agent.go`; their tests, `test/conformance/ui/`, `test/integration/testserver/`, and Vitest/hook tests. Proposed additive APIs are `WithUIMessageReaderInitialMessage` and `WithConvertDataPart`; pointer migrations are source-breaking but retain existing wire names. Chunk codec work is limited to existing optional-field presence and the registered `approvalDescriptor` field, not SSE framing (#180), new chunk types, or provider producer redesign.

No baseline or dependency upgrade is needed. This is one independently green root-module work package, not a cumulative provider stack. `PARITY.md` changes only if delivered evidence/support boundaries change. Metadata/data schema configuration and a general exported validator are not introduced; their absence is an explicit coverage boundary, not full `validateUIMessages` parity. This proposal is subject to design/API review, not authorization inferred from issue registration.
