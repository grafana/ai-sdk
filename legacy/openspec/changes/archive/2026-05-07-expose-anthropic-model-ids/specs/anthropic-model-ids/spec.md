## ADDED Requirements

### Requirement: Direct Anthropic model ID enumeration
The `anthropic` package SHALL export `ModelIDs() []string`, returning the package's curated list of model IDs for the direct Anthropic API in the form accepted by `New`.

The returned slice SHALL be sorted lexicographically, deterministic across calls, non-empty, and safe for callers to mutate without changing later results.

#### Scenario: List direct Anthropic model IDs
- **WHEN** `ModelIDs` is called
- **THEN** it SHALL return a non-empty sorted list containing curated direct Anthropic model IDs such as `claude-sonnet-4-6`
- **AND** every returned ID SHALL be in the form accepted by `New`

#### Scenario: Direct Anthropic list is copy-safe
- **WHEN** a caller mutates the slice returned by `ModelIDs`
- **THEN** a later call to `ModelIDs` SHALL return the original curated values unaffected by the mutation

### Requirement: Vertex Anthropic model ID enumeration
The `anthropic` package SHALL export `VertexModelIDs() []string`, returning the package's curated list of Anthropic model IDs for the Vertex AI partner channel in the resolved Vertex form expected by `NewVertex` requests.

The returned slice SHALL be sorted lexicographically, deterministic across calls, non-empty, and safe for callers to mutate without changing later results.

#### Scenario: List Vertex Anthropic model IDs
- **WHEN** `VertexModelIDs` is called
- **THEN** it SHALL return a non-empty sorted list containing curated Vertex Anthropic model IDs such as `claude-sonnet-4-5@20250929`
- **AND** IDs with Vertex date pins SHALL use `@YYYYMMDD` form
- **AND** IDs served by Vertex without a date suffix SHALL be returned as bare model names

#### Scenario: Vertex Anthropic list is copy-safe
- **WHEN** a caller mutates the slice returned by `VertexModelIDs`
- **THEN** a later call to `VertexModelIDs` SHALL return the original curated values unaffected by the mutation

### Requirement: Dual-available Anthropic model ID enumeration
The `anthropic` package SHALL export `DualAvailableModelIDs() []string`, returning curated model IDs that are available on both the direct Anthropic API and Vertex AI partner channel.

The returned IDs SHALL be in the direct `New` form. The returned slice SHALL be sorted lexicographically, deterministic across calls, non-empty, and safe for callers to mutate without changing later results.

#### Scenario: List model IDs available on both surfaces
- **WHEN** `DualAvailableModelIDs` is called
- **THEN** every returned ID SHALL be present in `ModelIDs`
- **AND** `ResolveVertexModelID` for every returned ID SHALL be present in `VertexModelIDs`
- **AND** no returned ID SHALL resolve to a fallback `@latest` Vertex ID

#### Scenario: Dual-available list is copy-safe
- **WHEN** a caller mutates the slice returned by `DualAvailableModelIDs`
- **THEN** a later call to `DualAvailableModelIDs` SHALL return the original curated values unaffected by the mutation

### Requirement: Public Vertex model ID resolution
The `anthropic` package SHALL export `ResolveVertexModelID(modelID string) string`, mapping model IDs accepted by `New` to the canonical model ID form expected by Vertex AI partner-channel requests.

Known curated direct model IDs SHALL resolve to their curated Vertex IDs. Already `@`-pinned IDs SHALL be returned unchanged. Unknown unpinned IDs SHALL be returned with `@latest` appended.

#### Scenario: Resolve known direct model ID
- **WHEN** `ResolveVertexModelID` is called with `claude-sonnet-4-5`
- **THEN** it SHALL return `claude-sonnet-4-5@20250929`

#### Scenario: Preserve already pinned Vertex model ID
- **WHEN** `ResolveVertexModelID` is called with `claude-sonnet-4-5@20250929`
- **THEN** it SHALL return `claude-sonnet-4-5@20250929`

#### Scenario: Resolve unknown unpinned model ID
- **WHEN** `ResolveVertexModelID` is called with `some-future-model`
- **THEN** it SHALL return `some-future-model@latest`

### Requirement: Model lists are advisory
The exported model ID list helpers SHALL NOT restrict the model IDs accepted by `New` or `NewVertex`; those constructors SHALL continue to accept arbitrary model ID strings.

#### Scenario: Unknown direct model ID remains accepted
- **WHEN** `New` is called with an ID that is absent from `ModelIDs`
- **THEN** the constructor SHALL still return a language model for that ID

#### Scenario: Unknown Vertex model ID remains accepted
- **WHEN** `NewVertex` is called with an ID that is absent from `VertexModelIDs`
- **THEN** the constructor SHALL still return a language model for that ID
