## Why

Issue #213: Go loses tool fields during UI persistence/assembly and changes resumed
model input. Initial SSE schema acceptance does not prove persistence or resume.
The 7.0.109 design was revalidated against the merged 7.0.116 baseline.

## What Changes

- Preserve represented tool/approval fields, optional presence and isolated resume.
- **BREAKING**: tool `ErrorText` and approval `Reason` become string pointers.
- Match filtering, raw-input/denial/metadata selection and provider tool-message
  coalescing; add text/file data conversion and retain required empty model text.
- Validate/normalize isolated history before Agent invocation; prove persistence,
  conversion and real hook resume against pinned TypeScript APIs.

## Capabilities

### New Capabilities

- `ui-tool-state-persistence`: tool/approval JSON presence and isolation.

### Modified Capabilities

- `ui-message-stream-reader`: field retention and cloned initial-message resume.
- `ui-message-conversion`: tool-state projection and optional data conversion.
- `agent-tool-loop`: represented state/schema validation and terminal normalization.

## Impact

Root UI/Agent orchestration, scoped provider text encoding, tests and guides.
Additive APIs: `WithUIMessageReaderInitialMessage` and `WithConvertDataPart`.
Existing wire names/framing and reader error contracts remain; lifecycle work is
#181. No provider redesign, new dependencies, general validator or application
metadata/data-schema configuration. Support boundaries remain explicit.
