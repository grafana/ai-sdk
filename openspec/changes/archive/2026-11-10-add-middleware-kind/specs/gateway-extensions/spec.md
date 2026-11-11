## ADDED Requirements

### Requirement: Tenant configuration resources
The Gateway's API group SHALL serve a namespaced `Middleware` kind in version `v1alpha1` that is not folder-scoped and declares no secure values. Its `spec` SHALL hold the extension `type`, a required `enabled` flag and an optional `config` object whose fields the kind schema does not restrict, so admission, not schema pruning, decides what an extension accepts. A resource's name SHALL equal its `spec.type`, so a namespace holds at most one resource per extension.

#### Scenario: Resource saved without a folder
- **WHEN** a writer creates a valid `Middleware` without a folder annotation
- **THEN** it SHALL be stored in the request namespace

#### Scenario: Name differs from type
- **WHEN** a writer creates or updates a `Middleware` whose name differs from `spec.type`
- **THEN** the write SHALL be refused

#### Scenario: Top-level configuration kept as written
- **WHEN** a writer stores a valid `Middleware` whose `config` holds scalar fields and arrays of scalars
- **THEN** reads SHALL return that `config` unchanged

### Requirement: Tenant configuration shape
A tenant extension's configuration SHALL be limited to top-level fields whose values are scalars or arrays of scalars, because the platform prunes objects nested inside a stored `config`. Tenant extension schemas SHALL NOT rely on nested objects, and the Gateway SHALL treat what the platform stores, as admission saw it, as the configuration.

#### Scenario: Flat configuration
- **WHEN** a tenant extension's schema declares top-level string, number, boolean and string-array fields
- **THEN** a valid configuration SHALL be stored and read back with every field

#### Scenario: Nested object in a configuration
- **WHEN** a writer stores a `config` holding an object nested inside it
- **THEN** the Gateway SHALL NOT promise that the nested object's fields are stored, and admission SHALL validate the configuration the platform passes it

### Requirement: Tenant configuration admission
Admission SHALL accept a create or update only when `spec.type` names a tenant-scope extension the distribution registered and `spec.config`, or an empty object when it is absent, matches that extension's configuration schema. An extension without a schema SHALL accept only an empty configuration. Operator-scope and unregistered types SHALL be refused with the same reason, so refusals do not reveal which operator extensions a distribution registers.

#### Scenario: Valid configuration
- **WHEN** a writer creates a `Middleware` for a registered tenant extension with a configuration its schema accepts
- **THEN** the write SHALL be accepted, whether `enabled` is `true` or `false`

#### Scenario: Configuration that fails the schema
- **WHEN** `spec.config` fails the extension's schema
- **THEN** the write SHALL be refused naming `spec.config` and the failing location

#### Scenario: Operator extension type
- **WHEN** `spec.type` names a registered operator-scope extension
- **THEN** the write SHALL be refused with the reason given for an unregistered type

#### Scenario: Shipped build
- **WHEN** the shipped Gateway, which registers no tenant extension, reviews a `Middleware` create
- **THEN** it SHALL refuse it

### Requirement: Disable-only updates
An update that sets `spec.enabled` to `false` and keeps the stored `spec.config` SHALL be accepted whatever the stored type, including a type the running build does not register as a tenant extension and a configuration that no longer matches its extension's schema. Any other update of a resource whose type is not a registered tenant extension SHALL be refused.

#### Scenario: Disabling after a rollback
- **WHEN** a resource was stored by a build that registers its type, and a build that does not register it reviews an update that only sets `enabled: false`
- **THEN** the update SHALL be accepted

#### Scenario: Enabling or reconfiguring an unknown type
- **WHEN** a build that does not register the stored type reviews an update that keeps `enabled: true` or changes `config`
- **THEN** the update SHALL be refused

#### Scenario: Disabling a configuration the schema no longer accepts
- **WHEN** a registered tenant extension's schema no longer accepts the stored configuration and an update only sets `enabled: false`
- **THEN** the update SHALL be accepted

