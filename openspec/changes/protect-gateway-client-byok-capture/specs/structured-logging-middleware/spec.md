## MODIFIED Requirements

### Requirement: Default redaction

`DefaultRedactor()` SHALL redact known secret-bearing keys even when capture options make their parent object eligible for logging.

The default redactor SHALL perform case-insensitive key matching for at least these patterns: `authorization`, `x-api-key`, `api-key`, `apikey`, `token`, `access_token`, `refresh_token`, `id_token`, `password`, `secret`, `credential`, `cookie`, and `set-cookie`.

The default redactor SHALL recurse through maps, slices, slog groups, and JSON-compatible values where feasible. If a captured attr is already an opaque string, the default redactor SHALL NOT rely on brittle substring rewriting; it SHALL redact only fields represented as structured attrs or typed values.

A custom `Redactor` SHALL receive the request context, event kind, and selected attrs immediately before logging. `DefaultRedactorWithExtraKeys` SHALL preserve default behavior while adding caller-supplied secret key patterns.

#### Scenario: Secret header is redacted after capture

- **WHEN** `Capture.Headers` is enabled and request headers include `Authorization: Bearer secret`
- **THEN** the emitted header attribute SHALL replace the authorization value with a redaction marker
- **AND** the unredacted token SHALL NOT appear in any emitted record

#### Scenario: Custom redactor can remove attrs

- **WHEN** `Options.Redactor` removes an attr from the provided attr slice
- **THEN** the removed attr SHALL NOT be emitted in the log record

Default capture SHALL structurally redact the complete providerOptions.gateway.byok subtree and gateway.byok in provider-options-only attributes, including JSON RawMessage/byte representations, ordered arrays and request bodies attached to errors. Protection SHALL occur before serialization, truncation or fallback formatting can turn the structure into an opaque string. If bounded structural sanitization cannot be completed, the affected attribute SHALL be omitted or replaced with a non-payload marker, never raw or partially sanitized JSON. The original model parameters, results and request metadata SHALL not be mutated.

#### Scenario: Captured BYOK includes unfamiliar fields
- **WHEN** provider-options or request-body capture encounters a gateway.byok subtree containing dummy credentials under known or unfamiliar nested keys
- **THEN** default capture SHALL redact the complete subtree while retaining ordinary sibling options

#### Scenario: Actual unary and streaming request capture
- **WHEN** logger middleware wraps a Grafana provider with request-body/provider-options capture enabled
- **THEN** captured unary, streaming and error-associated request records SHALL contain no BYOK dummy markers even at truncation boundaries
- **AND** the provider SHALL still receive the original request credentials

#### Scenario: Sanitization cannot safely serialize
- **WHEN** a BYOK-bearing capture representation cannot be structurally sanitized within bounds
- **THEN** capture SHALL omit or safely summarize it without logging raw fallback bytes or failing the model call

#### Scenario: Application content resembles credentials
- **WHEN** an allowed prompt/output or ordinary metadata string resembles an API key or contains the text gateway.byok
- **THEN** subtree protection SHALL NOT rewrite that application string
