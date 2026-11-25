## ADDED Requirements

### Requirement: New providers connected in the app serve unlisted models
The shared provider form SHALL disclose that a new provider serves every model and save that provider with `serveAllModels: true`. This applies on catalog creation and when connecting another provider. The form SHALL NOT change other providers or the API default for an omitted field.

#### Scenario: Create with one provider and no listed models
- **WHEN** a writer creates a catalog in the app
- **THEN** the app SHALL save its first provider enabled for unlisted serving, with no listed models required

#### Scenario: Connect a provider on a saved catalog
- **WHEN** a writer connects another provider in the app
- **THEN** the update SHALL enable unlisted serving for that provider alone and preserve existing providers and listed models
