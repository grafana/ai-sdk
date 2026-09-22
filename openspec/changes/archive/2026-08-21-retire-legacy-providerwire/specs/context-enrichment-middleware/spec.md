## ADDED Requirements

### Requirement: Composition with registry and Agent Observability

The enrichment module SHALL require no registry changes. It SHALL be usable with `registry.WithLanguageModelMiddleware` because registry already accepts `middleware.Middleware` values.

The enrichment module SHALL document middleware ordering with Agent Observability. When enrichment appears before `agentobservability.Stack(...)` in the middleware slice, Agent Observability hooks and recording SHALL observe enriched `CallOptions`; when enrichment appears after Agent Observability, enrichment SHALL be transport-only from Agent Observability's perspective.

#### Scenario: Registry applies enrichment to resolved model

- **WHEN** a provider registry is configured with `registry.WithLanguageModelMiddleware(enrichment.Middleware(opts))`
- **THEN** every model resolved by that registry SHALL receive enrichment behavior

#### Scenario: Enrichment before Agent Observability is visible to Agent Observability

- **WHEN** a model is wrapped with enrichment middleware before `agentobservability.Stack(...)`
- **THEN** subsequent Agent Observability middleware SHALL observe the enriched call options

#### Scenario: Enrichment after Agent Observability is transport-only for Agent Observability

- **WHEN** a model is wrapped with Agent Observability middleware before enrichment middleware
- **THEN** Agent Observability middleware SHALL observe the original call options and the inner provider SHALL observe enriched call options

## MODIFIED Requirements

### Requirement: Nested enrichment middleware module

The repository SHALL provide `middleware/enrichment/` as a separate Go module with module path `github.com/grafana/ai-sdk/middleware/enrichment` and `replace github.com/grafana/ai-sdk => ../../` for local development.

The production package SHALL depend on the ai-sdk root module and the Go standard library only. It SHALL NOT import any provider module. The root ai-sdk module SHALL NOT import `middleware/enrichment`.

