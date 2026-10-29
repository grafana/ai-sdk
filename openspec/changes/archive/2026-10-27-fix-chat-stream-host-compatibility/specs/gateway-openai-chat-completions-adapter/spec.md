## ADDED Requirements

### Requirement: Response writers without socket deadlines

The handler SHALL flush SSE through the response controller and accept a
writer that does not support socket write deadlines. Where supported, write
deadlines SHALL honor the host request deadline when present and react
to cancellation. Unsupported write deadlines SHALL NOT alone reject a valid
stream; host transport cancellation remains required for plugin writers.

#### Scenario: Flush-capable writer without deadline support
- **WHEN** the response writer supports flushing but reports unsupported write
  deadlines
- **THEN** a valid stream is flushed and completes with `[DONE]`.

#### Scenario: Supported socket deadlines
- **WHEN** the writer supports socket deadlines
- **THEN** the handler retains host deadline and cancellation behavior without adding or
  clearing a host timeout.

### Requirement: Represented Anthropic start metadata

The adapter SHALL discard a custom part whose kind is exactly
`anthropic.message_start` without emitting its raw payload or treating it as
represented progress. Canonical response metadata SHALL remain the source of
native identity. Other custom kinds SHALL continue to fail safely without a
successful stream terminator.

#### Scenario: Anthropic start metadata beside canonical identity
- **WHEN** canonical response metadata and an `anthropic.message_start` custom
  part precede valid text and finish
- **THEN** the adapter preserves native identity, emits the text and completes
  with `[DONE]` without exposing the custom payload.

#### Scenario: Unknown custom kind
- **WHEN** a part uses `anthropic.other` or another provider's custom kind
- **THEN** the adapter emits a fixed error where writable and omits `[DONE]`.
