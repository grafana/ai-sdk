# gateway-catalog-management Specification

## Purpose
Lets stack administrators manage the stack's `ModelCatalog` resources from the Gateway app, with write-only provider keys, and shows callers each catalog's base URL.

## Requirements

### Requirement: Catalog page
The app SHALL have a model catalogs page that lists the request namespace's `ModelCatalog` resources with their names, base URLs, provider types and provider and model counts, and a page per catalog that shows its base URL, its providers with their types, key states and the models that use them, and its models with their display names, descriptions, aliases, primaries and fallbacks. It SHALL read and write catalogs only through the `ModelCatalog` API as the signed-in user.

#### Scenario: Reader opens the page
- **WHEN** a user who may read catalogs opens the catalog page
- **THEN** the page SHALL list every catalog in the namespace with its base URL, its provider types and its provider and model counts

#### Scenario: Reader opens a catalog
- **WHEN** a user who may read catalogs opens a catalog's page
- **THEN** the page SHALL show each provider with its type, whether it has a saved key and which models use it, and each model with the providers and models it calls in order

#### Scenario: No catalogs
- **WHEN** the namespace holds no catalog
- **THEN** the page SHALL say so, and SHALL offer to create one only to a user who may create catalogs

### Requirement: Catalog writes from the page
A user who may write catalogs SHALL be able to create a catalog with a name, one provider of type `anthropic` or `openai` with its key, and one model; then add, edit and remove the catalog's providers and models one at a time; and delete the catalog after confirming. Each change SHALL be saved when the writer confirms it, as one update that keeps the loaded object's metadata, including its resource version, and the page SHALL continue from the object the update returns. Catalog names, provider names and model IDs SHALL be fixed once saved. The page SHALL NOT offer to remove a provider that a model uses, or a catalog's last model.

#### Scenario: Admin creates a catalog
- **WHEN** an Admin enters a name, a provider with a key and a model, and creates the catalog
- **THEN** the page SHALL create the catalog and open its page

#### Scenario: Admin changes a catalog
- **WHEN** an Admin adds, edits or removes a provider or model and confirms
- **THEN** the page SHALL send one update with the stored resource version, and SHALL show the result without reloading

#### Scenario: Provider in use
- **WHEN** a model calls a provider as its primary or a fallback
- **THEN** the page SHALL NOT offer to remove that provider, and SHALL say why

#### Scenario: Admin deletes a catalog
- **WHEN** an Admin deletes a catalog and confirms
- **THEN** the catalog SHALL be deleted and leave the list

#### Scenario: Concurrent edit
- **WHEN** a change is saved after another writer changed the same catalog
- **THEN** the page SHALL show the platform's conflict error and SHALL NOT overwrite the other change

### Requirement: Controls follow the caller's permissions
The page SHALL decide each control from the signed-in user's Grafana permissions for the `ModelCatalog` actions `get`, `create`, `update` and `delete`, never from the org role. Without `get` it SHALL show no catalog content; without `create`, `update` or `delete` the matching control SHALL be absent.

#### Scenario: Viewer
- **WHEN** a Viewer opens the catalog page and a catalog
- **THEN** they SHALL see the catalog read-only, with no control to create, change or delete

#### Scenario: Editor without the writer role
- **WHEN** an Editor who doesn't hold `aigateway:catalogwriter` opens the catalog page and a catalog
- **THEN** they SHALL see the catalog read-only, with no control to create, change or delete

#### Scenario: Permissions independent of org role
- **WHEN** a user's permissions grant or lack a catalog write action independently of their org role
- **THEN** the matching control SHALL follow the permission

### Requirement: Write-only provider keys
A key entered on the page SHALL be sent only as `secure.<provider>.create` for that provider. The page SHALL never display a stored key or request one. A provider with a stored key SHALL show that its key is saved, with a control to replace the key for a writer.

#### Scenario: Key after saving
- **WHEN** a provider is saved with a key, the catalog is reopened, or the list is shown
- **THEN** the page SHALL NOT contain the key, and the provider SHALL show that its key is saved
- **AND** no response the page fetches and no browser console message SHALL contain the key

#### Scenario: Key on create
- **WHEN** a writer creates a catalog or adds a provider with a key
- **THEN** the request SHALL carry it as `secure.<provider>.create`

### Requirement: Unchanged keys are omitted
An update SHALL omit the secure value of every provider whose stored key the writer did not replace, so the platform keeps it. It SHALL send `create` for a replaced key or a new provider's key, and `remove` for the key of a removed provider.

#### Scenario: Edit without touching a key
- **WHEN** a writer changes a model, or saves a provider without a new key
- **THEN** the update SHALL carry no secure value, and the stored keys SHALL be kept

#### Scenario: Replacing a key
- **WHEN** a writer replaces a provider's key and saves
- **THEN** the update SHALL carry only that provider's `create`

#### Scenario: Changing a stored provider's type
- **WHEN** a writer changes the type of a provider with a stored key
- **THEN** the page SHALL NOT save until a new key is entered for it, and SHALL then send it as `create`

#### Scenario: Removing a provider
- **WHEN** a writer removes a provider and confirms
- **THEN** the update SHALL carry `remove` for its stored key

### Requirement: Platform errors as returned
When the platform refuses a read or write, the page SHALL show the message the platform returned, in the dialog of the refused change when there is one, and SHALL keep the change's input. The page SHALL refuse locally only a missing value, a provider name or model ID the catalog already has, and a stored provider whose type changed without a new key. It SHALL leave name, ID, alias and policy rules to admission.

#### Scenario: Admission refuses a catalog
- **WHEN** admission refuses a create or update
- **THEN** the page SHALL show the returned message, and SHALL NOT show any entered key

#### Scenario: Duplicate provider name
- **WHEN** a writer adds a provider with a name the catalog already has
- **THEN** the page SHALL refuse to send the request and say which name is taken

### Requirement: Catalog base URLs on the overview
For a user who may read catalogs, the overview page SHALL list each catalog in the namespace with its base URL, the Gateway URL followed by `/modelcatalogs/{name}`, with a copy control and a link to the catalog's page. For other users it SHALL NOT request catalogs.

#### Scenario: Catalogs exist
- **WHEN** a reader opens the overview and the namespace holds catalogs
- **THEN** the overview SHALL show each catalog's base URL

#### Scenario: Catalogs can't be loaded
- **WHEN** listing catalogs fails
- **THEN** the overview SHALL show the returned message in the catalog section, and the rest of the page SHALL still render

### Requirement: Catalog management evidence
Jest tests SHALL cover the request bodies, the permission matrix, key states, local checks, error display and the overview. Playwright tests against Grafana with the built plugin SHALL cover an Admin creating a catalog, editing a model, replacing a key, adding and removing a provider, and deleting a catalog with keys never displayed, Viewer and Editor read-only pages, and the overview's base URLs.

#### Scenario: End-to-end suite
- **WHEN** the plugin end-to-end suite runs
- **THEN** it SHALL exercise the catalog page in a browser as Admin, Editor and Viewer
