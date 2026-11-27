# Spec Delta

## MODIFIED Requirements

### Requirement: Stack-catalog bounds
Every stack-catalog request SHALL hold the stored catalog to the operator catalog's shape bounds: 1,024 public IDs including aliases, 128 aliases and 8 candidates per model, 320-byte public IDs, and 2,048 bytes per name, description, provider and model value. A call SHALL construct at most 8 provider models and SHALL retain no catalog, built model or key after it ends. Grafana's storage, not a Gateway setting, SHALL bound the stored object's size.

#### Scenario: Catalog above a shape bound
- **WHEN** a stored stack catalog has 1,025 public IDs including aliases
- **THEN** every call against it SHALL fail with the catalog configuration category, with zero provider requests

#### Scenario: No state across requests
- **WHEN** many calls run against many stack catalogs
- **THEN** the Gateway SHALL keep no per-catalog model, key or observer after each call ends
