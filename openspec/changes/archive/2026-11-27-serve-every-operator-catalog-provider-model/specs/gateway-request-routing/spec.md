## MODIFIED Requirements

### Requirement: Authorized destinations only
Routing SHALL use only destinations the caller can already select directly. Configured callers SHALL name listed public route IDs, aliases or eligible qualified selectors from their catalog. Every candidate SHALL use its startup-resolved account and endpoint. Controls SHALL NOT otherwise supply credentials, endpoints, provider instances, backend model IDs or another access mode's accounts.

#### Scenario: Backend model ID named as a fallback
- **WHEN** a configured caller's `models` entry names a backend model ID or provider instance that is not a listed ID, alias or eligible qualified selector
- **THEN** the call SHALL fail with the fixed fallback-model-not-found document and zero provider requests

#### Scenario: BYOK selector from a configured caller
- **WHEN** a configured caller's `models` entry is a `provider/model` string that is not a listed ID, alias or eligible qualified selector
- **THEN** it SHALL be treated as an unknown fallback model, and no request account SHALL be constructed

#### Scenario: Configured route from a BYOK caller
- **WHEN** a BYOK caller's `models` entry names a configured public route ID or alias
- **THEN** it SHALL be decoded only as a BYOK selector, and no configured account SHALL become eligible

### Requirement: Unchanged execution without routing controls
Without `models`, `order` or `only`, listed configured calls SHALL use their startup-composed route model, unlisted configured calls SHALL construct just their one eligible candidate, and BYOK calls SHALL use their selected provider's accounts.

#### Scenario: Plain configured call
- **WHEN** a configured caller invokes a listed route without gateway controls
- **THEN** the call SHALL invoke the prebuilt route with its configured order and no per-request candidate or fallback construction
- **AND** only the logical observer chain SHALL be applied for the call, after selection

## ADDED Requirements

### Requirement: Qualified unlisted operator selectors

On both selection paths, after listed ID and alias lookup misses, an unlisted selector SHALL be `<provider-instance>/<model-id>` split at the first slash. Both the full selector and literal suffix SHALL meet the shared public-ID rule. The named instance SHALL have `serveAllModels` true and SHALL NOT block the suffix. The full selector SHALL be canonical and the suffix SHALL be the backend ID, served only by that startup-resolved account at its authorized destination, without consulting inventory.

#### Scenario: Qualified selector uses the named account
- **WHEN** a caller requests `team-openai/org/new-model` and the named provider opts in
- **THEN** it SHALL select `org/new-model` with `team-openai`'s startup-resolved account and destination only

#### Scenario: Listed ID or alias matches a qualified selector
- **WHEN** a listed ID or alias matches the request
- **THEN** the listed route SHALL win on both paths

#### Scenario: Blocked, disabled or malformed unlisted selector
- **WHEN** there is no listed match and the request names a blocked ID, an unknown or disabled instance, or a missing or invalid selector or suffix
- **THEN** the call SHALL fail as model not found without constructing a candidate or contacting a provider
- **AND** an ineligible unlisted `gateway.models` entry SHALL fail with the existing unknown fallback-model error before any provider request

#### Scenario: Prefix or case differs from a blocked ID
- **WHEN** a valid selector's backend ID has a blocked ID as a proper prefix or differs only in case
- **THEN** the blocklist SHALL NOT prevent selecting that ID

### Requirement: No implicit fallback or policy grant for unlisted operator models

An unlisted selector SHALL resolve to one candidate and SHALL NOT inherit listed fallbacks or native option grants. An unlisted Anthropic candidate SHALL have no opt-in to container skills or server-side fallback. A caller MAY explicitly add fallback models using `gateway.models` under the usual routing rules. The blocklist SHALL apply to language-model routing, not to provider-hosted tool options that name their own model.

#### Scenario: Listed model targets the same backend
- **WHEN** an unlisted selector names the same backend ID and provider instance as a differently named listed model with fallbacks or grants
- **THEN** selecting the unlisted ID alone SHALL use exactly one candidate without the listed fallbacks or grants

#### Scenario: Explicit unlisted fallback
- **WHEN** a `gateway.models` entry is an eligible unlisted selector
- **THEN** routing SHALL include its single candidate subject to existing filtering, ordering and deduplication
