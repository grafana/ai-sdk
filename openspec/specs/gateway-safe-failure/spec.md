# gateway-safe-failure Specification

## Purpose

Define protocol-neutral, privacy-safe gateway failures and their boundary-owned reduction contract.

## Requirements

### Requirement: Protocol-neutral safe failure value
The repository SHALL provide a gateway-owned safe failure value containing only a typed category, a non-empty approved public message, and retryability. The value SHALL NOT contain or unwrap a raw cause, provider error, URL, request or response body, headers, credentials, provider identity, backend model ID, or arbitrary metadata. Construction SHALL reject an unknown category or empty message.

#### Scenario: Safe failure is constructed
- **WHEN** a caller supplies a supported category and non-empty approved message
- **THEN** construction SHALL return a value exposing only its category, message, and retryability

#### Scenario: Unsupported category is rejected
- **WHEN** a caller supplies a category outside the defined vocabulary
- **THEN** construction SHALL fail rather than create an unclassifiable value

#### Scenario: Empty message is rejected
- **WHEN** a caller supplies an empty safe message
- **THEN** construction SHALL fail

#### Scenario: Rich error details are not representable
- **WHEN** the public shape and unwrapping behavior of the safe value are inspected
- **THEN** no raw cause, provider error, URL, bodies, headers, credentials, provider identity, backend model ID, or arbitrary metadata SHALL be available

#### Scenario: Zero value reaches a renderer
- **WHEN** a protocol renderer receives an invalid or zero safe-failure value that bypassed construction
- **THEN** it SHALL render its canonical internal failure rather than an empty or unknown category

### Requirement: Initial safe failure category vocabulary
The safe failure category SHALL be a named string type with constants for invalid request, authentication, permission, not found, rate limit, overload, failed dependency, upstream failure, timeout, cancellation, and internal failure. Invalid request, authentication, permission, not found, failed dependency, and cancellation SHALL be non-retryable. Rate limit, overload, upstream failure, timeout, and internal failure SHALL be retryable.

#### Scenario: Non-retryable category is constructed
- **WHEN** a safe failure is constructed for invalid request, authentication, permission, not found, failed dependency, or cancellation
- **THEN** its retryability SHALL be false

#### Scenario: Retryable category is constructed
- **WHEN** a safe failure is constructed for rate limit, overload, upstream failure, timeout, or internal failure
- **THEN** its retryability SHALL be true

### Requirement: Failure reduction remains boundary-owned
Shared authentication, catalog, policy, provider, and lifecycle boundaries SHALL reduce rich failures to the safe value before a protocol renders them. The safe-failure package SHALL NOT import an HTTP protocol, serialize a public error envelope, inspect provider response payloads, or become a general error-classification framework.

#### Scenario: Protocol renders a safe failure
- **WHEN** a protocol adapter receives a safe failure
- **THEN** that adapter SHALL choose its own status and wire envelope without adding protocol behavior to the safe-failure package

#### Scenario: Rich provider failure is reduced
- **WHEN** a provider or resolver returns a rich error containing private fields
- **THEN** the owning boundary SHALL select a category and approved message without copying those private fields into the safe value
