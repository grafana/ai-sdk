## MODIFIED Requirements

### Requirement: Nested logger middleware module

The repository SHALL provide a nested Go module at `middleware/logger` with module path `github.com/grafana/ai-sdk/middleware/logger`.

The module SHALL depend on the root `github.com/grafana/ai-sdk` module and the Go standard library. It SHALL NOT introduce dependencies on OpenTelemetry SDKs, Agent Observability, gRPC, vendor SDKs, provider modules, or other third-party logging libraries.

The root `github.com/grafana/ai-sdk` module SHALL NOT import `middleware/logger`, so consumers who import only the root module do not gain logger-specific dependencies or public API.

#### Scenario: Root consumers do not import logger

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk`
- **THEN** `github.com/grafana/ai-sdk/middleware/logger` SHALL NOT appear in the consumer's transitive import graph

#### Scenario: Logger module depends only on root ai-sdk and standard library

- **WHEN** running dependency inspection for `./middleware/logger/...`
- **THEN** dependencies outside the Go standard library SHALL be limited to `github.com/grafana/ai-sdk`
- **AND** the dependency graph SHALL NOT include Agent Observability, OpenTelemetry SDKs, gRPC, vendor provider SDKs, or `github.com/grafana/ai-sdk/providers/*`

### Requirement: Composition and documentation

The logger middleware SHALL compose as an ordinary `middleware.Middleware` with `middleware.Wrap`, `middleware.WrapLanguageModel`, `registry.WithLanguageModelMiddleware`, fallback models, and `middleware/agentobservability`.

The package documentation and user-facing docs SHALL explain:

- how to wrap a single model;
- how to attach the logger through `registry.WithLanguageModelMiddleware`;
- privacy defaults and capture/redaction controls;
- that root `GenerateText` currently appears to provider middleware as stream calls because it uses `StreamText` internally;
- that middleware ordering controls whether logs are outside or inside Agent Observability hooks/recording;
- that this package is a lightweight provider-layer logger, not the full upstream telemetry integration system.

#### Scenario: Registry applies logger middleware

- **WHEN** a `registry.ProviderRegistry` is created with `registry.WithLanguageModelMiddleware(logger.Middleware(opts))`
- **AND** a model is resolved from the registry
- **THEN** calls through the resolved model SHALL emit logger middleware records

#### Scenario: Agent Observability ordering is caller controlled

- **WHEN** logger middleware is placed before Agent Observability middleware in the `middleware.Wrap` slice
- **THEN** logger SHALL be the outer middleware according to existing middleware ordering
- **AND** docs SHALL describe that this logs attempted calls including Agent Observability denials

#### Scenario: Future telemetry boundary is documented

- **WHEN** users read the logger package or guide documentation
- **THEN** the documentation SHALL state that the logger observes provider calls only
- **AND** it SHALL NOT claim to replace operation-level telemetry, tracing integration registries, or tool execution telemetry
