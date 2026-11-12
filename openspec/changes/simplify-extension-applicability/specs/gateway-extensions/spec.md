# Spec Delta

## MODIFIED Requirements

### Requirement: Extension definitions
An extension definition SHALL declare a name, a scope of `operator` or `tenant`, a failure policy of `fail-closed` or `skip`, and a factory. It MAY declare key ownerships, a transform priority, a JSON Schema for its configuration, secret names and whether it is enabled by default. Names SHALL be 1 to 63 lowercase letters, digits or hyphens, starting and ending with a letter or digit.

#### Scenario: Invalid definition
- **WHEN** a definition has an invalid name, an unknown scope or failure policy, no factory, a configuration schema that does not compile, or an invalid or repeated secret name
- **THEN** startup SHALL fail before any request is served, naming the extension and the invalid field

#### Scenario: Duplicate names
- **WHEN** two definitions in the distribution's options share a name
- **THEN** startup SHALL fail naming the duplicate

## REMOVED Requirements

### Requirement: Key ownership and source filtering
**Reason**: Every source has exactly one key ownership, so a source filter could only narrow the ownership filter, and tied extensions to a source list that grows.
**Migration**: Declare `Keys` only; `Key ownership filtering` replaces this requirement. Read `Call.Source` as an informational label.

## ADDED Requirements

### Requirement: Key ownership filtering
Key ownership SHALL be `operator` for operator-catalog selections and `customer` for every other source, including sources added later. An extension SHALL apply to a call when its declared ownerships, which default to both, include the call's ownership. The call's source SHALL be informational for extensions and SHALL NOT decide where an extension applies. An extension that does not apply SHALL run no phase and get no observe record.

#### Scenario: Operator-owned extension on a BYOK call
- **WHEN** an extension declares ownership `operator` and a request BYOK call runs
- **THEN** none of its phases SHALL run for that call

#### Scenario: Undeclared ownership
- **WHEN** an extension declares no ownership
- **THEN** it SHALL apply to operator-catalog, routed and request BYOK calls

#### Scenario: Customer-owned extension
- **WHEN** an extension declares ownership `customer`
- **THEN** it SHALL apply to request BYOK and stack-catalog calls and SHALL NOT apply to operator-catalog calls
