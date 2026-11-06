# Gateway request capabilities

## Purpose

Prevent strict protocol adapters from accepting options that native request
builders discard, while preserving default provider parity.

## Requirements

### Requirement: Explicit request capability rejection

Provider CallOptions SHALL offer a Go-only RejectUnsupported opt-in. Native
OpenAI Responses, Anthropic and compatible Chat providers SHALL reject with
ErrUnsupportedFunctionality before I/O when request construction yields an
unsupported warning and the opt-in is enabled. Compatibility warnings SHALL
remain allowed. Default calls SHALL retain existing warning behavior.

#### Scenario: Unsupported explicit zero
- **WHEN** strict Chat calls Responses with frequency_penalty zero
- **THEN** native generation and streaming both fail before network I/O
- **AND** the adapter returns HTTP 400.

#### Scenario: Model-dependent unsupported sampling
- **WHEN** strict Chat requests sampling controls rejected by the native model
- **THEN** request-builder capability warnings cause rejection before I/O.

#### Scenario: ProviderWire keeps its warning contract
- **WHEN** a default V4 caller submits an unsupported option
- **THEN** its existing provider warning behavior is unchanged
- **AND** the strict opt-in is absent from wire JSON.
