# Spec Delta

## ADDED Requirements

### Requirement: Namespace tenant configuration
Each call's tenant-scope extensions SHALL follow the `Middleware` resources of the call's namespace. With no resource for an extension, it SHALL use its default state: on with an empty configuration when enabled by default, off otherwise. A resource with `enabled: false` SHALL turn it off. A resource with `enabled: true` SHALL run it with that resource's configuration. One namespace's resources SHALL NOT affect another namespace's calls.

#### Scenario: Two namespaces with different configurations
- **WHEN** namespace `a` stores `enabled: true` with one configuration for a default-off tenant extension, namespace `b` stores another configuration for it, and namespace `c` stores nothing
- **THEN** calls in `a` and `b` SHALL each run the extension with their own configuration, and calls in `c` SHALL NOT run it

#### Scenario: Disabling a default-on extension
- **WHEN** a namespace stores `enabled: false` for a tenant extension that is enabled by default
- **THEN** none of its phases SHALL run for that namespace's calls, and other namespaces SHALL keep running it

#### Scenario: Resource removed
- **WHEN** a namespace deletes its resource for an extension
- **THEN** after the change propagates, its calls SHALL use the extension's default state

### Requirement: Tenant configuration checked when read
The Gateway SHALL check each enabled resource's configuration against its extension's schema when the resource is read, as admission does, and SHALL pass the factory the stored JSON object with its numbers unchanged. A configuration that fails the check, or a factory that fails or panics, SHALL apply the extension's failure policy in that namespace. It SHALL be logged and counted without configuration values, and SHALL NOT fail startup or affect other namespaces.

#### Scenario: Schema tightened after storage
- **WHEN** a stored enabled configuration no longer matches its `skip` extension's schema
- **THEN** that extension SHALL NOT run for the namespace's calls, the failure SHALL be counted, and calls SHALL continue

#### Scenario: Fail-closed extension with invalid configuration
- **WHEN** a `fail-closed` tenant extension's stored configuration fails its schema
- **THEN** calls in that namespace to which it applies SHALL fail with the unavailable category before any provider call

#### Scenario: Large integer
- **WHEN** a stored configuration holds the integer `9007199254740993`
- **THEN** the factory SHALL receive that exact number

#### Scenario: Factory fails for one namespace
- **WHEN** a tenant extension's factory fails for one namespace's configuration
- **THEN** that namespace SHALL apply the failure policy, and another namespace with a valid configuration SHALL run the extension

### Requirement: Unknown tenant types at read time
An enabled resource whose type is not a tenant-scope extension of the running build SHALL be skipped. It SHALL be logged once per namespace and type while the namespace stays cached, and counted. A disabled resource of an unknown type SHALL be ignored without a log or count, since it has no effect.

#### Scenario: Rollback leaves an enabled resource
- **WHEN** a namespace stores an enabled resource whose extension the running build doesn't register, and three calls run
- **THEN** all three calls SHALL run without it, and the Gateway SHALL emit one log record and one count for that namespace and type

#### Scenario: Disabled unknown resource
- **WHEN** the unknown type's resource is disabled
- **THEN** the Gateway SHALL neither log nor count it

### Requirement: Tenant configuration read failures
A failed read SHALL NOT be treated as a missing resource. A failure is any error, timeout, refusal, unexpected status or response the Gateway can't decode, including a refused or not-found list. With no configuration loaded for the namespace, each tenant extension SHALL apply its failure policy. A failed read SHALL never apply a more permissive default.

#### Scenario: First read fails
- **WHEN** the first read for a namespace fails, and that namespace has a `skip` tenant extension that is enabled by default and a `fail-closed` one
- **THEN** the `skip` extension SHALL NOT run, calls to which the `fail-closed` one applies SHALL fail with the unavailable category, and neither SHALL use its default state

#### Scenario: Listing refused
- **WHEN** the platform answers a namespace's list with `403` or `404`
- **THEN** the Gateway SHALL treat it as a failed read, not as a namespace without resources

### Requirement: Last-known-good tenant configuration
When a refresh fails, the Gateway SHALL keep serving the namespace's last successfully read configuration for at most 10 minutes after that read. After that bound, each tenant extension in the namespace SHALL apply its failure policy until a read succeeds.

#### Scenario: Refresh fails within the bound
- **WHEN** a namespace's configuration was read successfully and refreshes fail for the next 9 minutes
- **THEN** its calls SHALL keep the last-known-good configuration, including any extension it disables

#### Scenario: Bound exceeded
- **WHEN** refreshes keep failing for more than 10 minutes after the last successful read
- **THEN** its calls SHALL apply each tenant extension's failure policy, and the next successful read SHALL restore the stored configuration

### Requirement: Tenant configuration reads as the plugin
The Gateway SHALL read `Middleware` resources with the plugin's own identity, on a context that carries no caller identity or cancellation, with a read timeout of its own. A caller's permissions, identity, deadline or cancellation SHALL NOT affect a read or its result.

