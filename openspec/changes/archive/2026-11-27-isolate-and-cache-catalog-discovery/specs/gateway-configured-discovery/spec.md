## MODIFIED Requirements

### Requirement: Catalog provider discovery is bounded and best effort
Discovery SHALL list only opted-in providers using the checked catalog's keys and the bounded provider listing transport. It SHALL admit at most one discovery per catalog and four per process, independently of health and provider inventory actions. It SHALL probe at most eight providers within two seconds and retain configured rows within the existing 1,024-ID budget including aliases. Inventory SHALL NOT be required for invocation.

#### Scenario: Listing is unavailable or incomplete
- **WHEN** a provider fails, returns a partial listing, or a discovery budget is exhausted
- **THEN** discovery SHALL still return all configured rows and any eligible IDs already collected within the budget, without upstream error details

#### Scenario: Inventory echoes a key
- **WHEN** a provider lists an ID that contains a saved key or its header form
- **THEN** discovery SHALL NOT publish the ID or follow a continuation cursor containing that key

#### Scenario: Discovery is busy
- **WHEN** another discovery for the catalog is active or the process-wide discovery slots are occupied
- **THEN** discovery SHALL return configured rows only
- **AND** discovery SHALL NOT occupy a health or provider inventory action slot

## ADDED Requirements

### Requirement: Short catalog discovery inventory cache
Discovery SHALL cache complete and partial provider inventories for 30 seconds in process, keyed by namespace, catalog name, resource version and provider instance. It SHALL NOT cache a listing if any requested page fails or if the request has no verified revision. The cache SHALL retain at most 128 entries and evict expired entries before evicting the oldest. Health and provider inventory actions SHALL remain live and uncached.

#### Scenario: Repeated discovery
- **WHEN** the same catalog revision is discovered before the provider inventory expires
- **THEN** discovery SHALL reuse that inventory without contacting that provider again

#### Scenario: Catalog changes or inventory expires
- **WHEN** discovery sees a new resource version or an expired entry
- **THEN** discovery SHALL obtain inventory on demand using the current catalog, including its current key
