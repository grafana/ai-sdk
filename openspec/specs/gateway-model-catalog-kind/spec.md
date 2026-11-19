# gateway-model-catalog-kind Specification

## Purpose
Lets a stack store AI Gateway model catalogs, with provider API keys held as Grafana secure values, as validated and access-controlled App Platform resources.

## Requirements

### Requirement: ModelCatalog resource
The Gateway's API group SHALL serve a namespaced `ModelCatalog` kind in version `v1alpha1` that is not folder-scoped. Its `spec` SHALL describe providers, models, aliases, primaries and fallbacks with the same field names as the operator catalog document, without `apiKeyEnv`.

#### Scenario: Catalog saved without a folder
- **WHEN** a writer creates a `ModelCatalog` without a folder annotation
- **THEN** it SHALL be stored in the request namespace

#### Scenario: Catalog with a folder annotation
- **WHEN** a writer creates a `ModelCatalog` that names a folder
- **THEN** the platform SHALL refuse it

#### Scenario: Operator catalog document as a spec
- **WHEN** a valid operator catalog document without `apiKeyEnv`, with only `anthropic` and `openai` providers, is used as a `spec` together with one secure value per provider
- **THEN** the catalog SHALL be accepted

### Requirement: Unsupported fields are refused, not pruned
The kind schema SHALL keep the operator provider fields `baseURL`, `providerName` and `supportsStructuredOutputs`, so schema pruning cannot remove them before admission. Admission SHALL refuse any catalog that sets one of them, with any value.

#### Scenario: Endpoint override
- **WHEN** a catalog provider sets `baseURL`
- **THEN** the write SHALL be refused, and no key SHALL be sent anywhere

#### Scenario: Compatibility field set to its default
- **WHEN** a catalog provider sets `supportsStructuredOutputs: false` or an empty `providerName`
- **THEN** the write SHALL be refused

### Requirement: Stack catalog provider policy
Admission SHALL accept only provider types `anthropic` and `openai`, and provider names that are DNS labels: 1 to 63 lowercase ASCII letters, digits or `-`, starting and ending with a letter or digit.

#### Scenario: Compatible provider type
- **WHEN** a catalog provider has type `openai-compatible` or an unknown type
- **THEN** the write SHALL be refused

#### Scenario: Provider name outside the pattern
- **WHEN** a provider name contains an uppercase letter, `.`, `_`, `/` or a space, or exceeds 63 characters
- **THEN** the write SHALL be refused

### Requirement: Shared catalog shape validation
A stack catalog SHALL require `spec.providers` and `spec.models` maps, which MAY be empty. Admission SHALL apply the shared checks to entries that are present: provider shape, required model fields, provider references, unique candidates, public ID and alias syntax and collisions, discovery cardinality and attempt bounds, and discovery text bounds. The operator catalog SHALL still require at least one provider and one model and SHALL retain its credential checks.

#### Scenario: No providers or models
- **WHEN** a writer creates or updates a stack catalog with both maps empty and no resulting secure keys
- **THEN** admission SHALL accept the write

#### Scenario: Missing or null map
- **WHEN** a write omits a required map or sends it as null
- **THEN** the write SHALL be refused

#### Scenario: Provider connected before adding a model
- **WHEN** a writer creates or updates a stack catalog with an empty model map and one secure value for each valid provider with no extra secure values
- **THEN** admission SHALL accept the write

#### Scenario: Unknown provider reference
- **WHEN** a model's primary or fallback names a provider the catalog does not declare
- **THEN** the write SHALL be refused

#### Scenario: Alias collision
- **WHEN** an alias equals another model's ID or alias
- **THEN** the write SHALL be refused

#### Scenario: Too many attempts
- **WHEN** a model's primary and fallbacks exceed the provider attempt bound
- **THEN** the write SHALL be refused

### Requirement: One secure value per provider
Each provider's API key SHALL be a secure value whose key is the provider name. After an operation, the resource's secure keys and its providers SHALL match one to one. Reads SHALL return only secure value names, never the key.

#### Scenario: Key created inline
- **WHEN** a writer creates a catalog with `secure.<provider>.create`
- **THEN** the platform SHALL store a secret, and reads of the catalog SHALL return only `secure.<provider>.name`

#### Scenario: Provider without a key
- **WHEN** a provider has no secure value after the operation
- **THEN** the write SHALL be refused

#### Scenario: Key without a provider
- **WHEN** a secure key names no provider
- **THEN** the write SHALL be refused

