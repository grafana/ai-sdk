## MODIFIED Requirements

### Requirement: Public Vertex model ID resolution
The `providers/anthropic` module's `anthropic` package SHALL export `ResolveVertexModelID(modelID string) string`, mapping model IDs accepted by `New` to the canonical model ID form expected by Vertex AI partner-channel requests.

Known curated direct model IDs SHALL resolve to their curated Vertex IDs. Already `@`-pinned IDs SHALL be returned unchanged. Unknown unpinned IDs SHALL be returned with `@latest` appended.

#### Scenario: Resolve known direct model ID
- **WHEN** `ResolveVertexModelID` is called with `claude-sonnet-4-5`
- **THEN** it SHALL return `claude-sonnet-4-5@20250929`

#### Scenario: Resolve undated Vertex model ID
- **WHEN** `ResolveVertexModelID` is called with `claude-sonnet-5-5`
- **THEN** it SHALL return `claude-sonnet-5-5`, because Vertex serves that model without a date suffix, and SHALL NOT append `@latest`

#### Scenario: Preserve already pinned Vertex model ID
- **WHEN** `ResolveVertexModelID` is called with `claude-sonnet-4-5@20250929`
- **THEN** it SHALL return `claude-sonnet-4-5@20250929`

#### Scenario: Resolve unknown unpinned model ID
- **WHEN** `ResolveVertexModelID` is called with `some-future-model`
- **THEN** it SHALL return `some-future-model@latest`
