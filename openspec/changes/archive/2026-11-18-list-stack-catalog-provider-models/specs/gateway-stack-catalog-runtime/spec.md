# Stack Catalog Runtime Delta

## ADDED Requirements

### Requirement: List one saved provider's models
The `available-models` action SHALL accept exactly one `provider` query parameter naming a provider in the checked parent catalog and no body. It SHALL run the same stored-catalog preflight as selection and health before serving a listing, use only the named provider's saved key and validated destination, and never accept draft configuration or credentials. The action SHALL NOT run inference or modify the catalog.

#### Scenario: List one of two accounts of the same type
- **WHEN** a valid caller asks for one named provider in a saved catalog
- **THEN** the action SHALL use that provider's account only and SHALL NOT substitute another provider's key or ambient credentials

#### Scenario: Invalid input
- **WHEN** the action receives a body, a missing or invalid selector, duplicate selectors, or extra parameters
- **THEN** it SHALL return a fixed `400` with no provider request

#### Scenario: Unknown provider
- **WHEN** the selector is valid but absent from an otherwise valid catalog
- **THEN** the action SHALL return a fixed `404` with no provider request

#### Scenario: Invalid stored configuration
- **WHEN** the saved catalog fails shared shape, endpoint or key checks
- **THEN** the action SHALL return a fixed `424` without contacting the provider

### Requirement: Return bounded listing outcomes
An accepted listing request SHALL return `200` JSON with `status` and `modelIds`. Status SHALL be one of `ok`, `partial`, `invalidKey`, `unreachable`, `refused`, or `notChecked`. `modelIds` SHALL be a bounded, sorted array of exact, distinct IDs from valid pages, with no upstream metadata. `ok` and `partial` describe the listing only and SHALL NOT attest to inference. Other statuses SHALL return an empty array.

#### Scenario: Complete empty listing
- **WHEN** a provider returns a valid empty list and no further page is indicated
- **THEN** the action SHALL return `ok` with `modelIds: []`

#### Scenario: Incomplete listing
- **WHEN** at least one valid page is read but a later page fails, traversal reaches a bound before completion, or an ID cannot safely be returned
- **THEN** the action SHALL return `partial` with only the safe IDs observed

#### Scenario: Failure before any valid page
- **WHEN** the provider rejects the key, is unreachable, refuses the request, or cannot be checked within the response bound
- **THEN** the action SHALL use the same failure classifications as health for `status` without forwarding the upstream error

### Requirement: Bound and protect provider listing work
Inventory and health SHALL share endpoint validation and the bounded listing operation, with deadlines, response size and page limits, no redirects or request retries, and shared per-catalog and per-process admission for probes. The inventory route SHALL return fixed `429` on admission refusal. Gateway responses and telemetry SHALL NOT expose keys or serialize upstream URLs, cursors, headers, raw bodies or errors.

#### Scenario: Provider echoes the key in a successful listing
- **WHEN** a provider returns any saved key from the parent or its HTTP header wire form in an ID or continuation cursor
- **THEN** the ID SHALL NOT be returned, and the cursor SHALL NOT be followed
- **AND** the key SHALL NOT appear in logs, metrics or traces

### Requirement: Do not cache provider inventory
Each admitted, valid request SHALL query the provider using the current checked parent. The Gateway SHALL NOT cache inventory, and Gateway-produced action responses SHALL use `Cache-Control: no-store`.

#### Scenario: A writer replaces a provider key
- **WHEN** an authorized caller requests a listing after key replacement
- **THEN** the action SHALL query the provider using the newly saved key rather than reuse the earlier listing
