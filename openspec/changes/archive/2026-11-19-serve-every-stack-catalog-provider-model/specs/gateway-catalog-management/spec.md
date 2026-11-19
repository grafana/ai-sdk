# Spec Delta

## ADDED Requirements

### Requirement: Provider serving settings on the catalog page
Each saved provider card SHALL show whether it serves only listed models or also unlisted models, and its blocked model IDs. Writers SHALL be able to edit a **Serve every model** switch and the adjacent **Blocked model IDs** field together, with the catalog instance's selector format explained. Readers without update permission SHALL see the saved settings without editing controls.

#### Scenario: Provider connected without listed models
- **WHEN** a writer opens a saved provider's card before listing a model
- **THEN** the settings SHALL be available and the page SHALL distinguish having no listed models from serving no models

#### Scenario: Writer disables unlisted serving
- **WHEN** a writer turns the switch off and saves without editing the blocklist
- **THEN** the provider SHALL serve only listed models and the saved blocklist SHALL be retained

#### Scenario: Read-only catalog
- **WHEN** a user can read but not update the catalog
- **THEN** the card SHALL show its serving mode and blocked IDs read-only

### Requirement: Save serving settings together without key changes
Changing the switch or blocklist SHALL NOT write until the writer explicitly saves. One update SHALL carry both settings and the stored resource version, without secure values or changes to listed models. The edit and save SHALL NOT run a provider listing or health check. The card SHALL show save refusals and retain the draft.

#### Scenario: Enable serving with a blocklist
- **WHEN** the writer turns the switch on, enters blocked IDs, and saves
- **THEN** the page SHALL send one update and show the saved settings returned by the API, without any interim state that serves every model without the blocklist

#### Scenario: Refused update
- **WHEN** admission rejects a blocklist conflict or the platform detects a stale resource version
- **THEN** the card SHALL show the returned error, preserve the draft, and leave the saved policy unchanged by this attempted edit
