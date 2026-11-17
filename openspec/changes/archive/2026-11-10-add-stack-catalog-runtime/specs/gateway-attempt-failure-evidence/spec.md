## ADDED Requirements

### Requirement: Request-scoped catalog key summary protection
A stack-catalog call SHALL protect every decrypted key of its catalog, including keys of providers the call does not use, in the native failure summaries it publishes, together with the startup keys and under the same replacement and omission rules. The protection SHALL apply only to that call. It SHALL NOT be retained afterwards or change summary protection of other calls. BYOK summary protection SHALL stay unchanged.

#### Scenario: Provider error echoes the catalog key
- **WHEN** a stack-catalog candidate fails with a message, type or string code containing the catalog's decrypted key
- **THEN** every occurrence SHALL be replaced in the execution overview and in the event-local native error, unary and streaming, including after a successful fallback

#### Scenario: Unused provider key
- **WHEN** a provider error echoes the key of a catalog provider that the call's chain does not use
- **THEN** that key SHALL also be replaced

#### Scenario: Startup keys still protected
- **WHEN** a stack-catalog provider error echoes an operator provider key or the exporter credential
- **THEN** that value SHALL be replaced as for any other call

#### Scenario: Later calls are unaffected
- **WHEN** a BYOK or operator-catalog call follows a stack-catalog call
- **THEN** its summaries SHALL be protected exactly as without the earlier call
