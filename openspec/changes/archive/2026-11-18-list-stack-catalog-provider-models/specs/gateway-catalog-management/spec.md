# Gateway Catalog Management Delta

## ADDED Requirements

### Requirement: Browse models when adding to a saved provider
A writer with read, update, and `modelcatalogs/available-models:get` access SHALL be able to search a saved provider's listing and type a custom ID in the same model ID field. Opening the list SHALL fetch once or reuse a pending request for the same saved provider. Focusing alone and filtering an open list SHALL NOT request a listing. Selecting a result SHALL fill the field without saving.

#### Scenario: Writer adds a listed model
- **WHEN** the writer opens the model ID field and searches the returned IDs
- **THEN** clicking a match or pressing Enter on a highlighted match SHALL fill the same field
- **AND** **Add model** SHALL keep the existing validation, defaults and save behavior

#### Scenario: Create flow or listing permission absent
- **WHEN** a writer adds a model during catalog creation, before the provider key is saved, or without listing permission
- **THEN** the form SHALL use manual entry without calling the listing action
- **AND** in the create flow it SHALL explain that browsing becomes available after save

### Requirement: The listing does not gate manual model entry
The **Add model** form SHALL accept IDs that are absent from the list, subject to the same validation as manual entry before listing was available. It SHALL show progress, incomplete results, an empty listing, and failure as distinct states without overwriting or disabling manual entry.

#### Scenario: Writer leaves search without confirming
- **WHEN** the writer presses Tab or Escape with a suggestion highlighted
- **THEN** the list SHALL close without replacing the typed model ID

#### Scenario: Listing is empty or fails
- **WHEN** the provider has no suggestions or a listing cannot be obtained
- **THEN** the picker SHALL show a fixed explanation, and the writer SHALL be able to add a custom ID

#### Scenario: Writer reopens the picker while the listing is pending
- **WHEN** the writer closes and reopens the picker before its request settles
- **THEN** the picker SHALL reuse that request and display its result; a successful catalog save SHALL invalidate pending requests for reuse

#### Scenario: Listing or health check is busy
- **WHEN** either action receives `429`
- **THEN** the page SHALL explain that another check is running and suggest trying again shortly

#### Scenario: Catalog changes while listing
- **WHEN** the catalog is saved or the writer closes the picker or leaves the catalog while a listing is in flight
- **THEN** the closed picker or superseded view SHALL NOT display the late result