#### Scenario: Caller without read access
- **WHEN** a caller that can't read `Middleware` resources makes the first call in a namespace
- **THEN** the read SHALL be made as the plugin and its result applied to the call

#### Scenario: Caller cancels during the first read
- **WHEN** the first caller in a namespace cancels while the read is in flight
- **THEN** the read SHALL complete within its own timeout, and the next call SHALL use its result

### Requirement: Tenant configuration cache
The Gateway SHALL cache each namespace's configuration, including the absence of resources, for a TTL of 1 minute. A call SHALL wait only for a namespace's first read, and concurrent calls SHALL share one read. After the TTL, calls SHALL use the cached configuration while one background refresh runs. A failed read SHALL be retried no earlier than a fixed backoff. The cache SHALL hold a bounded number of namespaces, evicting the least recently used.

#### Scenario: Concurrent first calls
- **WHEN** ten calls arrive for an uncached namespace at the same time
- **THEN** the Gateway SHALL make one read, and every call SHALL use its result

#### Scenario: Change applies within the TTL
- **WHEN** a resource changes and a call arrives more than one TTL after the namespace was last read
- **THEN** that call SHALL trigger a refresh, and calls after the refresh completes SHALL use the new configuration

#### Scenario: Failure backs off
- **WHEN** a refresh fails and further calls arrive before the backoff ends
- **THEN** no further read SHALL be made until the backoff ends

#### Scenario: Bound reached
- **WHEN** a new namespace is read while the cache holds its maximum number of namespaces
- **THEN** the least recently used namespace SHALL be evicted, and its next call SHALL read again

### Requirement: Independent replica convergence
Each Gateway process SHALL cache independently. A change SHALL take effect on each process within one TTL of that process's next call in the namespace, plus that read's duration. Processes MAY disagree inside that window.

#### Scenario: Two processes
- **WHEN** two Gateway processes serve one namespace and its resource changes
- **THEN** each SHALL apply the change after its own refresh, without coordination between them

### Requirement: Tenant configuration reads only when needed
The Gateway SHALL read a namespace's configuration only for a call to which at least one tenant-scope extension applies. With no tenant-scope extension registered, it SHALL make no reads.

#### Scenario: Shipped build
- **WHEN** the shipped backend, which registers no tenant extension, serves calls
- **THEN** it SHALL make no `Middleware` reads

#### Scenario: Extension narrowed to another source
- **WHEN** the only tenant extension declares ownership `operator`, and a request BYOK call runs
- **THEN** that call SHALL NOT read the namespace's configuration

### Requirement: Per-namespace extension instances
A tenant extension enabled by a namespace's resource SHALL get its own instance, built once for each configuration revision of that namespace and reused while the configuration stays unchanged. An instance SHALL be closed after its namespace's configuration is replaced, evicted or expires, and only once no call or observation still uses it. Defaults SHALL use the instance built at startup.

#### Scenario: Unchanged refresh
- **WHEN** a refresh reads the same configuration
- **THEN** the namespace SHALL keep its instances, and no factory SHALL run

#### Scenario: Replaced during a stream
- **WHEN** a namespace's configuration changes while a stream that uses its instance is open
- **THEN** the stream SHALL keep that instance until it ends and its observation completes, then the instance SHALL be closed once

#### Scenario: Eviction
- **WHEN** a namespace with configured instances is evicted
- **THEN** its instances SHALL be closed after their in-flight calls and observations end

### Requirement: Extension metrics shared across instances
Every instance of one definition SHALL receive the same metrics registerer, with the extension's name as a constant label. Registering a collector that the same definition already registered SHALL succeed, so a definition that creates its collectors once can register them in every factory run. A different collector with the same descriptors SHALL still fail registration.

#### Scenario: Two namespaces register one counter
- **WHEN** a definition creates one counter and its factory registers it for two namespaces
- **THEN** both factory runs SHALL succeed, and both instances SHALL increment the same series

#### Scenario: Conflicting collector
- **WHEN** a factory creates a new collector with the same descriptors on each run
- **THEN** its second run SHALL fail registration, and that namespace SHALL apply the extension's failure policy

### Requirement: Tenant configuration without API access
When the host gives the plugin no API access, the Gateway SHALL make no reads, SHALL run every tenant-scope extension in its default state in every namespace, and SHALL log this once at startup when the build registers a tenant-scope extension.

#### Scenario: Self-hosted without API access
- **WHEN** a self-hosted host gives the plugin no API access, and the build registers a default-on and a default-off tenant extension
- **THEN** the first SHALL run and the second SHALL NOT in every namespace, and one log record SHALL state that tenant configuration is not applied

### Requirement: Tenant configuration writes refused without API access
When the plugin has no API access, admission SHALL refuse every `Middleware` create and update with `400`, before any other admission check, with a fixed reason stating that stored configuration would not be applied. Deletes SHALL stay unreviewed.

#### Scenario: Create without API access
- **WHEN** an Admin creates a valid `Middleware` on a Gateway without API access
- **THEN** the write SHALL be refused with `400` and the fixed reason

