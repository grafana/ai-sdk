# Spec Delta

## Purpose

Define the Gateway's customer telemetry resolution boundary so destinations, recording policy, and ingestion authority can evolve independently without mixing customer stacks or operator telemetry. This initial contract does not activate delivery.

## ADDED Requirements

### Requirement: Authorized stack attribution
Customer telemetry resolution SHALL be scoped to the stack established by Gateway authorization. Missing or refused attribution SHALL yield no customer recording plan. Resolved state belonging to a different stack SHALL be rejected without substituting an operator or default stack.

#### Scenario: Destination belongs to another customer
- **WHEN** a resolution result for authorized stack A identifies stack B
- **THEN** validation rejects that result without a fallback destination

#### Scenario: No authorized customer stack
- **WHEN** attribution is empty or access is refused
- **THEN** no customer recording plan can be validated

### Requirement: Independent signal destinations
Resolution SHALL represent generation, trace, metric, and log destinations independently, including ingestion identities and credential references. Unavailable signals SHALL be explicit. A destination SHALL NOT imply product eligibility or authorize recording. Invalid destinations SHALL be rejected before transport use.

#### Scenario: Distinct ingestion identities
- **WHEN** a stack has different generation, trace, metric, and log ingestion identities
- **THEN** resolution preserves all four without substituting stack or other signal identifiers

#### Scenario: Partial availability
- **WHEN** a valid metric destination exists and generation delivery is unavailable
- **THEN** metric destination validation succeeds independently while generation delivery remains unavailable

#### Scenario: Unsafe destination
- **WHEN** a destination lacks its required identity or contains a non-HTTPS URL, URL credentials, query, or fragment
- **THEN** validation rejects it without echoing the rejected value in diagnostics

### Requirement: Separate ingestion authority
Destination and policy snapshots SHALL contain no ingestion secrets. Credential references SHALL identify a separate authority source, independently versioned from recording policy. Caller authentication and provider keys SHALL NOT be treated as ingestion credentials. Diagnostics SHALL omit raw credentials and destination values.

#### Scenario: Credential rotation
- **WHEN** an ingestion credential revision changes for the same destination and policy
- **THEN** the credential change can be distinguished without changing the policy revision
- **AND** destination and policy snapshots contain no credential material

### Requirement: Conservative default recording policy
The default policy SHALL permit only metadata-only generations and traces and a fixed, documented model-metric profile for independently eligible signals. Customer log recording SHALL default to disabled. Unknown eligibility SHALL NOT grant access. Preferences SHALL NOT override product ineligibility or mandatory service restrictions.

#### Scenario: Eligible stack with no custom preference
- **WHEN** eligibility is authoritative and the optional customer preference is absent
- **THEN** the policy selects documented defaults for eligible signals
- **AND** prompt, output, reasoning, tool payloads, raw errors, arbitrary attributes, and raw request or response data remain excluded

#### Scenario: Product eligibility differs by signal
- **WHEN** generations are ineligible but metrics are eligible
- **THEN** the policy disables generations without disabling independently eligible model metrics

#### Scenario: No customer logging producer
- **WHEN** a log destination is available under the default policy
- **THEN** customer log recording remains disabled and operator logs are not forwarded

### Requirement: Absence and policy failure are distinct
Policy evaluation SHALL distinguish absent optional preferences, explicit disablement, and lookup failure. A failure SHALL NOT select absent-preference defaults or discard a known disablement. Policy validity SHALL be explicit; expired or invalid policy SHALL NOT authorize new recording.

#### Scenario: Failed read after disablement
- **WHEN** a customer preference was disabled and its refresh fails
- **THEN** the result does not restore default recording

#### Scenario: First read fails
- **WHEN** no valid policy is available and a policy read fails
- **THEN** no customer recording plan is authorized

#### Scenario: Policy expires
- **WHEN** the current time reaches a policy's validity deadline
- **THEN** that policy cannot authorize new recording

### Requirement: Stable recording snapshots
Validated destination and policy snapshots SHALL preserve stack attribution and values when caller-owned inputs change. Policy, destination, and credential changes SHALL be independently distinguishable. Default profile changes SHALL require explicit compatibility review; upgrades SHALL NOT silently broaden content capture or metric dimensions.

#### Scenario: Input mutation
- **WHEN** a caller changes an input collection after snapshot construction
- **THEN** the validated snapshot retains its original values

#### Scenario: Policy-only update
- **WHEN** recording is disabled without changing a destination or credential reference
- **THEN** the policy revision changes independently of those destination and credential references

### Requirement: Contract introduction does not activate delivery
Introducing these contracts SHALL NOT start discovery, create customer exporters, change existing operator recording, or change inference requests, responses, or streams. The package SHALL be testable without Cloud credentials, network access, or a customer configuration resource.

#### Scenario: Existing Gateway startup
- **WHEN** the Gateway starts after the contract package is introduced
- **THEN** startup performs no new customer discovery or delivery
- **AND** existing operator telemetry and inference behavior remain unchanged
