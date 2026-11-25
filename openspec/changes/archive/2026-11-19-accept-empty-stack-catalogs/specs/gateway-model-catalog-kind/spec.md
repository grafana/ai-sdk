## MODIFIED Requirements

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
