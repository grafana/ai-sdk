# Spec Delta

## ADDED Requirements

### Requirement: Provider serving settings on the catalog page
Each saved provider card SHALL show its saved serving mode and explain the qualified selector. It SHALL offer one model picker at a time: to add listed models in listed-only mode, or to edit **Blocked model IDs** in serve-every mode. Writers SHALL be able to open the blocklist editor while listed-only without changing mode. Existing listed models SHALL remain visible and editable in either mode.

#### Scenario: Provider connected without listed models
- **WHEN** a writer opens a saved provider's card before listing a model
- **THEN** the settings SHALL be available and the page SHALL distinguish having no listed models from serving no models

#### Scenario: Writer disables unlisted serving
- **WHEN** a writer turns the switch off and the update succeeds
- **THEN** the provider SHALL serve only listed models and the saved blocklist SHALL be retained

#### Scenario: Writer enables unlisted serving
- **WHEN** a writer turns on **Serve every model** and the update succeeds
- **THEN** the add-listed-model control SHALL be replaced by the blocklist editor without changing listed models

#### Scenario: Writer edits the blocklist while listed-only
- **WHEN** a writer opens **Edit blocked IDs** while listed-only
- **THEN** the blocklist editor SHALL replace the add-listed-model control without making a write or changing serving mode
- **AND** the editor SHALL accept a typed ID, and offer suggestions from the saved account on demand only when the writer may list provider models

#### Scenario: Read-only catalog
- **WHEN** a user can read but not update the catalog
- **THEN** the card SHALL show its serving mode and blocked IDs read-only

### Requirement: Save each serving action without key changes
Toggling **Serve every model**, adding an ID, or removing an ID SHALL each send one versioned update without secure values or changes to listed models. Typing or choosing a suggestion SHALL NOT save. A write SHALL NOT start a provider listing or health check. The card SHALL display the saved settings until the write succeeds.

#### Scenario: Enable serving with an empty or prepared blocklist
- **WHEN** the writer turns the switch on with an empty blocklist or one saved while the switch was off
- **THEN** the page SHALL send one update enabling unlisted serving with that same blocklist, without requiring another action

#### Scenario: Refused update
- **WHEN** admission rejects a blocklist conflict or the platform detects a stale resource version
- **THEN** the card SHALL show the returned error, retain any ID being added for correction, and leave the saved policy unchanged by this attempted edit
