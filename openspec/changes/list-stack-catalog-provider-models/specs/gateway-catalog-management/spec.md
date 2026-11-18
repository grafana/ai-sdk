# Gateway Catalog Management Delta

## ADDED Requirements

### Requirement: Browse models when adding to a saved provider
A writer who can read and update a saved catalog and has `get` on `modelcatalogs/available-models` SHALL be able to browse and search a named saved provider's listing from **Add model**. Selecting an ID SHALL fill the provider model ID field without saving. The listing SHALL be fetched only on explicit request and search SHALL NOT trigger further provider requests.

#### Scenario: Writer adds a listed model
- **WHEN** the writer opens the picker and searches the returned IDs
- **THEN** the writer SHALL be able to select a match and add it with the existing model defaults, validation and save behavior

#### Scenario: Create flow or listing permission absent
- **WHEN** a writer adds a model during catalog creation, before the provider key is saved, or without listing permission
- **THEN** the form SHALL use manual entry without calling the listing action
- **AND** in the create flow it SHALL explain that browsing becomes available after save

### Requirement: The listing does not gate manual model entry
The **Add model** form SHALL accept IDs that are absent from the list, subject to the same validation as manual entry before listing was available. It SHALL show progress, incomplete results, an empty listing, and failure as distinct states without overwriting or disabling manual entry.

#### Scenario: Listing is empty or fails
- **WHEN** the provider has no suggestions or a listing cannot be obtained
- **THEN** the picker SHALL show a fixed explanation, and the writer SHALL be able to add a custom ID

#### Scenario: Catalog changes while listing
- **WHEN** the catalog is saved or the writer closes the picker or leaves the catalog while a listing is in flight
- **THEN** the page SHALL discard that listing and SHALL NOT show its late result
