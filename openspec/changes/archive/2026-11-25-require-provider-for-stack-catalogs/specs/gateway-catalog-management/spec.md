## MODIFIED Requirements

### Requirement: Catalog writes from the page
A user who may create catalogs SHALL be able to create one with a name and one provider, its key and an operator-allowed endpoint where applicable, then open the saved catalog's page on success. On a stored catalog, a writer SHALL be able to connect and remove providers, replace keys, add, edit and remove models, and delete the catalog from its page after confirming. Each change to the stored catalog SHALL be saved when the writer confirms it, as one update that keeps the loaded object's metadata, including its resource version, and the page SHALL continue from the object the update returns. Removing a provider SHALL remove its key and the models whose primary it is after confirmation, but SHALL be disabled if this would remove the last provider or a surviving model falls back to it.

#### Scenario: Admin creates a catalog
- **WHEN** an Admin enters a valid name and provider details and creates a catalog
- **THEN** the page SHALL send one create with the provider and its secure key and an empty model map, then open the saved catalog's page

#### Scenario: Admin connects the first provider
- **WHEN** an Admin starts a new catalog, or connects another provider from its saved page
- **THEN** the app SHALL use the same provider fields for both flows, with a required endpoint for OpenAI-compatible, an optional endpoint for native providers, and no model input on create

#### Scenario: Admin changes a catalog
- **WHEN** an Admin connects or removes a provider, replaces a key, or adds, edits or removes a model, and confirms
- **THEN** the page SHALL send one update with the stored resource version, and SHALL show the result without reloading

#### Scenario: Admin removes the last model
- **WHEN** an Admin confirms removing the last model
- **THEN** the catalog SHALL have no models and keep its providers and keys

#### Scenario: Provider's primary models have fallbacks elsewhere
- **WHEN** a provider's primary models fall back to other providers and no surviving model falls back to the provider being removed
- **THEN** the page SHALL offer to remove the provider, its key and its primary models together, even if they are the last models
- **AND** it SHALL identify the models to be removed before confirmation, without promoting fallbacks or rewriting surviving models

#### Scenario: Admin removes the last provider
- **WHEN** an Admin views a catalog with only one provider
- **THEN** the provider's remove action SHALL be disabled and SHALL suggest deleting the catalog instead

#### Scenario: Provider used as a fallback
- **WHEN** another provider's model falls back to a provider
- **THEN** the page SHALL NOT offer to remove that provider, and SHALL say why

#### Scenario: Admin deletes a catalog
- **WHEN** an Admin deletes a catalog from its page and confirms
- **THEN** the catalog SHALL be deleted, and the page SHALL return to the list, which no longer shows it

#### Scenario: Concurrent edit
- **WHEN** a change is saved after another writer changed the same catalog
- **THEN** the page SHALL show the platform's conflict error and SHALL NOT overwrite the other change

### Requirement: Write-only provider keys
A key entered on the page SHALL be sent only as `secure.<provider>.create` for that provider. The page SHALL never display a stored key or request one. The page SHALL NOT show a saved-key badge. A writer SHALL have a control to replace a provider's key.

#### Scenario: Key after saving
- **WHEN** a provider is saved with a key, the catalog is reopened, or the list is shown
- **THEN** the page SHALL NOT contain the key or a saved-key badge
- **AND** no response the page fetches and no browser console message SHALL contain the key

#### Scenario: Key on create
- **WHEN** a writer creates a catalog with its first provider
- **THEN** the create SHALL carry the key only as `secure.<provider>.create`

#### Scenario: Key on connect
- **WHEN** a writer connects a provider with a key
- **THEN** the update SHALL carry the key as `secure.<provider>.create`

### Requirement: Check the saved catalog from its page
The saved catalog page SHALL offer an explicit **Check health** action only when the catalog has a connected provider and the caller may read the catalog and has the `modelcatalogs/health` `get` permission. After a successful create, the saved page SHALL check once if the caller has both permissions. It SHALL NOT check on refresh, ordinary arrival, other saves, or from the list or overview. It SHALL prevent duplicate submissions while checking and report route failures separately from probe results.

#### Scenario: Writer checks a catalog
- **WHEN** a caller with both permissions clicks **Check health** on a saved catalog with at least one provider
- **THEN** the page SHALL request one health check for that catalog and display its progress without modifying the catalog

#### Scenario: Caller may read or invoke but may not run health
- **WHEN** a caller without health permission opens a stored catalog
- **THEN** the page SHALL neither show the control nor request health results

#### Scenario: A saved catalog has no providers
- **WHEN** a writer tries to create a catalog without a provider
- **THEN** the catalog SHALL NOT be saved and the page SHALL NOT check health

#### Scenario: A writer arrives after creating a catalog
- **WHEN** a writer with health permission arrives from a successful catalog create
- **THEN** the page SHALL run one check using the saved catalog and show the usual result
- **AND** refreshing or returning to that page SHALL NOT start another check

### Requirement: Browse models when adding to a saved provider
A writer with read, update, and `modelcatalogs/available-models:get` access SHALL be able to search a saved provider's listing and type a custom ID in the same model ID field. Opening the list SHALL fetch once or reuse a pending request for the same saved provider. Focusing alone and filtering an open list SHALL NOT request a listing. Selecting a result SHALL fill the field without saving.

#### Scenario: Writer adds a listed model
- **WHEN** the writer opens the model ID field and searches the returned IDs
- **THEN** clicking a match or pressing Enter on a highlighted match SHALL fill the same field
- **AND** **Add model** SHALL keep the existing validation, defaults and save behavior

#### Scenario: Writer browses before adding the first model
- **WHEN** a writer with listing permission connects a provider and its key is saved
- **THEN** the model ID field SHALL offer suggestions even if the provider has no configured models

#### Scenario: Create flow or listing permission absent
- **WHEN** a writer opens the create form or adds a model to a saved provider without listing permission
- **THEN** the page SHALL NOT offer browsing or call the listing action
- **AND** the writer adding a model SHALL be able to enter its ID manually