### Requirement: Secure values merged on update
On update, admission SHALL compute the resulting secure keys as the platform will store them: the stored keys, plus keys the update creates or keeps by their unchanged stored name, minus keys it removes. It SHALL check the one-to-one rule against that result.

#### Scenario: Update omits secure values
- **WHEN** a writer updates a catalog's spec without `secure`
- **THEN** the stored secure values SHALL be kept and the update SHALL pass

#### Scenario: Removing a key a provider uses
- **WHEN** an update sets `secure.<provider>.remove` while the provider stays in the spec
- **THEN** the update SHALL be refused

#### Scenario: Removing a provider and its key
- **WHEN** an update removes a provider from the spec and sets `remove` on its secure key
- **THEN** the update SHALL pass

#### Scenario: Replacing a key
- **WHEN** an update sets `create` on a stored key
- **THEN** the update SHALL pass

#### Scenario: Read, modify and write
- **WHEN** a writer sends back a catalog it read, with each stored secure value name unchanged
- **THEN** the update SHALL pass

### Requirement: No shared secret references
Admission SHALL refuse a secure value that references a secret by `name`, unless it is an update that repeats the name already stored under the same key. A catalog SHALL NOT reference a secret it does not own.

#### Scenario: Reference on create
- **WHEN** a catalog is created with `secure.<provider>.name`
- **THEN** the write SHALL be refused

#### Scenario: Reference to another secret on update
- **WHEN** an update sets a secure name that differs from the stored name under that key
- **THEN** the update SHALL be refused

#### Scenario: Swapping stored names between keys
- **WHEN** an update sets a key's name to the name stored under another key of the same catalog
- **THEN** the update SHALL be refused

### Requirement: Secret-safe admission refusals
Admission refusals SHALL be `400 Bad Request` with a reason naming the failing field path. Neither the reason nor any log or metric the plugin produces SHALL contain a secure value's `create` content. Reasons MAY name catalog identifiers such as provider names, secure keys, model IDs and aliases.

#### Scenario: Invalid catalog with plaintext keys
- **WHEN** a refused create or update carries plaintext keys in `create`
- **THEN** the response body and the plugin's logs SHALL NOT contain any of them

### Requirement: Catalog admission scope
Admission SHALL review `CREATE` and `UPDATE` only. Deletes SHALL NOT be reviewed, so a catalog can be deleted while the plugin is unavailable, and writes SHALL fail closed while it is.

#### Scenario: Delete
- **WHEN** a writer deletes a catalog
- **THEN** the plugin SHALL receive no admission request

### Requirement: Catalog roles
The manifest SHALL declare `aigateway:catalogreader` (viewer permission set on `ModelCatalog`, bound to Viewer), `aigateway:catalogwriter` (editor set on `ModelCatalog` and `get` on `ModelCatalog/health` and `ModelCatalog/available-models`, bound to Admin) and `aigateway:modelinvoker` (`get` on `ModelCatalog`, `ModelCatalog/language-model`, `ModelCatalog/config` and `ModelCatalog/messages`, bound to Editor). Gateway role names SHALL use lowercase letters and digits only.

#### Scenario: Viewer
- **WHEN** a Viewer or Editor gets or lists catalogs in their namespace
- **THEN** the request SHALL be allowed, and their create, update and delete requests SHALL be refused

#### Scenario: Admin
- **WHEN** an Admin creates, updates or deletes a catalog
- **THEN** authorization SHALL allow the request, subject to admission

#### Scenario: Invoker
- **WHEN** an Editor or Admin calls `language-model`, `config` or `v1/messages` on a catalog in their namespace
- **THEN** Grafana's declared check SHALL allow it, and a Viewer's call SHALL be refused with `403` before the plugin is called

#### Scenario: Authorized health check
- **WHEN** a caller has access to the parent and `get` on `modelcatalogs/health`
- **THEN** Grafana's declared check SHALL allow the health action regardless of the caller's org role, subject to the plugin's catalog caller and parent checks

#### Scenario: Reader or invoker without health access
- **WHEN** a caller can read the parent catalog but does not have `get` on `modelcatalogs/health`
- **THEN** Grafana SHALL return `403` before decrypting the parent's keys or calling the plugin

#### Scenario: Authorized provider listing
- **WHEN** a caller has access to the parent and `get` on `modelcatalogs/available-models`
- **THEN** Grafana's declared check SHALL allow the listing action regardless of the caller's org role, subject to the plugin's catalog caller and parent checks

