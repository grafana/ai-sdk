# gateway-catalog-management Specification

## Purpose
Lets stack administrators manage the stack's `ModelCatalog` resources from the Gateway app, with write-only provider keys, and shows callers each catalog's base URL.

## Requirements

### Requirement: Catalog page
The app SHALL have a model catalogs page that lists the request namespace's `ModelCatalog` resources with their names, base URLs, provider types and provider and model counts, and a page per catalog that shows its base URL and each provider with its type, whether it has a saved key, and the models whose primary it is, with their display names, aliases and fallbacks. It SHALL read and write catalogs only through the `ModelCatalog` API as the signed-in user.

#### Scenario: Reader opens the page
- **WHEN** a user who may read catalogs opens the catalog page
- **THEN** the page SHALL list every catalog in the namespace with its base URL, its provider types and its provider and model counts

#### Scenario: Reader opens a catalog
- **WHEN** a user who may read catalogs opens a catalog's page
- **THEN** the page SHALL show each provider with its key state and the models it serves, and SHALL show a model's details read-only

#### Scenario: No catalogs
- **WHEN** the namespace holds no catalog
- **THEN** the page SHALL say so, and SHALL offer to create one only to a user who may create catalogs

### Requirement: Catalog writes from the page
A user who may write catalogs SHALL be able to create a catalog by naming it, connecting providers of type `anthropic` or `openai` with their keys and adding models, sent as one create; then connect and remove providers, replace keys, add, edit and remove models, and delete the catalog from its page after confirming. On a stored catalog each change SHALL be saved when the writer confirms it, as one update that keeps the loaded object's metadata, including its resource version, and the page SHALL continue from the object the update returns. Removing a provider SHALL remove the models whose primary it is, after the writer confirms both. The page SHALL NOT offer to remove a provider that another provider's model falls back to, or a stored catalog's last model.

#### Scenario: Admin creates a catalog
- **WHEN** an Admin names a catalog, connects a provider with a key, adds a model and creates the catalog
- **THEN** the page SHALL send one create and open the catalog's page

#### Scenario: Admin changes a catalog
- **WHEN** an Admin connects or removes a provider, replaces a key, or adds, edits or removes a model, and confirms
- **THEN** the page SHALL send one update with the stored resource version, and SHALL show the result without reloading

#### Scenario: Provider used as a fallback
- **WHEN** another provider's model falls back to a provider
- **THEN** the page SHALL NOT offer to remove that provider, and SHALL say why

#### Scenario: Admin deletes a catalog
- **WHEN** an Admin deletes a catalog from its page and confirms
- **THEN** the catalog SHALL be deleted, and the page SHALL return to the list, which no longer shows it

#### Scenario: Concurrent edit
- **WHEN** a change is saved after another writer changed the same catalog
- **THEN** the page SHALL show the platform's conflict error and SHALL NOT overwrite the other change

### Requirement: Catalog defaults
The page SHALL ask only for what a common catalog needs, and derive the rest. A new provider SHALL be named after its type, and the page SHALL ask for a name only when the catalog already has a provider with that name. A model added by its provider model ID SHALL use that ID as its model ID and display name. A model's ID, display name, description, aliases, primary and fallbacks SHALL be editable from the model, with the display name defaulting to the ID when left empty. Provider and catalog names SHALL be fixed once saved.

#### Scenario: Writer adds a provider model
- **WHEN** a writer adds the provider model `claude-sonnet-4-6` to the `anthropic` provider
- **THEN** the catalog SHALL gain the model `claude-sonnet-4-6`, named `claude-sonnet-4-6`, with primary `anthropic` and `claude-sonnet-4-6`

#### Scenario: Second account of a type
- **WHEN** a writer connects a provider of a type whose name the catalog already uses
- **THEN** the page SHALL ask for the new provider's name, suggesting the type with the first free number

#### Scenario: Writer changes a model's ID
- **WHEN** a writer gives a stored model another ID and saves
- **THEN** the update SHALL carry the model under the new ID only, with its other fields unchanged

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
An update SHALL omit the secure value of every provider whose stored key the writer did not replace, so the platform keeps it. It SHALL send `create` for a replaced key or a new provider's key, and `remove` for the key of a removed provider. A provider's type SHALL NOT change once connected, because its key belongs to that type's account.

#### Scenario: Edit without touching a key
- **WHEN** a writer changes a model
- **THEN** the update SHALL carry no secure value, and the stored keys SHALL be kept

#### Scenario: Replacing a key
- **WHEN** a writer replaces a provider's key and saves
- **THEN** the update SHALL carry only that provider's `create`

#### Scenario: Removing a provider
- **WHEN** a writer removes a provider and confirms
- **THEN** the update SHALL carry `remove` for its stored key

### Requirement: Platform errors as returned
When the platform refuses a read or write, the page SHALL show the message the platform returned, in the dialog of the refused change when there is one, and SHALL keep the change's input. The page SHALL refuse locally a missing value, a provider name or model ID the catalog already has, and a name, ID or alias outside the platform's format: Grafana's resource name rules for the catalog name, DNS labels for provider names, and the Gateway's public ID format for model IDs and aliases. It SHALL check the catalog name as the writer types, and SHALL show the catalog's URL only for a valid name. Other fields SHALL show a format error once the writer leaves them, and a missing value only when the writer saves; a shown error SHALL clear as soon as its value becomes valid. Collisions between aliases and IDs, and policy rules, SHALL be left to admission.

#### Scenario: Admission refuses a catalog
- **WHEN** admission refuses a create or update
- **THEN** the page SHALL show the returned message, and SHALL NOT show any entered key

#### Scenario: Taken model ID
- **WHEN** a writer adds a provider model whose ID the catalog already has
- **THEN** the page SHALL refuse to send the request and say which ID is taken

#### Scenario: Writer fixes a value
- **WHEN** a field shows an error and the writer corrects its value
- **THEN** the error SHALL disappear without another save

#### Scenario: Invalid catalog name
- **WHEN** a writer types a catalog name with a character Grafana refuses
- **THEN** the page SHALL say which characters are allowed, and SHALL NOT show a URL for it

### Requirement: Catalog base URLs on the overview
For a user who may read catalogs, the overview page SHALL list each catalog in the namespace with its base URL, the Gateway URL followed by `/modelcatalogs/{name}`, with a copy control and a link to the catalog's page. For other users it SHALL NOT request catalogs.

#### Scenario: Catalogs exist
- **WHEN** a reader opens the overview and the namespace holds catalogs
- **THEN** the overview SHALL show each catalog's base URL

#### Scenario: Catalogs can't be loaded
- **WHEN** listing catalogs fails
- **THEN** the overview SHALL show the returned message in the catalog section, and the rest of the page SHALL still render

### Requirement: Catalog management evidence
Jest tests SHALL cover the request bodies, the defaults, the permission matrix, key states, local checks, error display and the overview. Playwright tests against Grafana with the built plugin SHALL cover an Admin creating a catalog, editing and adding models, replacing a key, connecting and removing a provider, and deleting a catalog with keys never displayed, Viewer and Editor read-only pages, and the overview's base URLs.

#### Scenario: End-to-end suite
- **WHEN** the plugin end-to-end suite runs
- **THEN** it SHALL exercise the catalog page in a browser as Admin, Editor and Viewer
