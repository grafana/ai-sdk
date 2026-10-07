## MODIFIED Requirements

### Requirement: Immutable static catalog
The package SHALL provide a static catalog constructor that copies its entries and nested metadata slices, including explicitly supplied configured candidate facts, stores only non-nil models, and exposes no mutation API.

#### Scenario: Source entries are mutated after construction
- **WHEN** the caller mutates the input entries, aliases, capabilities or configured candidates after successful construction
- **THEN** subsequent catalog resolution and listing SHALL remain unchanged

#### Scenario: Static model resolves
- **WHEN** a canonical ID or registered alias identifies a static entry
- **THEN** the catalog SHALL return that entry's canonical ID and model

#### Scenario: Request context is not retained
- **WHEN** the static catalog resolves or lists with a request context
- **THEN** it SHALL complete without storing the context in catalog state

### Requirement: Stable model listing
Listing SHALL return one `ModelInfo` per canonical entry in ascending canonical-ID order. Each entry SHALL include its canonical ID and SHALL preserve optional name, description, aliases, capabilities and configured candidate facts supplied at construction, including candidate and alias order. Each returned nested slice SHALL be independently copied.

#### Scenario: Catalog contains aliases
- **WHEN** models are listed
- **THEN** aliases SHALL appear as metadata on their canonical entry and SHALL NOT appear as separate catalog model rows

#### Scenario: Listing result is mutated
- **WHEN** a caller mutates the returned list, aliases, capabilities or configured candidate facts
- **THEN** later listing and resolution SHALL remain unchanged

#### Scenario: Empty catalog is listed
- **WHEN** a valid catalog has no entries
- **THEN** listing SHALL return an empty list without an error

#### Scenario: Registry route supplies configured facts
- **WHEN** a registry-backed catalog explicitly receives ordered candidate metadata
- **THEN** construction/listing SHALL preserve and defensively copy those facts without deriving additional candidates from registry lookup

### Requirement: Public model metadata semantics
`ModelInfo` SHALL require a canonical public ID and SHALL support optional presentation name, description, explicit aliases, typed model capabilities and explicitly supplied configured candidates. Each candidate SHALL contain only provider-instance reference, effective adapter/provider identifier and configured invocation model ID. Metadata SHALL describe the authorized configured public route, not selected/completed attempts or provider-reported response identity. Canonical IDs, aliases, capabilities and candidates SHALL be supplied by the catalog owner; the catalog SHALL NOT derive them from `LanguageModel.ModelID()`, provider `ModelIDs()` inventories, built-in public-name policy or runtime responses. Missing configured candidate metadata SHALL remain missing rather than be invented. Credentials and secret references SHALL NOT be represented by candidate metadata.

#### Scenario: Provider inventories do not create public routes
- **WHEN** provider packages expose supported model ID inventories
- **THEN** the catalog SHALL NOT register those IDs or infer public names or candidate mappings unless the catalog owner supplies explicit entries

#### Scenario: Fallback route declares capabilities
- **WHEN** a public route can select more than one backend model
- **THEN** its declared capabilities SHALL represent behavior guaranteed by every possible backend

#### Scenario: Presentation metadata is omitted
- **WHEN** an entry supplies only its required canonical ID
- **THEN** the catalog SHALL accept and list the entry without inventing provider-derived metadata

#### Scenario: Model-reported identity differs
- **WHEN** the resolved model reports a provider-specific identity
- **THEN** listing metadata SHALL remain the explicitly configured public metadata and candidate facts

#### Scenario: Configured fallback candidates are inspected
- **WHEN** the catalog owner supplies primary and fallback candidate facts in explicit order
- **THEN** listing SHALL retain that order without executing candidates or identifying any as selected or successful
