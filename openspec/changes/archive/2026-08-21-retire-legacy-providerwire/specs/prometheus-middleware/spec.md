## MODIFIED Requirements

### Requirement: Nested Go module for Prometheus middleware

`middleware/prometheus/` SHALL be a separate Go module under the ai-sdk repository, declared with `module github.com/grafana/ai-sdk/middleware/prometheus` and `replace github.com/grafana/ai-sdk => ../../`, following the existing nested middleware module convention.

The module SHALL depend on the root `github.com/grafana/ai-sdk` module and the Prometheus Go client. The root ai-sdk module SHALL NOT import `middleware/prometheus` and SHALL NOT gain a dependency on `github.com/prometheus/client_golang`.

The module documentation SHALL describe that the middleware records local/client-side provider-call metrics and does not configure remote hosted-service metrics controls.

#### Scenario: Root module dependency isolation

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk` or root module tests list dependencies from the repository root
- **THEN** `github.com/prometheus/client_golang` SHALL NOT appear in the root module dependency graph

#### Scenario: Nested module path

- **WHEN** `middleware/prometheus/go.mod` is inspected
- **THEN** it SHALL declare module path `github.com/grafana/ai-sdk/middleware/prometheus`
- **AND** it SHALL replace `github.com/grafana/ai-sdk` with `../../`

#### Scenario: Hosted metrics controls remain independent

- **WHEN** documentation describes Prometheus middleware alongside remote hosted-service controls
- **THEN** it SHALL state that Prometheus middleware measures local client-side provider calls
- **AND** it SHALL NOT require option types from a removed provider module

### Requirement: Documentation and validation coverage

The module SHALL include package documentation describing the public API, metric contract, default buckets, provider-call scope, stream finalization behavior, privacy/cardinality guardrails, Agent Observability composition ordering, registry integration, and the boundary between local metrics and remote service controls.

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
