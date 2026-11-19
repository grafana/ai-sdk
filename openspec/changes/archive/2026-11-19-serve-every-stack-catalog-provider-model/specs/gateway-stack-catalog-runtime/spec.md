# Spec Delta

## MODIFIED Requirements

### Requirement: Stack-catalog routing and fallback
Stack-catalog selections SHALL resolve canonical IDs and aliases within the catalog first, then try an eligible unlisted selector. They SHALL apply `models`, `order` and `only` and ordered fallback with the same rules, errors, deduplication and attempt bound as operator-catalog selections, and provider slugs SHALL be the provider types.

#### Scenario: Alias call
- **WHEN** a call requests an alias of a catalog model, even if it has the form of an unlisted selector
- **THEN** the selection SHALL use that model's canonical ID and candidates

#### Scenario: Listed ID looks like an unlisted selector
- **WHEN** a call requests a listed ID that also has the form of an unlisted selector
- **THEN** the selection SHALL use the listed model and its configured candidates

#### Scenario: Unknown model
- **WHEN** a call requests an ID that is neither a listed ID, an alias, nor an eligible unlisted selector in the catalog
- **THEN** the call SHALL fail with `404 model_not_found`, with zero provider requests

#### Scenario: No configured models
- **WHEN** a valid catalog call has no listed models and does not name an eligible unlisted selector
- **THEN** the call SHALL fail with the existing model not found category (`404 model_not_found`), without constructing a candidate, using another catalog, or contacting a provider

#### Scenario: Unknown models entry
- **WHEN** a `gateway.models` entry names no listed model, alias or eligible unlisted selector in the catalog
- **THEN** the call SHALL fail with model not found and the fallback-model reason, with zero provider requests

#### Scenario: Primary fails and the fallback succeeds
- **WHEN** the primary candidate fails with a fallback-eligible error and the fallback succeeds
- **THEN** the call SHALL succeed with two provider requests and an overview listing both attempts

#### Scenario: Composed chain above the bound
- **WHEN** routing controls compose more than 8 candidates
- **THEN** the call SHALL fail with the attempt-bound document, with zero constructed models and zero provider requests

## ADDED Requirements

### Requirement: Qualified unlisted stack-catalog selectors
After listed lookup misses, an eligible unlisted selector SHALL be `<provider-instance>/<model-id>`, split at its first slash. The instance SHALL be named in the catalog with `serveAllModels` true, and the suffix SHALL meet blocklist ID validation without matching an entry. Selection SHALL use the full selector as its canonical ID and the literal suffix as the backend ID, with the named instance's saved key and destination. It SHALL NOT consult provider inventory.

#### Scenario: Two instances of the same provider type are enabled
- **WHEN** a call requests `team-openai/org/new-model` and the catalog has enabled `team-openai` and `other-openai` instances but has no listed models
- **THEN** it SHALL select `org/new-model` with `team-openai`'s key only, with `team-openai/org/new-model` as the canonical ID
- **AND** requesting `org/new-model` without the instance qualifier SHALL fail as unknown if `org` is not an enabled instance

#### Scenario: Blocked, disabled or malformed unlisted selector
- **WHEN** there is no listed match and the request names a blocked ID, an instance that is unknown or off, or a missing or invalid suffix
- **THEN** the call SHALL fail with `404 model_not_found`, without building a candidate or contacting a provider

#### Scenario: Prefix or case differs from a blocked ID
- **WHEN** a valid unlisted selector's ID has a blocked ID as a proper prefix or differs only in case
- **THEN** the blocklist SHALL NOT prevent selecting that ID

### Requirement: Blocklist applies to language-model routing
The provider blocklist SHALL apply to language-model routing. Provider-hosted tool options that name their own model SHALL NOT be covered.

#### Scenario: Hosted tool names a separate model
- **WHEN** an otherwise allowed language-model call sets a model ID in a provider-hosted tool option
- **THEN** that option SHALL follow the existing tool behavior, without a `modelBlocklist` check

### Requirement: No implicit fallback or policy grant for unlisted models
An unlisted selector SHALL resolve to one candidate and SHALL NOT inherit listed fallbacks or native option grants. An unlisted Anthropic candidate SHALL have no opt-in to container skills or server-side fallback. A caller MAY explicitly add fallback models using `gateway.models` under the usual catalog routing rules.

#### Scenario: Listed model targets the same backend
- **WHEN** an unlisted selector names the same backend ID as a differently named listed model with a fallback
- **THEN** selecting the unlisted ID alone SHALL compose exactly one candidate, without the listed fallback

#### Scenario: Explicit unlisted fallback
- **WHEN** a `gateway.models` entry is an eligible unlisted selector
- **THEN** routing SHALL include its single candidate subject to existing filtering, ordering and deduplication

#### Scenario: Blocked explicit fallback
- **WHEN** a `gateway.models` entry resolves to no listed model or alias and its qualified backend ID is blocked
- **THEN** the call SHALL fail with the existing unknown fallback-model error before constructing or invoking any candidate

#### Scenario: Anthropic opts in on an unlisted model
- **WHEN** a caller requests a container skill or active Anthropic server-side fallback on an unlisted candidate
- **THEN** the candidate SHALL refuse the option under the existing deny-by-default policy
