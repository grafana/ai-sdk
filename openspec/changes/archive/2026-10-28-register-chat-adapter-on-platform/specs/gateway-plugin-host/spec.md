## REMOVED Requirements

### Requirement: Routes-only app plugin

**Reason:** The two-route manifest restriction is replaced by the three-route
manifest with the explicitly registered Chat operation.

**Migration:** Preserve both ProviderWire routes, plugin identity and the
undeclared-route refusal; add only the namespaced Chat operation.

#### Scenario: Two-route restriction is replaced
- **WHEN** the Chat host integration is applied
- **THEN** the replacement requirement preserves the two ProviderWire routes
  and permits the declared Chat route as the third operation.

## ADDED Requirements

### Requirement: Routes-only app plugin with Chat

The Gateway SHALL ship as one Grafana app plugin with no kinds and one API
version declaring exactly three namespaced operations: GET `config`, POST
`language-model` and POST `v1/chat/completions`. The manifest SHALL declare no
other routes, kinds or admission/conversion hooks and SHALL match the
production route table.

#### Scenario: Committed three-route manifest
- **WHEN** the committed generated manifest is inspected
- **THEN** it declares one version, no kinds and exactly those three operations.

#### Scenario: Plugin identity is fixed
- **WHEN** the backend is built by any supported method, including plain Go
  builds in tests
- **THEN** it identifies as the plugin ID declared in the plugin metadata.

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path or uses an undeclared method
- **THEN** no discovery, selection or provider work occurs for it.
