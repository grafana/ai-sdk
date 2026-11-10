# Spec Delta

## MODIFIED Requirements

### Requirement: One startup-composed logical middleware chain

The Gateway SHALL construct its logical observer chains once at startup: approved context enrichment, Agent Observability recording, structured logging and Prometheus model metrics, in that request order. It SHALL wrap each configured canonical catalog entry's model exactly once at startup with canonical public identity only. After selection, the execution runtime SHALL apply the chain for the selection's source once per call, outside that identity and the inner model. In WP8 the inner model SHALL be the direct provider; WP9 MAY replace it with fallback beneath the unchanged logical wrapper and SHALL NOT wrap physical candidates with WP8 observers.

#### Scenario: Alias uses one canonical chain
- **WHEN** an authenticated text request resolves an alias for a configured canonical model
- **THEN** the catalog SHALL return the same startup-composed model used by the canonical ID
- **AND** logging, metrics, and Agent Observability SHALL each observe one logical call

#### Scenario: ProviderWire behavior remains unchanged
- **WHEN** equivalent unary or streaming text calls execute with and without the logical observers in a deterministic test
- **THEN** both calls SHALL invoke the direct model once with semantically identical call options
- **AND** SHALL produce the same ProviderWire status, response events, event order, and clean-EOF behavior

#### Scenario: Observers are not constructed per call
- **WHEN** many configured, routed and BYOK calls run
- **THEN** observer middleware and metric collectors SHALL be the ones constructed at startup, and no call SHALL register collectors

### Requirement: Shared logical chain traversal and adapter isolation

Response observation SHALL occur in reverse order. Alias and canonical resolution SHALL return the same identity-wrapped model instance, and one invocation SHALL traverse each logical observer exactly once. The chain SHALL be shared service behavior below public API adapters, applied by the execution runtime. It SHALL NOT be constructed in ProviderWire request handling and SHALL NOT change ProviderWire validation, public bytes, error mapping, commitment, stream ordering, timeout, or cancellation precedence.

#### Scenario: Shared logical chain traversal and adapter isolation
- **WHEN** alias and canonical calls traverse the startup chain
- **THEN** each SHALL use the same identity-wrapped instance and reverse response-observation order without constructing observers in ProviderWire or changing public protocol behavior
