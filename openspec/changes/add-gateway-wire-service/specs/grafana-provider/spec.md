## ADDED Requirements

### Requirement: Strict remote response reads are bounded

When `WithStrictProviderWire()` is enabled, the Grafana provider SHALL default to 16 MiB for unary success, 1 MiB for non-2xx or invalid-content-type diagnostics, and 8 MiB for one complete SSE event. Positive options SHALL configure both cloud-auth and access-token constructors; zero, negative, or nil options SHALL fail construction. Existing constructor calls and HTTP-client precedence SHALL remain source compatible.

#### Scenario: Strict unary exact limit succeeds

- **WHEN** a valid strict unary body equals its configured limit
- **THEN** the provider SHALL decode it normally

#### Scenario: Strict unary limit plus one fails

- **WHEN** a strict unary body exceeds its configured limit by one byte
- **THEN** the provider SHALL return a non-retryable bounded protocol error

#### Scenario: Strict diagnostic read remains bounded

- **WHEN** a strict non-success or invalid-content-type body exceeds its diagnostic limit
- **THEN** the provider SHALL not retain the unbounded remainder

#### Scenario: Both strict constructors share limits

- **WHEN** equivalent limit options configure strict cloud-auth and access-token providers
- **THEN** their models SHALL enforce the same limits

### Requirement: Strict SSE event parsing is bounded

Strict Grafana mode SHALL parse SSE incrementally and apply its limit to the complete accumulated event, including prefixes, multiline data, line endings, and terminating blank line. Canonical strict events SHALL count exactly `data: `, JSON, and `\n\n`, matching the strict server. Final bytes returned with `io.EOF` SHALL be processed before clean completion.

#### Scenario: Strict SSE exact limit succeeds

- **WHEN** a complete valid strict event equals its configured limit
- **THEN** its stream part SHALL be decoded

#### Scenario: Strict SSE limit plus one fails

- **WHEN** a complete or unterminated strict event exceeds its limit
- **THEN** Grafana SHALL emit one non-retryable protocol error part and close

#### Scenario: Strict multiline size is aggregate

- **WHEN** individually small strict SSE lines exceed the limit together
- **THEN** the complete event SHALL fail the aggregate limit

#### Scenario: Strict final EOF bytes are decoded

- **WHEN** a valid final strict data event has no trailing newline
- **THEN** Grafana SHALL decode it before returning clean EOF

### Requirement: Legacy mode remains unchanged

Without `WithStrictProviderWire()`, Grafana SHALL retain its original legacy request and response codecs and original reader behavior. The new strict unary, diagnostic, and SSE limits SHALL NOT apply to legacy mode or change its `/language-model` path, headers, authentication, streaming selection, request bytes, retry behavior, or client precedence.

#### Scenario: Default request remains legacy

- **WHEN** a model call uses either constructor without `WithStrictProviderWire()`
- **THEN** its request and response behavior SHALL remain legacy compatible

#### Scenario: Legacy limit options do not bound reads

- **WHEN** a legacy client is constructed with one of the new response-limit options
- **THEN** its original unary, diagnostic, and SSE readers SHALL remain unchanged

### Requirement: Strict bidirectional codec opt-in remains explicit

Grafana SHALL expose only the binary `WithStrictProviderWire()` opt-in rather than a general provider-wire mode enum. Strict mode SHALL use only `gateway/providerwire/v4` for request encoding and unary, error, and SSE decoding, reject legacy-only response shapes, apply strict response limits, and preserve the existing model API. Changing the default or deleting legacy mode is deferred.

#### Scenario: Strict request uses V4 codec

- **WHEN** strict mode performs generate or stream
- **THEN** request and response conversion SHALL use the strict V4 package without legacy fallback

#### Scenario: Strict legacy response is rejected

- **WHEN** strict mode receives a legacy-only result or event
- **THEN** it SHALL return a non-retryable protocol error

#### Scenario: Strict service error normalizes

- **WHEN** strict unary service returns a registered safe category
- **THEN** Grafana SHALL preserve status and retryability and normalize it without provider-private data

#### Scenario: Strict stream category remains recoverable

- **WHEN** a strict stream error carries safe category data
- **THEN** `NormalizeAPICallError` SHALL recover the category without changing the stream contract

#### Scenario: Strict server and client sizes agree

- **WHEN** a canonical event is at or above the configured limit
- **THEN** server and strict Grafana client SHALL make the same framed-byte decision
