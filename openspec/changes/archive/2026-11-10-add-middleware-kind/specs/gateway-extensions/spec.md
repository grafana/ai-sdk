## ADDED Requirements

### Requirement: Tenant configuration resources
The Gateway's API group SHALL serve a namespaced `Middleware` kind in version `v1alpha1` that is not folder-scoped and declares no secure values. Its `spec` SHALL hold the extension `type`, a required `enabled` flag and, as optional fields, one typed member per public tenant type with configuration, named the lowerCamel form of the type. A resource's name SHALL equal its `spec.type`, so a namespace holds at most one resource per extension.

#### Scenario: Resource saved without a folder
- **WHEN** a writer creates a valid `Middleware` without a folder annotation
- **THEN** it SHALL be stored in the request namespace

#### Scenario: Name differs from type
- **WHEN** a writer creates or updates a `Middleware` whose name differs from `spec.type`
- **THEN** the write SHALL be refused

#### Scenario: Nested configuration kept as written
- **WHEN** a writer stores a valid `Middleware` whose member holds nested objects and arrays of objects
- **THEN** reads SHALL return that member unchanged

### Requirement: Public tenant extension types
Tenant extension types SHALL be declared in a public, experimental package: each with a name, a description, an optional configuration schema, a default state and an example configuration that its schema accepts. The package SHALL bind an implementation to a type to produce a tenant-scope definition. The registry the shipped Gateway uses SHALL be the only source of members in the manifest, and SHALL be empty in this version.

#### Scenario: Private implementation of a public type
- **WHEN** a distribution binds its own implementation to a public type and passes the definition to the entrypoint
- **THEN** the Gateway SHALL start and admit resources of that type, with the manifest unchanged

#### Scenario: Invalid type
- **WHEN** a type has an invalid or repeated name, a member that collides with another type's or with `type` or `enabled`, a schema without an example it accepts, or an example without a schema
- **THEN** the repository checks SHALL fail naming the type

### Requirement: Additive tenant type evolution
Because the platform validates every write against the current schema without exempting unchanged fields, a public type's schema SHALL change only additively: new optional fields and nothing narrower. An incompatible change SHALL use a new type name.

#### Scenario: Required field added to a type
- **WHEN** a change to a public type would add a required field or narrow a field's type
- **THEN** it SHALL be made as a new type instead

### Requirement: Tenant definitions bind to public types
Startup SHALL fail, before any setting is read and also on hosts that start idle, when a tenant-scope definition's name is not a public type's, or when its configuration schema or default differs from that type's. Private extensions SHALL be operator-scope unless they implement a public tenant type.

#### Scenario: Tenant definition without a public type
- **WHEN** a distribution passes a tenant-scope definition whose name no public type has
- **THEN** startup SHALL fail naming the extension

#### Scenario: Definition differs from its type
- **WHEN** a tenant-scope definition has a public type's name but another schema or default
- **THEN** startup SHALL fail naming the extension and the differing field

### Requirement: Tenant configuration member projection
Each manifest member SHALL be projected from its type's schema of closed objects and arrays, keeping JSON types, properties, items, required fields and descriptions, and dropping value constraints and defaults. The manifest SHALL have no enum on `spec.type`, no union across members and no required member. A type whose projection is not structural, or whose example pruning changes, SHALL be refused. A drift check SHALL guarantee the committed manifest equals the projection.

#### Scenario: Value constraints stay in admission
- **WHEN** a type's schema has an enum, a pattern or a bound on a field
- **THEN** the manifest member SHALL declare only the field's JSON type, and admission SHALL check the constraint

#### Scenario: Schema the platform would prune
- **WHEN** a type's schema has an open object, an empty object, an object without an explicit `additionalProperties: false`, a typed map, a `$ref`, an untyped node or a combinator
- **THEN** the type SHALL be refused before it reaches the manifest

#### Scenario: Integer field bounds
- **WHEN** a type's schema declares a `minimum` or `maximum` on an integer field outside ±(2^53−1)
- **THEN** the type SHALL be refused before it reaches the manifest

#### Scenario: Drift
- **WHEN** the registry changes without regenerating the manifest
- **THEN** the repository checks SHALL fail

### Requirement: Tenant configuration admission
Admission SHALL accept a create or update only when the spec sets no member other than its type's, `spec.type` names a tenant-scope extension the distribution implements, a type with configuration has its member when `enabled` is `true`, a type without configuration has no member, and a present member matches that type's full schema. Operator-scope, unregistered and unimplemented types SHALL be refused with the same reason.

#### Scenario: Valid configuration
- **WHEN** a writer creates a `Middleware` for an implemented tenant type with a member its schema accepts
- **THEN** the write SHALL be accepted, whether `enabled` is `true` or `false`

#### Scenario: Member of another type
- **WHEN** the spec sets a member other than its type's, or more than one member
- **THEN** the write SHALL be refused naming only the type's member

#### Scenario: Enabled without its member
- **WHEN** an enabled resource of a type with configuration has no member
- **THEN** the write SHALL be refused