### Requirement: Tenant configuration size bound
Admission SHALL refuse a `spec.config` whose JSON encoding exceeds 64 KiB.

#### Scenario: Oversized configuration
- **WHEN** a create or update carries a `config` larger than the bound
- **THEN** it SHALL be refused naming `spec.config` and the bound, without its content

### Requirement: Secret-safe tenant configuration refusals
Admission refusals SHALL be `400 Bad Request` with a reason naming the failing field path and, for schema failures, the failing location within `config`. A reason MAY name the type. Neither a reason nor any log or metric the plugin produces SHALL contain a configuration value or the schema validator's own message.

#### Scenario: Refused write carrying sentinel values
- **WHEN** refused creates and updates carry sentinel strings as configuration values and as undeclared configuration keys
- **THEN** no response body and no plugin log SHALL contain them

### Requirement: Tenant configuration admission scope
Admission SHALL review `CREATE` and `UPDATE` of `Middleware` only. Deletes SHALL NOT be reviewed, so a resource can always be deleted, including while the plugin is unavailable or no longer registers its type. Creates and updates SHALL fail closed while the plugin is unavailable.

#### Scenario: Delete
- **WHEN** a writer deletes a `Middleware`
- **THEN** the plugin SHALL receive no admission request, and the delete SHALL succeed even when the plugin would refuse every review

### Requirement: Tenant configuration roles
The manifest SHALL declare `aigateway:middlewarereader`, granting the viewer permission set on `Middleware` and bound to Viewer, and `aigateway:middlewarewriter`, granting the editor permission set on `Middleware` and bound to Admin. They SHALL share the manifest's roles map and bindings with the catalog roles, which SHALL stay unchanged.

#### Scenario: Viewer and Editor
- **WHEN** a Viewer or Editor gets or lists `Middleware` resources in their namespace
- **THEN** the request SHALL be allowed, and their create, update and delete requests SHALL be refused by authorization

#### Scenario: Admin
- **WHEN** an Admin creates, updates or deletes a `Middleware`
- **THEN** authorization SHALL allow the request, subject to admission

#### Scenario: Catalog roles unchanged
- **WHEN** the committed manifest is inspected
- **THEN** the catalog roles SHALL grant exactly what they granted before, and grant nothing on `Middleware`

### Requirement: Stored tenant configuration has no runtime effect
A stored `Middleware` SHALL NOT change which extensions run, or with which configuration, on any route. Tenant extensions SHALL keep running as "Tenant extensions without namespace configuration" requires.

#### Scenario: Stored configuration next to a running extension
- **WHEN** a namespace stores `enabled: true` for a tenant extension that is off by default, and `enabled: false` for one that is on by default
- **THEN** the first SHALL not run and the second SHALL run, as without the resources

### Requirement: Tenant configuration documentation
The extension guide SHALL describe the `Middleware` resource, its roles, the admission rules, that creates and updates fail while the plugin is unavailable while deletes always work, that stored resources are not applied yet, and which `config` shapes the platform is known to preserve.

#### Scenario: Extension author reads the guide
- **WHEN** an extension author reads the guide's tenant configuration section
- **THEN** they SHALL find that top-level fields and arrays of scalars are preserved, and that the platform may empty objects nested inside `config`

### Requirement: Tenant configuration evidence
Roles, type and name refusals and secret-safe refusals SHALL be proven by end-to-end tests against a running Grafana with the built plugin. While the shipped Gateway registers no tenant extension, acceptance, schema refusals, operator-type refusals, disable-only updates and unreviewed deletes SHALL be proven through the protocol test host with test-only definitions passed through the composition options, and no test-only definition SHALL be compiled into the shipped backend.

#### Scenario: End-to-end suite with the shipped build
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise the roles, the refusals and the secret-safe responses and plugin logs through Grafana's API

#### Scenario: Shipped backend
- **WHEN** the plugin package is built
- **THEN** its backend SHALL register only the shipped definitions
