# Spec Delta

## MODIFIED Requirements

### Requirement: Unconditional metadata placement
Enrichment SHALL place the overview under `gateway.execution` and its routing summary under `gateway.routing`, and always relocate a native `gateway` value, complete, under `gateway.nativeMetadata`, without merging it into trusted facts. Other namespaces SHALL remain opaque. Enrichment SHALL have no size limit and SHALL be absent only when no overview exists or it cannot be encoded. Namespace presence SHALL NOT establish provenance, routing authority or absence of attempts.

#### Scenario: Native namespace collides
- **WHEN** native metadata already has a `gateway` value and an overview is attached
- **THEN** the complete native value SHALL appear under `nativeMetadata` while execution and routing facts remain separate and inputs remain unchanged

#### Scenario: Large relocated namespace
- **WHEN** the native `gateway` value or the overview is larger than the former unary or frame budget
- **THEN** relocation SHALL still happen and the complete enriched output SHALL be written

#### Scenario: Enrichment cannot be encoded
- **WHEN** optional overview assembly contains malformed data
- **THEN** enrichment SHALL return no metadata, without truncation or a new error, and existing content, error or finish behavior SHALL remain unchanged

#### Scenario: Routing travels with the overview
- **WHEN** an overview is published on a unary result, a unary or setup error document, a stream finish or a committed stream error
- **THEN** the same carrier SHALL contain `gateway.routing` projected from that overview, and SHALL contain neither when no overview is published

## ADDED Requirements

### Requirement: Routing summary projection
The routing summary SHALL be a pure projection of the published overview, with each attempt grouped under the model its chain position was composed from. It SHALL use a subset of Vercel's documented `gateway.routing` field names: `originalModelId`, `canonicalSlug`, `resolvedProvider`, `finalProvider`, `modelAttemptCount`, `totalProviderAttemptCount` and `modelAttempts`. It SHALL add no fact that the overview lacks.

#### Scenario: Configured fallback model serves the call
- **WHEN** a call requested alias `a`, its route `A` had one failed candidate, and `models` entry `b` resolved to route `B`, whose anthropic candidate was selected
- **THEN** `originalModelId` SHALL be `a`, `canonicalSlug` SHALL be `A`, `resolvedProvider` SHALL be the first attempt's provider, and `finalProvider` SHALL be `anthropic`
- **AND** `modelAttempts` SHALL list `A` with one unsuccessful provider attempt, then `B` with one successful provider attempt, and `totalProviderAttemptCount` SHALL be 2

#### Scenario: Unrouted call
- **WHEN** a configured call without routing controls publishes an overview
- **THEN** `modelAttempts` SHALL contain exactly one entry for the requested route

#### Scenario: All attempts fail
- **WHEN** every invoked attempt failed or the last was canceled
- **THEN** `finalProvider` SHALL be absent and every provider attempt SHALL have `success` false

### Requirement: Routing model attempt fields
Each `modelAttempts` entry SHALL contain `modelId`, `success`, `providerAttemptCount` and `providerAttempts`, plus `canonicalSlug` when the model came from catalog resolution. It SHALL appear only for models with at least one observed attempt. `modelId` SHALL be the requested ID or `models` entry as sent, and `canonicalSlug` its resolved public ID.

#### Scenario: BYOK model attempts
- **WHEN** a BYOK call's `anthropic/x` account fails and its `openai/y` fallback account is selected
- **THEN** `modelAttempts` SHALL list `anthropic/x` then `openai/y`, each without `canonicalSlug`, and the top-level `canonicalSlug` SHALL be absent

#### Scenario: Model never reached
- **WHEN** a `models` entry was composed into the chain but the call committed before reaching it
- **THEN** no `modelAttempts` entry SHALL describe it

### Requirement: Routing provider attempt fields
Each `providerAttempts` entry SHALL contain `provider` and `success`, plus `statusCode` when the attempt's failure summary has one. `success` SHALL mean the attempt was selected, not that a stream completed. The summary SHALL NOT contain timestamps, request or response IDs, costs, credential types, account indexes, unrun candidates or prose planning text.

#### Scenario: Selected stream later errors
- **WHEN** a stream's selected attempt later emits an error part
- **THEN** its provider attempt SHALL still have `success` true, and the error SHALL remain in the stream's own error part

#### Scenario: Failed attempt with native status
- **WHEN** an attempt failed with a native 429 summary
- **THEN** its provider attempt SHALL have `success` false and `statusCode` 429, and the full summary SHALL remain only in `gateway.execution`

### Requirement: Routing summary validity
The routing summary SHALL be published exactly when the overview is, on the same carrier. It SHALL never invent or reorder attempts, and SHALL describe the same attempts as `gateway.execution`.

#### Scenario: Overview omitted
- **WHEN** the observed attempts outnumber the attempts the call's chain expected
- **THEN** neither `gateway.execution` nor `gateway.routing` SHALL be published
