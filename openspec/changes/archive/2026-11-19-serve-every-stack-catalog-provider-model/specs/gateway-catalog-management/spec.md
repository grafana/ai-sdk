# Spec Delta

## ADDED Requirements

### Requirement: Provider serving settings on the catalog page
Each saved provider card SHALL show its serving mode. Writers SHALL be able to edit the **Serve every model** switch and blocklist together. In listed-only mode the card SHALL offer to add a listed model. In serve-every mode it SHALL show the **Blocked model IDs** editor instead, with the qualified selector format explained. Existing listed models SHALL remain visible and editable in either mode.

#### Scenario: Provider connected without listed models
- **WHEN** a writer opens a saved provider's card before listing a model
- **THEN** the settings SHALL be available and the page SHALL distinguish having no listed models from serving no models

#### Scenario: Writer disables unlisted serving
- **WHEN** a writer turns the switch off and saves without editing the blocklist
- **THEN** the provider SHALL serve only listed models and the saved blocklist SHALL be retained

#### Scenario: Writer changes the draft mode
- **WHEN** a writer turns on **Serve every model**
- **THEN** the add-listed-model control SHALL be replaced by the blocklist editor without saving or changing listed models
- **AND** the editor SHALL accept a typed ID, and offer suggestions from the saved account on demand only when the writer may list provider models

#### Scenario: Read-only catalog
- **WHEN** a user can read but not update the catalog
- **THEN** the card SHALL show its serving mode and blocked IDs read-only

### Requirement: Save serving settings together without key changes
Changing the switch or blocklist SHALL NOT write until the writer explicitly saves. One update SHALL carry both settings and the stored resource version, without secure values or changes to listed models. Toggling the switch and saving SHALL NOT run a provider listing or health check. The card SHALL show save refusals and retain the draft.

#### Scenario: Enable serving with a blocklist
- **WHEN** the writer turns the switch on, enters blocked IDs, and saves
- **THEN** the page SHALL send one update and show the saved settings returned by the API, without any interim state that serves every model without the blocklist

#### Scenario: Refused update
- **WHEN** admission rejects a blocklist conflict or the platform detects a stale resource version
- **THEN** the card SHALL show the returned error, preserve the draft, and leave the saved policy unchanged by this attempted edit