#### Scenario: Root consumers do not import enrichment

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk`
- **THEN** `github.com/grafana/ai-sdk/middleware/enrichment` SHALL NOT be pulled into the consumer's transitive dependency graph

#### Scenario: Enrichment module remains provider-agnostic

- **WHEN** the enrichment module's production imports are inspected
- **THEN** no provider module import SHALL be present

### Requirement: Header output merge semantics

The package SHALL provide header enrichment through `Options.Headers`. `HeaderOptions` SHALL support explicit `Map map[string]string` mode from enrichment key to HTTP header name, optional `Prefix string` mode, `Conflict ConflictPolicy`, and additional protected header names. Header output SHALL be disabled when both `Map` and `Prefix` are empty.

Header merge SHALL start from existing caller headers. Header conflict detection SHALL be case-insensitive using canonical HTTP header names. The default conflict policy SHALL be `ConflictCallerWins`.

For conflicts:

- `ConflictCallerWins` SHALL preserve the existing caller header.
- `ConflictEnrichmentWins` SHALL overwrite the existing caller header unless the target header is protected.
- `ConflictError` SHALL return an error.

Protected auth and transport headers SHALL NOT be written or overwritten by enrichment by default, regardless of conflict policy. If enrichment targets a protected header and no caller value exists, the header output SHALL still omit that enrichment header. The protected set SHALL include common auth and provider transport headers such as `Authorization`, `Proxy-Authorization`, `X-Access-Token`, `X-Grafana-Id`, `Content-Type`, and provider API-key or protocol headers. Callers SHALL be able to add deployment-specific protected header names, but the initial API SHALL NOT provide an opt-in to write built-in protected header names.

#### Scenario: Caller wins by default

- **WHEN** existing call options contain `X-Request-Id: caller` and enrichment would write `X-Request-Id: enriched`
- **THEN** the resulting headers SHALL keep the caller value by default

#### Scenario: Case-insensitive conflicts are detected

- **WHEN** existing call options contain `x-request-id` and enrichment targets `X-Request-Id`
- **THEN** the header output SHALL treat the two names as a conflict

#### Scenario: Enrichment wins when configured

- **WHEN** a header conflict occurs and `Options.Headers` is configured with `ConflictEnrichmentWins`
- **THEN** the resulting headers SHALL use the enrichment value unless the header name is protected

#### Scenario: ConflictError fails the header output

- **WHEN** a header conflict occurs and `Options.Headers` is configured with `ConflictError`
- **THEN** the header output SHALL return an error

#### Scenario: Protected headers are not written when absent

- **WHEN** enrichment targets an absent protected header such as `Authorization` or `X-Access-Token`
- **THEN** the resulting headers SHALL NOT contain that header from enrichment

#### Scenario: Protected headers are not overwritten

- **WHEN** enrichment targets a protected header such as `Authorization` or `X-Access-Token`
- **THEN** the header output SHALL NOT overwrite the existing protected header even if `ConflictEnrichmentWins` is configured

### Requirement: Provider options output merge semantics

The package SHALL provide provider-options enrichment through `Options.ProviderOptions`. `ProviderOptionsConfig` SHALL include `ProviderKey string`, `ObjectKey string`, `Map map[string]string`, and `Conflict ConflictPolicy`. Provider-options output SHALL be disabled when `ProviderKey` is empty.

The output SHALL write string enrichment values into `provider.CallOptions.ProviderOptions` under `ProviderKey`. Values selected by `ProviderOptionsConfig.Map` SHALL write to the corresponding JSON field names. Globally included values not present in the map SHALL write under their original keys. Values selected solely by the header mapping SHALL NOT be emitted into provider options. If `ObjectKey` is non-empty, the output SHALL write values into a nested object named by `ObjectKey`; otherwise it SHALL write values into the top-level provider option object.

The output SHALL preserve unrelated existing fields. When the provider key exists, it SHALL marshal the existing typed or raw provider option to JSON, require object-shaped JSON for merging, and store the merged result as `provider.RawProviderOption{Key: ProviderKey, Raw: mergedJSON}`. Field conflicts SHALL obey `ConflictPolicy`, with `ConflictCallerWins` as the default.

#### Scenario: Absent provider key creates raw option

- **WHEN** provider options are nil or do not contain the configured provider key
- **THEN** the provider-options output SHALL allocate provider options as needed
- **AND** it SHALL store a `provider.RawProviderOption` containing the enrichment JSON under the configured provider key

#### Scenario: Existing raw object is merged

- **WHEN** provider options contain a `provider.RawProviderOption` with object JSON for the configured provider key
- **THEN** enrichment fields SHALL be shallow-merged into that JSON object or nested object
- **AND** unrelated existing fields SHALL be preserved

#### Scenario: Existing typed option is merged through JSON

- **WHEN** provider options contain a typed provider option for the configured provider key
- **THEN** the provider-options output SHALL marshal the typed option to JSON, merge enrichment into the object JSON, and store the result as `provider.RawProviderOption`

#### Scenario: Existing non-object obeys conflict policy

- **WHEN** provider options contain non-object JSON for the configured provider key
- **THEN** `ConflictCallerWins` SHALL preserve the existing option without enrichment
- **AND** `ConflictEnrichmentWins` SHALL replace it with the enrichment object
- **AND** `ConflictError` SHALL return an error

#### Scenario: ResolveOption remains usable after merge

- **WHEN** a provider option is merged and stored as `provider.RawProviderOption`
- **THEN** downstream code SHALL be able to recover typed views using `provider.ResolveOption` for compatible option structs

### Requirement: Validation and documentation for safe use

The implementation SHALL include unit tests for value collection, context defensive copies, default-deny filtering, per-output selection isolation, sensitive redaction/drop behavior, cardinality filtering, over-limit value dropping, generate and stream transformation, header conflict policies, protected headers including absent protected targets, provider-options creation and merge behavior, unrelated-field preservation, registry composition, and middleware ordering examples.

The package godoc SHALL document that enrichment is opt-in, default-deny, string-only, and provider-agnostic. It SHALL warn against propagating secrets, API tokens, auth claims without explicit filtering, prompts, tool arguments, raw user input, and high-cardinality metric labels. It SHALL state that the module does not emit telemetry and does not change provider/UI representation behavior unless callers explicitly attach it to a model.

#### Scenario: Unit tests cover generate and stream calls

- **WHEN** the enrichment module test suite runs
- **THEN** it SHALL verify that both `DoGenerate` and `DoStream` receive enriched call options when configured

#### Scenario: Documentation warns about sensitive data

- **WHEN** a consumer reads the package documentation
- **THEN** it SHALL explain the default-deny model and warn not to propagate secrets, tokens, prompts, tool arguments, or raw user input

#### Scenario: No default conformance fixture changes are required

- **WHEN** this module remains opt-in without wrapping any model by default
- **THEN** existing provider and UI/SSE conformance fixtures SHALL remain unchanged

## REMOVED Requirements

### Requirement: Composition with registry, Agent Observability, and Grafana hosted provider
**Reason**: The requirement name and guidance depend on a removed provider module and its hosted controls.
**Migration**: Use the registry and Agent Observability composition requirement; generic provider-option merge behavior remains specified separately.
