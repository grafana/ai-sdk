# gateway-catalog-management Specification

## Purpose
Lets stack administrators manage the stack's `ModelCatalog` resources from the Gateway app, with write-only provider keys, and shows callers each catalog's base URL.

## Requirements

### Requirement: Catalog page
The app SHALL have a model catalogs page that lists the request namespace's `ModelCatalog` resources with their names, providers and model IDs, and shows a catalog's providers, models, aliases, primaries and fallbacks. It SHALL read and write catalogs only through the `ModelCatalog` API as the signed-in user.

#### Scenario: Reader opens the page
- **WHEN** a user who may read catalogs opens the catalog page
- **THEN** the page SHALL list every catalog in the namespace with its providers and model IDs

#### Scenario: No catalogs
- **WHEN** the namespace holds no catalog
- **THEN** the page SHALL say so, and SHALL offer to create one only to a user who may create catalogs

### Requirement: Catalog writes from the page
A user who may write catalogs SHALL be able to create a catalog with a name, providers of type `anthropic` or `openai` with their keys, and models; edit its providers, keys and models; and delete it after confirming. An edit SHALL keep the stored object's metadata, including its resource version.

#### Scenario: Admin creates a catalog
- **WHEN** an Admin fills in a name, a provider with a key and a model, and saves
- **THEN** the page SHALL create the catalog, and the list SHALL show it

#### Scenario: Admin deletes a catalog
- **WHEN** an Admin deletes a catalog and confirms
- **THEN** the catalog SHALL be deleted and leave the list

#### Scenario: Concurrent edit
- **WHEN** an edit is saved after another writer changed the same catalog
- **THEN** the page SHALL show the platform's conflict error and SHALL NOT overwrite the other change

### Requirement: Controls follow the caller's permissions
The page SHALL decide each control from the signed-in user's Grafana permissions for the `ModelCatalog` actions `get`, `create`, `update` and `delete`, never from the org role. Without `get` it SHALL show no catalog content; without `create`, `update` or `delete` the matching control SHALL be absent or read-only.

#### Scenario: Viewer
- **WHEN** a Viewer opens the catalog page and a catalog
- **THEN** they SHALL see the catalog read-only, with no control to create, save or delete

#### Scenario: Editor without the writer role
- **WHEN** an Editor who doesn't hold `aigateway:catalogwriter` opens the catalog page and a catalog
- **THEN** they SHALL see the catalog read-only, with no control to create, save or delete

#### Scenario: Permissions independent of org role
- **WHEN** a user's permissions grant or lack a catalog write action independently of their org role
- **THEN** the matching control SHALL follow the permission

### Requirement: Write-only provider keys
A key entered on the page SHALL be sent only as `secure.<provider>.create` for that provider. The page SHALL never display a stored key or request one. A provider with a stored key SHALL show as configured, with a control to replace the key for a writer.

#### Scenario: Key after saving
- **WHEN** a catalog is saved with a key and then reopened, or the list is shown
- **THEN** the page SHALL NOT contain the key, and the provider SHALL show as configured
- **AND** no response the page fetches and no browser console message SHALL contain the key

#### Scenario: Key on create
- **WHEN** a writer creates a catalog with a key for a provider
- **THEN** the create request SHALL carry it as `secure.<provider>.create`

### Requirement: Unchanged keys are omitted
An update SHALL omit the secure value of every provider whose stored key the writer did not replace, so the platform keeps it. It SHALL send `create` for a replaced key or a new provider's key, and `remove` for the key of a provider the edit deletes or renames.

#### Scenario: Edit without touching a key
- **WHEN** a writer changes a model and saves
- **THEN** the update SHALL carry no secure value for the unchanged providers, and the stored keys SHALL be kept

#### Scenario: Replacing a key
- **WHEN** a writer replaces a provider's key and saves
- **THEN** the update SHALL carry only that provider's `create`

#### Scenario: Changing a stored provider's type
- **WHEN** a writer changes the type of a provider with a stored key
- **THEN** the page SHALL NOT save until a new key is entered for it, and SHALL then send it as `create`

#### Scenario: Removing or renaming a provider
- **WHEN** a writer deletes a provider, or renames it and enters its key, and saves
- **THEN** the update SHALL carry `remove` for the stored key under the old name, and `create` under a new name

### Requirement: Platform errors as returned
When the platform refuses a read or write, the page SHALL show the message the platform returned, including admission refusals, and SHALL keep the form's input. The page SHALL refuse locally only input a request body can't express, such as duplicate provider names or model IDs, and a stored provider whose type changed without a new key. It SHALL NOT repeat admission's checks.

#### Scenario: Admission refuses a catalog
- **WHEN** admission refuses a create or update
- **THEN** the page SHALL show the returned message, and SHALL NOT show any entered key

#### Scenario: Duplicate provider name
- **WHEN** two providers in the form share a name
- **THEN** the page SHALL refuse to send the request and say which name is duplicated

### Requirement: Catalog base URLs on the overview
For a user who may read catalogs, the overview page SHALL list each catalog in the namespace with its base URL, the Gateway URL followed by `/modelcatalogs/{name}`, with a copy control. For other users it SHALL NOT request catalogs.

#### Scenario: Catalogs exist
- **WHEN** a reader opens the overview and the namespace holds catalogs
- **THEN** the overview SHALL show each catalog's base URL

#### Scenario: Catalogs can't be loaded
- **WHEN** listing catalogs fails
- **THEN** the overview SHALL show the returned message in the catalog section, and the rest of the page SHALL still render

### Requirement: Catalog management evidence
Jest tests SHALL cover the request bodies, the permission matrix, key states, error display and the overview. Playwright tests against Grafana with the built plugin SHALL cover an Admin creating, editing and deleting a catalog with keys never displayed, Viewer and Editor read-only pages, and the overview's base URLs.

#### Scenario: End-to-end suite
- **WHEN** the plugin end-to-end suite runs
- **THEN** it SHALL exercise the catalog page in a browser as Admin, Editor and Viewer
