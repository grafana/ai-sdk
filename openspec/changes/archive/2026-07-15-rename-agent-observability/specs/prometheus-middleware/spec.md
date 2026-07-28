## MODIFIED Requirements

### Requirement: Documentation and validation coverage

The module SHALL include package documentation describing the public API, metric contract, default buckets, provider-call scope, stream finalization behavior, privacy/cardinality guardrails, Agent Observability composition ordering, registry integration, and Grafana hosted metrics independence.

The implementation SHALL include tests using `prometheus.NewRegistry()` and `prometheus/testutil` that cover collector registration, duplicate registration, generate success/error/cancellation, stream success/error/cancellation, response identity preference, requested identity mode, normalizers, stream chunk/timing metrics, disabled stream chunk metrics, registry integration, privacy label exclusions, and root dependency isolation.

The repository task configuration SHALL include a targeted Prometheus middleware test task and include the nested module in aggregate test, short-test, vet, tidy, and build tasks.

#### Scenario: Package docs cover composition order

- **WHEN** a user reads the `middleware/prometheus` package documentation
- **THEN** it SHALL explain how to compose Prometheus with Agent Observability when measuring provider calls closest to the provider
- **AND** it SHALL explain that putting Prometheus outside Agent Observability measures a broader wrapped-model operation

#### Scenario: Registry integration is tested

- **WHEN** a model is resolved through a registry configured with a middleware returned by `prometheus.Middleware(opts)`
- **THEN** provider calls through that model SHALL emit Prometheus metrics

#### Scenario: Aggregate tasks include nested module

- **WHEN** contributors run aggregate test, short-test, vet, tidy, or build tasks after implementation
- **THEN** those tasks SHALL include `middleware/prometheus` alongside existing nested modules
