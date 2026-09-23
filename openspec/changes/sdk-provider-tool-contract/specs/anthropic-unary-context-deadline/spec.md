## ADDED Requirements

### Requirement: Bounded unary Anthropic calls use the request context deadline
For direct Anthropic `DoGenerate`, a future caller-supplied context deadline SHALL become a default SDK request timeout before the native non-streaming token-estimate check. The context SHALL continue to bound the call; the provider SHALL not silently reduce `max_tokens` or reject an otherwise valid large-default-token request solely because the SDK lacks a request timeout. Explicit caller-configured SDK request timeouts SHALL retain precedence over the context-derived default. Without a deadline or explicit timeout, the existing SDK guard SHALL remain in effect. Streaming behavior SHALL be unchanged.

#### Scenario: Deadline permits large default token budget
- **WHEN** a unary call for a model whose default `max_tokens` exceeds the SDK's non-streaming threshold has a future request context deadline and no explicit request timeout
- **THEN** the native Anthropic HTTP request SHALL be attempted with the original token budget; it SHALL remain cancelable by that context

#### Scenario: Explicit timeout wins
- **WHEN** the model has an explicit SDK request timeout and the unary call also has a longer context deadline
- **THEN** the explicit timeout SHALL govern the attempt while the context remains an outer cancellation bound

#### Scenario: No deadline preserves SDK guard
- **WHEN** the same large-default-token unary call has no context deadline or explicit SDK request timeout
- **THEN** the SDK's existing non-streaming guard MAY reject it before HTTP I/O rather than silently introducing an unbounded provider call

#### Scenario: Expired deadline does not attempt I/O
- **WHEN** the request context deadline has already elapsed
- **THEN** `DoGenerate` SHALL return a recognizable context deadline error without issuing native HTTP
