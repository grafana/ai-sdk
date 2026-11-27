## MODIFIED Requirements

### Requirement: Explicit configured route projection
Configured-access-authorized discovery SHALL expose ordered canonical public rows with typed aliases, primary and ordered fallback facts, without generation, inferred candidates or substitution of native identity.

#### Scenario: Explicit configured route projection policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Configured-access-authorized `GET` on the plugin `config` route SHALL return one row per visible canonical model, in ascending canonical-ID order, retaining its public `id` and `specification.modelId`, specification version `v4`, provider `grafana` and existing name/description. Each command-configured row SHALL include `gateway: {aliases, primary: {providerInstance, provider, providerModelId}, fallbacks: [{providerInstance, provider, providerModelId}]}`. Alias and fallback order SHALL preserve configuration; empty aliases and fallbacks SHALL be `[]`. Aliases SHALL remain callable without appearing as separate rows. `providerModelId` SHALL identify the configured native invocation model, never a public selection ID or response identity. Ordinary catalog entries without candidate metadata SHALL omit the extension rather than infer it.

#### Scenario: Alias and mixed-provider candidates are inspected
- **WHEN** an authorized caller discovers an alias for a route with an Anthropic primary and an OpenAI fallback
- **THEN** one canonical row SHALL expose its explicit aliases, primary and ordered fallbacks with provider-instance/provider/providerModelId facts
- **AND** its public specification SHALL NOT be replaced with a native candidate or response identity

#### Scenario: Direct compatible route is inspected
- **WHEN** a configured compatible provider has an explicit providerName or uses its constructor default
- **THEN** its candidate SHALL expose the effective adapter/provider identifier, configured instance reference and exact configured invocation model ID
- **AND** discovery SHALL NOT derive these values from canonical middleware or provider responses

#### Scenario: No inference is needed
- **WHEN** discovery projects explicitly listed routes
- **THEN** their metadata SHALL come from configuration, without provider generation, streaming, inventory enumeration or credential checks to determine listed candidate facts

### Requirement: Catalog provider discovery is bounded and best effort
Discovery SHALL use the checked catalog's keys and bounded listing transport only for opted-in providers. It SHALL check the cache before admitting probe runs: at most one per catalog and four per process, separate from health and inventory actions. It SHALL probe at most eight providers within two seconds and preserve listed rows within the 1,024-ID budget including aliases. Invocation SHALL NOT depend on inventory.

#### Scenario: Listing is unavailable or incomplete
- **WHEN** a provider fails, returns a partial listing, or a discovery budget is exhausted
- **THEN** discovery SHALL still return all configured rows and any eligible IDs already collected within the budget, without upstream error details

#### Scenario: Inventory echoes a key
- **WHEN** a provider lists an ID that contains a saved key or its header form
- **THEN** discovery SHALL NOT publish the ID or follow a continuation cursor containing that key

#### Scenario: Discovery is busy
- **WHEN** another discovery for the catalog is active or the process-wide discovery slots are occupied
- **THEN** discovery SHALL return configured rows and any eligible cached rows
- **AND** discovery SHALL NOT occupy a health or provider inventory action slot

#### Scenario: Concurrent discovery can use warm inventory
- **WHEN** all eligible provider inventories are cached and another discovery occupies an admission slot
- **THEN** discovery SHALL return the eligible cached rows alongside configured rows without taking a slot or contacting a provider

## ADDED Requirements

### Requirement: Operator discovery of qualified selectors

After configured-account authorization, operator `/config` SHALL preserve listed rows and add eligible unshadowed selectors from opted-in providers' inventory within the discovery budget, without constructing models. Added rows SHALL use the qualified stack-catalog projection. Discovery SHALL NOT expose keys, secret references, blocklists, upstream errors or inventory IDs containing a key or its header form.

#### Scenario: Operator inventory contains eligible, blocked and shadowed IDs
- **WHEN** an authorized caller discovers an operator catalog with an opted-in provider that returns those IDs
- **THEN** discovery SHALL skip blocked IDs, IDs or full selectors that fail the shared public-ID rule, and selectors shadowed by a listed ID or alias
- **AND** added rows SHALL use the qualified selector as public and specification ID, and one primary with the same instance, slug and literal backend ID as invocation, without aliases or fallbacks

#### Scenario: Operator catalog is listed-only or caller lacks configured access
- **WHEN** no operator provider opts in or the caller lacks configured-account authorization
- **THEN** the request SHALL make no provider inventory requests

### Requirement: Bounded operator discovery inventory

Operator discovery SHALL reuse stack discovery's listing transport, bounds and short inventory cache rules, using startup-validated destinations and cache keys scoped to the current startup catalog and provider instance. It SHALL check the cache before admitting missing inventories, with at most one operator probe run at a time independent of stack discovery and admin actions. The response SHALL retain `Cache-Control: no-store`; invocation SHALL NOT depend on inventory.

#### Scenario: Repeated discovery
- **WHEN** an authorized caller discovers the same startup catalog within 30 seconds of a cached listing
- **THEN** discovery SHALL reuse that inventory without contacting the provider again
- **AND** the cache SHALL keep at most 128 entries and SHALL NOT store a listing if a requested page failed

#### Scenario: Concurrent discovery can use warm inventory
- **WHEN** all eligible provider inventories are cached and the operator probe slot is occupied
- **THEN** discovery SHALL return the eligible cached rows without taking a slot or contacting a provider

#### Scenario: Inventory is unavailable or discovery is busy
- **WHEN** provider listing fails, discovery hits a resource bound, or the operator discovery slot is occupied
- **THEN** discovery SHALL still return all listed rows and any eligible IDs collected within the two-second, eight-provider and 1,024-total-public-ID limits (including aliases), without upstream error details
