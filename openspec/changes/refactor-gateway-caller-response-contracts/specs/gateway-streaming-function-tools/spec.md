## MODIFIED Requirements

### Requirement: Basic streamed calls and results
Private DTOs SHALL encode function calls and correlated basic results with non-null JSON result and optional isError. Duplicate calls/results and unmatched results SHALL fail safely. Client-executed calls SHALL not require a result before finish. Provider-executed, dynamic and preliminary enabled behavior SHALL remain deferred to WP13; supported tool-event providerMetadata SHALL be preserved through #280 under ordinary reviewed bounds without enabling deferred execution behavior.

#### Scenario: Standalone call awaits client execution
- **WHEN** a complete function call with no incremental input events is followed by valid finish
- **THEN** the client SHALL receive the call and finish without requiring a server result

#### Scenario: Basic result transport
- **WHEN** a recording model emits a call followed by one matching basic JSON result
- **THEN** both clients SHALL receive equivalent call/result values in order

#### Scenario: Deferred execution marker
- **WHEN** providerExecuted, dynamic or preliminary is true in provider output
- **THEN** unsupported output SHALL produce a safe terminal failure without emitting the unsupported value or native transport
