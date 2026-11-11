## MODIFIED Requirements

### Requirement: Explicit registration
Extensions SHALL run only when the distribution passes their definitions to the public entrypoint's options. There SHALL be no global or import-time registry. A function SHALL return the definitions the shipped Gateway registers, which are the `default-instructions` and `default-settings` tenant extensions, and the shipped backend SHALL pass exactly those.

#### Scenario: Importing registers nothing
- **WHEN** a distribution imports the public extension and composition packages without passing definitions
- **THEN** no extension SHALL run

#### Scenario: Shipped build
- **WHEN** the shipped backend starts
- **THEN** it SHALL pass the shipped definitions, which are both off by default, and no mandatory extension, and every request in a namespace that stores no `Middleware` resource SHALL reach the model as without extensions

### Requirement: Public tenant extension types
Tenant extension types SHALL be declared in a public, experimental package: each with a name, a description, an optional configuration schema, a default state and an example configuration that its schema accepts. The package SHALL bind an implementation to a type to produce a tenant-scope definition. The registry the shipped Gateway uses SHALL be the only source of members in the manifest, and SHALL hold `default-instructions` and `default-settings`.

#### Scenario: Private implementation of a public type
- **WHEN** a distribution binds its own implementation to a public type and passes the definition to the entrypoint
- **THEN** the Gateway SHALL start and admit resources of that type, with the manifest unchanged

#### Scenario: Invalid type
- **WHEN** a type has an invalid or repeated name, a member that collides with another type's or with `type` or `enabled`, a schema without an example it accepts, or an example without a schema
- **THEN** startup SHALL fail naming the type and the field

#### Scenario: Shipped registry
- **WHEN** the committed manifest is inspected
- **THEN** the `Middleware` spec SHALL declare exactly `type`, `enabled`, `defaultInstructions` and `defaultSettings`

### Requirement: Tenant configuration admission
Admission SHALL accept a create or update only when the spec sets no member other than its type's, `spec.type` names a tenant-scope extension the distribution implements, a type with configuration has its member when `enabled` is `true`, a type without configuration has no member, and a present member matches that type's full schema. Operator-scope, unregistered and unimplemented types SHALL be refused with the same reason. Members SHALL be decoded as written, so numbers are not rounded.

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

#### Scenario: Large integer
- **WHEN** a member holds an integer above 2^53 and the write is accepted
- **THEN** admission and the stored resource SHALL keep its exact value

#### Scenario: Shipped build
- **WHEN** the shipped Gateway reviews `Middleware` creates for `default-instructions` and `default-settings` with valid members, and one for any other type
- **THEN** it SHALL accept the first two and refuse the other with the unregistered-type reason

### Requirement: Tenant configuration evidence
Acceptance, roles, refusals, secret-safe refusals, round-trips, updates, disable-only updates, deletes and runtime effect SHALL be proven end to end against Grafana with the built plugin and its shipped types. What those types can't show, such as operator types, rollbacks, other schemas and several namespaces, SHALL be proven through the protocol test host with test-only types and definitions, never compiled into the shipped backend.

#### Scenario: End-to-end suite with the shipped build
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise the roles, writes of the shipped types, the refusals, the secret-safe responses and plugin logs, and the runtime effect through Grafana's API

#### Scenario: Namespace reads through the test host
- **WHEN** Gateway tests store different resources in two namespaces of the protocol test host and call the language-model route in each
- **THEN** each call SHALL apply its namespace's configuration, read over the platform API with the plugin's own token

#### Scenario: Shipped backend
- **WHEN** the plugin package is built
- **THEN** its backend SHALL register only the shipped types and definitions

### Requirement: Tenant configuration reads only when needed
The Gateway SHALL read a namespace's configuration only for a call to which at least one tenant-scope extension applies. With no tenant-scope extension registered, it SHALL make no reads.

#### Scenario: No tenant extension registered
- **WHEN** a distribution that registers no tenant extension serves calls
- **THEN** it SHALL make no `Middleware` reads

#### Scenario: Shipped build
- **WHEN** the shipped backend serves calls in a namespace that stores no `Middleware` resource
- **THEN** it SHALL read that namespace's configuration at most once per cache TTL, and the calls SHALL reach the model unchanged

#### Scenario: Extension narrowed to another source
- **WHEN** the only tenant extension declares ownership `operator`, and a request BYOK call runs
- **THEN** that call SHALL NOT read the namespace's configuration
