## MODIFIED Requirements

### Requirement: Best-effort delivery and event-local current errors

Unary result/stream finish SHALL optionally carry providerMetadata.gateway.execution.

#### Scenario: Best-effort delivery and event-local current errors policy
- **WHEN** a configured invocation is observed and its result or failure is delivered
- **THEN** Unary/setup failures SHALL optionally add top-level providerMetadata; committed errors SHALL optionally add error.data.providerMetadata and a current nativeError summary. Existing mapped message/type/code/status/retry fields SHALL preserve classification and retry behavior. Post-selection native failures SHALL remain ordered event-local data and SHALL NOT be retained in finish history. No raw diagnostic tree, separate endpoint field, new event, accessor or public error type SHALL be added. Enrichment SHALL be encoded once with the primary output and SHALL NOT be omitted for size; a failure document SHALL fall back to its fixed base document only when that encoding fails.

#### Scenario: Leading and later stream errors
- **WHEN** a selected provider emits multiple errors followed by valid content and finish
- **THEN** both clients SHALL retain each current summary in its own event, preserve error/content/finish order and expose the selected destination without finish error history or fallback replay

#### Scenario: Success overview does not fit
- **WHEN** a valid primary unary result or finish would fit the former unary or frame limit only without its overview
- **THEN** the response SHALL remain successful with the overview attached and a native `gateway` namespace relocated under `nativeMetadata`

#### Scenario: Provider-originated credential echo
- **WHEN** a native message, type or string code echoes a configured server API key
- **THEN** the Gateway SHALL replace exact matches before publishing the summary in execution metadata or event-local nativeError, independently of caller type and operator logger field policy
- **AND** caller-owned BYOK echoes SHALL remain unchanged unless they also match configured keys; status and numeric code SHALL remain unchanged

#### Scenario: Failure enrichment does not fit
- **WHEN** a classified failure would fit the former 64 KiB error limit only without its overview and current summary
- **THEN** the complete failure SHALL be emitted with them and with unchanged classification

#### Scenario: Failure enrichment cannot be encoded
- **WHEN** encoding a classified failure with its optional overview or current summary fails
- **THEN** the fixed base failure SHALL be emitted without a new diagnostic-induced error

#### Scenario: Primary output is invalid
- **WHEN** original metadata, content or usage is invalid
- **THEN** the existing adaptation failure SHALL remain effective without relocation repairing it or another candidate replaying generation
