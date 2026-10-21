# Spec Delta

## MODIFIED Requirements

### Requirement: Best-effort delivery and event-local current errors

Unary result/stream finish SHALL optionally carry providerMetadata.gateway.execution.

#### Scenario: Best-effort delivery and event-local current errors policy
- **WHEN** a configured invocation is observed and its result or failure is delivered
- **THEN** Unary/setup failures SHALL optionally add top-level providerMetadata; committed errors SHALL optionally add error.data.providerMetadata and a current nativeError summary. Existing mapped message/type/code/status/retry fields SHALL preserve classification and retry behavior. Post-selection native failures SHALL remain ordered event-local data and SHALL NOT be retained in finish history. No raw diagnostic tree, separate endpoint field, new event, accessor or public error type SHALL be added. Optional evidence SHALL be encoded with the primary result, error or event without fit-based omission; only optional data that cannot be encoded SHALL be dropped.

#### Scenario: Leading and later stream errors
- **WHEN** a selected provider emits multiple errors followed by valid content and finish
- **THEN** both clients SHALL retain each current summary in its own event, preserve error/content/finish order and expose the selected destination without finish error history or fallback replay

#### Scenario: Success overview does not fit
- **WHEN** a valid primary unary result or finish with an overview exceeds the former unary or frame budget
- **THEN** the response SHALL remain successful with native metadata unchanged and SHALL include the overview, relocating an opaque native gateway namespace

#### Scenario: Provider-originated credential echo
- **WHEN** a native message, type or string code echoes a configured server API key
- **THEN** the Gateway SHALL replace exact matches before publishing the summary in execution metadata or event-local nativeError, independently of caller type and operator logger field policy
- **AND** caller-owned BYOK echoes SHALL remain unchanged unless they also match configured keys; status and numeric code SHALL remain unchanged

#### Scenario: Failure enrichment does not fit
- **WHEN** a classified failure carries an overview and current summary larger than the former 64 KiB error budget
- **THEN** the classified failure SHALL include them without a diagnostic-induced error or omission

#### Scenario: Primary output is invalid
- **WHEN** original metadata, content or usage is invalid
- **THEN** the existing adaptation failure SHALL remain effective without relocation repairing it or another candidate replaying generation

### Requirement: Configured API-key summary protection

The Gateway SHALL protect exact configured API-key values in its produced native failure summaries without filtering arbitrary response content.

#### Scenario: Immutable configured-key policy
- **WHEN** the service constructs its language-model handler
- **THEN** it SHALL supply all resolved configured provider API keys, including unused accounts, and the enabled exporter credential to an immutable summary policy
- **AND** changes to caller-owned configuration slices SHALL NOT change that policy; keys SHALL NOT enter discovery or request-selected catalog identity
- **AND** static Gateway access keys SHALL remain outside this policy, preserving their authentication-owned hash-only storage without retaining raw copies for summary matching

#### Scenario: Configured and BYOK callers receive summaries
- **WHEN** message, type or string code contains an exact configured-key match
- **THEN** every occurrence SHALL be replaced before publishing execution metadata or event-local nativeError, including after successful fallback
- **AND** configured ownership SHALL take precedence over a matching BYOK value; unrelated BYOK echoes, numeric code, status, classification and retryability SHALL remain unchanged
- **AND** JSON escapes SHALL be handled through ordinary decoding; arbitrary transformed or encoded-secret detection SHALL NOT be implied

#### Scenario: Protection cannot preserve a field or fit the envelope
- **WHEN** a replacement still contains a configured key, or protected optional output exceeds the former envelope budget
- **THEN** the affected field SHALL be omitted rather than publishing its unredacted form, without replaying provider work; envelope size alone SHALL NOT cause omission

#### Scenario: Native content remains outside summary protection
- **WHEN** native metadata, generated text or tool output contains the same value
- **THEN** this summary policy SHALL NOT rewrite those values or buffer stream fragments; source errors and independent SDK observers SHALL remain unchanged

### Requirement: Real runtime and consumer evidence

Tests SHALL exercise actual configured invocation counts through production handlers and authenticated command routes, both registered TypeScript and independent Go client access, consumer middleware and primary validation. Frontend wire changes SHALL include schema-parsed cross-language assembly. Synthetic provider inputs SHALL remain focused tests, not recorded conformance evidence. Historical reviews SHALL NOT be claimed as approval of the rewrite.

#### Scenario: Both clients inspect secondary success and exhausted chain
- **WHEN** the authenticated command serves successful secondary, noneligible primary and exhausted heterogeneous routes
- **THEN** native request counts and returned candidate-local summaries SHALL agree across both clients and invocation modes without enabling capture or raw chunks

#### Scenario: Both clients consume large evidence
- **WHEN** the protocol test host serves success and failure evidence larger than the former server envelope budgets
- **THEN** both registered clients SHALL consume it under their default limits
