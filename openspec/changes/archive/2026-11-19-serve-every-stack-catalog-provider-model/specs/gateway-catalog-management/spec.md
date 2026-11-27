# Spec Delta

## ADDED Requirements

### Requirement: Provider serving settings on the catalog page
Each saved provider card SHALL show its saved serving mode and explain the qualified selector. It SHALL offer one model picker at a time: to add listed models in listed-only mode, or to edit **Blocked model IDs** in serve-every mode. In listed-only mode the card SHALL hide blocklist editing. Existing listed models SHALL remain visible and editable in either mode.

#### Scenario: Provider connected without listed models
- **WHEN** a writer opens a saved provider's card before listing a model
- **THEN** the settings SHALL be available and the page SHALL distinguish having no listed models from serving no models

#### Scenario: Writer disables unlisted serving
- **WHEN** a writer turns the switch off and the update succeeds
- **THEN** the provider SHALL serve only listed models, the editor SHALL be hidden, and the saved blocklist SHALL be retained

#### Scenario: Writer enables unlisted serving
- **WHEN** a writer turns on **Serve every model** and the update succeeds
- **THEN** the add-listed-model control SHALL be replaced by the blocklist editor without changing listed models

#### Scenario: Writer edits the blocklist while listed-only
- **WHEN** a writer opens a provider card in listed-only mode
- **THEN** the card SHALL offer to add listed models and SHALL NOT offer to edit blocked IDs

#### Scenario: Writer edits the blocklist in serve-every mode
- **WHEN** a writer opens a provider card in serve-every mode
- **THEN** the editor SHALL show the saved blocklist and accept a typed ID, with suggestions from the saved account on demand only when the writer may list provider models

#### Scenario: Read-only catalog
- **WHEN** a user can read but not update the catalog
- **THEN** the card SHALL show its serving mode and, only in serve-every mode, its blocked IDs read-only

### Requirement: Save each serving action without key changes
Toggling **Serve every model**, adding an ID, or removing an ID SHALL each send one versioned update without secure values or changes to listed models. Typing or choosing a suggestion SHALL NOT save. A write SHALL NOT start a provider listing or health check. The card SHALL display the saved settings until the write succeeds.

#### Scenario: Enable serving with an empty or prepared blocklist
- **WHEN** the writer turns the switch on with an empty or previously saved blocklist
- **THEN** the page SHALL send one update enabling unlisted serving with that same blocklist, without requiring another action

#### Scenario: Refused update
- **WHEN** admission rejects a blocklist conflict or the platform detects a stale resource version
- **THEN** the card SHALL show the returned error, retain any ID being added for correction, and leave the saved policy unchanged by this attempted edit

### Requirement: New providers connected in the app serve unlisted models
The shared provider form SHALL disclose that a new provider serves every model and save that provider with `serveAllModels: true`. This applies on catalog creation and when connecting another provider. The form SHALL NOT change other providers or the API default for an omitted field.

#### Scenario: Create with one provider and no listed models
- **WHEN** a writer creates a catalog in the app
- **THEN** the app SHALL save its first provider enabled for unlisted serving, with no listed models required

#### Scenario: Connect a provider on a saved catalog
- **WHEN** a writer connects another provider in the app
- **THEN** the update SHALL enable unlisted serving for that provider alone and preserve existing providers and listed models