#### Scenario: Disabled without a member
- **WHEN** a writer creates a disabled resource of an implemented type without its member
- **THEN** the write SHALL be accepted

#### Scenario: Configuration that fails the schema
- **WHEN** the member fails the type's schema, including a constraint the manifest does not declare
- **THEN** the write SHALL be refused naming the member and the failing location

#### Scenario: Operator or unimplemented type
- **WHEN** `spec.type` names an operator-scope extension, or a public type this build does not implement
- **THEN** the write SHALL be refused with the reason given for an unregistered type

#### Scenario: Safe integer
- **WHEN** a member holds an integer within ±(2^53−1)
- **THEN** admission SHALL compare it exactly against the type's schema, and MAY round integers beyond that range as upstream does

#### Scenario: Shipped build
- **WHEN** the shipped Gateway, which implements no tenant extension, reviews a `Middleware` create
- **THEN** it SHALL refuse it

### Requirement: Disable-only updates
An update that sets `spec.enabled` to `false` and keeps the stored member, or removes it, SHALL be accepted whatever the stored type, including a type the running build does not implement, a member that no longer matches its schema or bound, and a member the running manifest no longer declares. Any other update of a resource whose type is not implemented SHALL be refused.

#### Scenario: Disabling after a rollback
- **WHEN** a resource was stored by a build that implements its type, and a build that does not reviews an update that only sets `enabled: false`
- **THEN** the update SHALL be accepted

#### Scenario: Disabling with the member removed
- **WHEN** an update sets `enabled: false` and omits the stored member
- **THEN** the update SHALL be accepted

#### Scenario: Enabling or reconfiguring an unknown type
- **WHEN** a build that does not implement the stored type reviews an update that keeps `enabled: true` or changes the member
- **THEN** the update SHALL be refused

#### Scenario: Disabling a configuration the schema no longer accepts
- **WHEN** an implemented type's schema no longer accepts the stored member and an update only sets `enabled: false`
- **THEN** the update SHALL be accepted

### Requirement: Tenant configuration size bound
Admission SHALL refuse a member whose JSON encoding exceeds 64 KiB.

#### Scenario: Oversized configuration
- **WHEN** a create or update carries a member larger than the bound
- **THEN** it SHALL be refused naming the member and the bound, without its content

### Requirement: Secret-safe tenant configuration refusals
Admission refusals SHALL be `400 Bad Request` with a reason naming the failing field path and, for schema failures, the failing location within the member. A reason MAY name the type and its member. Neither a reason nor any log or metric the plugin produces SHALL contain a configuration value, an undeclared key or the schema validator's own message.

#### Scenario: Refused write carrying sentinel values
- **WHEN** refused creates and updates carry sentinel strings as member values, as undeclared member keys and as undeclared spec members
- **THEN** no response body and no plugin log SHALL contain them

### Requirement: Tenant configuration admission scope
Admission SHALL review `CREATE` and `UPDATE` of `Middleware` only. Deletes SHALL NOT be reviewed, so a resource can always be deleted, including while the plugin is unavailable or no longer implements its type. Creates and updates SHALL fail closed while the plugin is unavailable.

#### Scenario: Delete
- **WHEN** a writer deletes a `Middleware`
- **THEN** the plugin SHALL receive no admission request, and the delete SHALL succeed even when the plugin would refuse every review

### Requirement: Tenant configuration roles
The manifest SHALL declare `aigateway:middlewarereader`, granting the viewer permission set on `Middleware` and bound to Viewer, and `aigateway:middlewarewriter`, granting the editor permission set on `Middleware` and bound to Admin. They SHALL share the manifest's roles map and bindings with the catalog roles, which SHALL stay unchanged.

#### Scenario: Viewer and Editor
- **WHEN** a Viewer or Editor gets or lists `Middleware` resources in their namespace
- **THEN** the request SHALL be allowed, and their create, update and delete requests SHALL be refused

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
The extension guide SHALL describe the `Middleware` resource and its members, its roles, the admission rules, how to disable after a rollback, that creates and updates fail while the plugin is unavailable while deletes always work, that stored resources are not applied yet, how a public tenant type is added and projected, and that private extensions are operator-scope unless they implement a public type.

#### Scenario: Extension author reads the guide
- **WHEN** an extension author reads the guide's tenant configuration section
- **THEN** they SHALL find that a tenant type's name, schema and default are public, that its implementation may be private, and that type schemas change only additively

### Requirement: Tenant configuration evidence
Roles, type and name refusals and secret-safe refusals SHALL be proven end to end against Grafana with the built plugin. While the shipped registry is empty, acceptance, member rules, schema and operator-type refusals, disable-only updates and unreviewed deletes SHALL be proven through the protocol test host with test-only types and definitions passed through internal options, never compiled into the shipped backend.

#### Scenario: End-to-end suite with the shipped build
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise the roles, the refusals and the secret-safe responses and plugin logs through Grafana's API

#### Scenario: Shipped backend
- **WHEN** the plugin package is built
- **THEN** its backend SHALL register only the shipped types and definitions