#### Scenario: Delete without API access
- **WHEN** an Admin deletes a stored `Middleware` on a Gateway without API access
- **THEN** the plugin SHALL receive no admission request, and the delete SHALL succeed

### Requirement: Operator setting excludes tenant extensions
The operator extension setting SHALL NOT configure tenant-scope extensions.

#### Scenario: Tenant extension in the operator setting
- **WHEN** the operator setting names a tenant-scope extension
- **THEN** startup SHALL fail naming it

## MODIFIED Requirements

### Requirement: Extension instance lifecycle
Operator-scope extensions enabled by the operator setting, and tenant-scope extensions enabled by default, SHALL be constructed at startup, before requests are served, with a logger scoped to the extension, the definition's shared registerer, a clock and resolved secrets. A startup factory error SHALL fail startup. Per-namespace tenant instances follow "Per-namespace extension instances". Instances SHALL be shared by concurrent calls, and one call's phases SHALL never run concurrently.

#### Scenario: Factory contract allows more than one run
- **WHEN** a developer reads the factory documentation
- **THEN** it SHALL state that a factory may run more than once per process, once per configuration it is given, and that collectors are registered through the shared registerer

#### Scenario: Factory fails
- **WHEN** an extension's factory returns an error or panics at startup
- **THEN** startup SHALL fail naming the extension, without the error's text

#### Scenario: Concurrent calls
- **WHEN** many calls run concurrently through one extension
- **THEN** each call's phases SHALL run sequentially and see only that call's state

### Requirement: Disable-only updates
On a Gateway with API access, an update that sets `spec.enabled` to `false` and keeps the stored `spec.config` SHALL be accepted whatever the stored type, including a type the running build does not register as a tenant extension and a configuration that no longer matches its extension's schema. Any other update of a resource whose type is not a registered tenant extension SHALL be refused.

#### Scenario: Disabling after a rollback
- **WHEN** a resource was stored by a build that registers its type, and a build that does not register it reviews an update that only sets `enabled: false`
- **THEN** the update SHALL be accepted

#### Scenario: Enabling or reconfiguring an unknown type
- **WHEN** a build that does not register the stored type reviews an update that keeps `enabled: true` or changes `config`
- **THEN** the update SHALL be refused

#### Scenario: Disabling a configuration the schema no longer accepts
- **WHEN** a registered tenant extension's schema no longer accepts the stored configuration and an update only sets `enabled: false`
- **THEN** the update SHALL be accepted

### Requirement: Tenant configuration admission
On a Gateway with API access, admission SHALL accept a create or update only when `spec.type` names a registered tenant-scope extension and `spec.config`, or an empty object when absent, matches its configuration schema. An extension without a schema SHALL accept only an empty configuration. Operator-scope and unregistered types SHALL be refused with the same reason, so refusals do not reveal which operator extensions a distribution registers.

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

### Requirement: Tenant configuration documentation
The extension guide SHALL describe the `Middleware` resource, its roles and admission rules, how stored configuration is applied and propagates within the TTL on each replica, read-failure behavior and its bound, self-hosted behavior without API access, which `config` shapes the platform preserves, and that tenant extensions see the caller UID and the provider instances and models of operator-catalog candidates and attempts.

#### Scenario: Extension author reads the guide
- **WHEN** an extension author reads the guide's tenant configuration section
- **THEN** they SHALL find that top-level fields and arrays of scalars are preserved, that the platform may empty objects nested inside `config`, and that a factory may run once per namespace configuration

#### Scenario: Org admin reads the guide
- **WHEN** an org admin reads how a change takes effect
- **THEN** they SHALL find the TTL, that replicas converge independently, and that a failed read never falls back to a more permissive default

### Requirement: Tenant configuration evidence
Roles, refusals and secret-safe refusals SHALL be proven end to end against a running Grafana with the built plugin. Until a shipped tenant extension exists, acceptance, schema refusals, disable-only updates, unreviewed deletes and namespace configuration reads SHALL be proven through the protocol test host with test-only definitions passed through the composition options. No test-only definition SHALL be compiled into the shipped backend.

#### Scenario: End-to-end suite with the shipped build
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise the roles, the refusals and the secret-safe responses and plugin logs through Grafana's API

#### Scenario: Namespace reads through the test host
- **WHEN** Gateway tests store different resources in two namespaces of the protocol test host and call the language-model route in each
- **THEN** each call SHALL apply its namespace's configuration, read over the platform API with the plugin's own token

#### Scenario: Shipped backend
- **WHEN** the plugin package is built
- **THEN** its backend SHALL register only the shipped definitions

## REMOVED Requirements

### Requirement: Tenant extensions without namespace configuration
**Reason**: Tenant extensions now follow each namespace's `Middleware` resources. The default state applies only without a resource, or without API access.
**Migration**: See "Namespace tenant configuration" and "Tenant configuration without API access". The operator-setting rule moves to "Operator setting excludes tenant extensions".

### Requirement: Stored tenant configuration has no runtime effect
**Reason**: Stored resources now take effect per namespace.
**Migration**: See "Namespace tenant configuration".