#### Scenario: Reader or invoker without listing access
- **WHEN** a caller can read the parent catalog but does not have `get` on `modelcatalogs/available-models`
- **THEN** Grafana SHALL return `403` before decrypting the parent's keys or calling the plugin

### Requirement: Catalog platform requirements
Operator documentation SHALL state the minimum self-hosted Grafana version and feature toggles that store plugin kinds with secure values and register manifest roles, and that Grafana Cloud does not yet store secure values for plugin kinds.

#### Scenario: Operator reads the requirements
- **WHEN** an operator reads the Gateway requirements
- **THEN** they SHALL find the minimum version, the toggles and the Cloud limitation

### Requirement: Catalog evidence against Grafana
Roles, folderless saves, secure storage and reads, updates, refusals, secret-safe errors and the catalog routes SHALL be proven by end-to-end tests against a running Grafana with the built plugin. The shape, policy, key and merge checks SHALL also have unit tests.

#### Scenario: End-to-end suite
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise each behavior in this capability through Grafana's API

### Requirement: Stored catalogs serve only their catalog routes
A stored `ModelCatalog` SHALL be discoverable and invocable only through its own catalog routes. It SHALL NOT change discovery, model selection, the operator catalog or BYOK on the namespace routes.

#### Scenario: Catalog stored next to the operator catalog
- **WHEN** a namespace holds a valid catalog
- **THEN** its namespace routes SHALL list and invoke the operator catalog exactly as without it, and the catalog's routes SHALL list and invoke only the catalog

### Requirement: Catalog management documentation
Operator documentation SHALL explain managing catalogs from the app, which roles read, write and call catalogs, and when changes take effect: catalog edits apply on the next call, while removed roles and revoked tokens can take longer because of platform caches. The catalog package guide SHALL distinguish stack catalogs from the Go catalog package.

#### Scenario: Operator reads how changes take effect
- **WHEN** an operator reads the stored catalog documentation
- **THEN** they SHALL find the catalog page, the three roles, and that catalog edits apply on the next call while role and token removals can take longer

#### Scenario: Developer reads the catalog package guide
- **WHEN** a developer reads the catalog package guide
- **THEN** it SHALL say that stack catalogs are stored `ModelCatalog` resources the Gateway serves, and link to their documentation

### Requirement: Optional provider serving settings
A `ModelCatalog` provider SHALL accept optional `serveAllModels` (boolean) and `modelBlocklist` (array of backend model ID strings). An omitted or false switch SHALL mean listed models only, and an omitted or empty blocklist SHALL block no IDs. A blocklist SHALL be accepted and retained while the switch is off. The operator catalog schema SHALL remain unchanged.

#### Scenario: Existing catalog
- **WHEN** an existing catalog omits both fields
- **THEN** it SHALL remain valid and its providers SHALL serve listed models only

#### Scenario: Prepare a blocklist before enabling
- **WHEN** a writer saves valid blocked IDs with `serveAllModels` off
- **THEN** the write SHALL be accepted and a read SHALL return those IDs without enabling unlisted models

### Requirement: Validate provider blocklists
Each provider's `modelBlocklist` SHALL contain at most 128 distinct, exact, case-sensitive backend IDs. An ID SHALL be valid UTF-8, 1 to 2,048 bytes, with no control characters or leading or trailing whitespace. Admission SHALL reject invalid fields and a blocklist entry that equals a listed primary or fallback target's backend ID for that provider instance, regardless of the switch. Refusals SHALL identify the field without echoing the blocked ID or keys.

#### Scenario: Wrong field type
- **WHEN** a write supplies either setting with the wrong type
- **THEN** the catalog API SHALL refuse the write

#### Scenario: Invalid blocklist entries
- **WHEN** a blocklist has too many entries, a duplicate ID, an empty or whitespace-padded ID, a control character, or an ID exceeding the byte bound
- **THEN** admission SHALL refuse the write with `400`

#### Scenario: Conflicting primary or fallback
- **WHEN** a write would leave a blocked ID as a listed model's primary or fallback for the same provider instance
- **THEN** the write SHALL be refused with `400`, even if `serveAllModels` is off

#### Scenario: Same backend ID on another instance
- **WHEN** a listed target uses an ID blocked only by a different instance, even of the same provider type
- **THEN** the blocklist conflict check SHALL accept the write
