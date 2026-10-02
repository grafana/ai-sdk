## MODIFIED Requirements

### Requirement: Authenticated public discovery
`Provider.ListModels(ctx)` SHALL issue authenticated `GET /config`, read within the configured discovery limit, and return public ID, name, optional description and the specification version/provider/model-ID triple, plus optional typed `ModelInfo.Gateway *ConfiguredRoute`. `ConfiguredRoute` SHALL expose canonical model ID, aliases and ordered configured candidates; each `ConfiguredCandidate` SHALL expose provider-instance, provider and configured model ID. This SHALL remain the existing discovery method, without a second client or AGPL module dependency.

The client SHALL validate required values, registered `v4` and `grafana` identity, model-ID consistency, valid public IDs, duplicate IDs and every recognized configured-route field, including route-group consistency and duplicate aliases/candidate tuples. It SHALL independently enforce the gateway-configured-discovery row/candidate/alias/string limits and existing configurable document-byte limit. It SHALL preserve response order and aliases as independent rows exactly as served. Missing gateway SHALL remain nil; present null/incomplete/malformed gateway SHALL invalidate the whole response. Unknown additive members accepted by the registered client SHALL remain ignored. Configured mappings SHALL be retained only when supplied by the server, never inferred from responses, models or inventories; credentials and arbitrary configuration SHALL NOT be exposed.

#### Scenario: Canonical and alias rows are discovered
- **WHEN** the authenticated service returns a canonical model and alias row with configured route facts
- **THEN** both rows SHALL be returned in server order with their public specification fields and matching typed gateway facts

#### Scenario: Discovery contains additive metadata
- **WHEN** otherwise valid discovery rows or the root document contain unrelated unknown members
- **THEN** the client SHALL ignore those members without exposing them through ModelInfo
- **AND** a recognized gateway extension SHALL be validated and retained rather than discarded

#### Scenario: Discovery is structurally unsafe
- **WHEN** the document is oversized, malformed, duplicated, contains an invalid ID, mismatched specification model ID, non-v4 specification, non-grafana provider, malformed gateway facts, over-limit collections/strings or inconsistent configured groups
- **THEN** discovery SHALL fail atomically with no partial catalog result

#### Scenario: Server has no configured extension
- **WHEN** a valid complete public discovery response omits gateway
- **THEN** existing public rows SHALL remain consumable and each Gateway field SHALL be nil, with no inferred topology

#### Scenario: Caller inspects candidates without generation
- **WHEN** an authorized Go caller reads ListModels rows and inspects Gateway.Candidates
- **THEN** configured order and provider-instance/provider/model facts SHALL be available without any model or inventory request

### Requirement: No implicit client retry or backend selection
The Grafana client SHALL issue at most one Gateway model request per `DoGenerate` or `DoStream` invocation after token acquisition. It SHALL preserve retryability for existing SDK retry/fallback orchestration but MUST NOT select physical providers, traverse Gateway candidates, retry through the retired endpoint, or retry after any response or stream event. Discovery SHALL retain authorized configured facts only through the approved optional gateway field; inspecting them SHALL NOT implement backend selection. This discovery exception SHALL NOT change runtime public result/error projection, which remains governed by its separate contracts.

#### Scenario: Retryable setup error occurs
- **WHEN** the Gateway returns a retryable non-2xx response
- **THEN** the invocation SHALL return that retryable error after one Gateway request and leave any retry decision to its caller

#### Scenario: Stream error occurs after output
- **WHEN** an error event or transport failure occurs after a stream part has been delivered
- **THEN** the client SHALL not issue another HTTP request or change model identity

#### Scenario: Configured candidates are inspected
- **WHEN** ListModels exposes more than one configured candidate
- **THEN** the client SHALL not contact, select or probe any of them

### Requirement: Exact-pinned differential and black-box evidence
Automated tests SHALL compare equivalent Go and registered Gateway-client scenarios for semantic method, path, effective protocol and call headers, body presence, normalized unary and stream results, error category, retryability, cancellation, discovery, `[DONE]`, raw filtering, timestamp conversion and EOF. Tests SHALL use the versions registered in `test/conformance/upstream.yaml`; a baseline change SHALL update pins, lockfiles, captures, classification and client behavior together. Separate hostile fake-server tests SHALL prove all client bounds and resource cleanup. Authenticated black-box tests SHALL exercise the Gateway command over HTTP without importing Gateway implementation packages into Apache production code.

Configured-discovery tests SHALL additionally prove Go typed retention and the approved TS helper against the same command, while explicitly preserving stock normalized TS extension loss. Configured facts SHALL be available without inference and without credentials or unrelated account state; this SHALL NOT imply new runtime response/error identity retention.

#### Scenario: Equivalent text calls are compared
- **WHEN** the differential suite issues representable unary and streaming text/scalar calls through both clients
- **THEN** their semantic requests and normalized observable results SHALL agree except for documented parity-preserving Go adaptations

#### Scenario: Registered baseline changes
- **WHEN** the Gateway or provider package pin changes
- **THEN** baseline validation SHALL fail until differential evidence and every observed divergence are reviewed and updated together

#### Scenario: Client bounds are tested
- **WHEN** fake endpoints exercise exact-limit and one-byte/one-event-over-limit discovery, unary, error and SSE inputs plus cancellation under blocked delivery
- **THEN** tests SHALL prove bounded allocation/read behavior, body closure, channel closure and absence of retained client goroutines

#### Scenario: Authenticated command is exercised
- **WHEN** the repository integration suite starts the command with deterministic auth and provider fakes
- **THEN** the Go client SHALL complete discovery, unary text, streaming text, acting-user propagation, cancellation and registered errors without exposing configured credentials or unrelated account state
- **AND** authorized configured topology SHALL be available only through its approved discovery projection, with runtime behavior unchanged by this feature

#### Scenario: Configured discovery consumers are compared
- **WHEN** Go ListModels, pinned TS getAvailableModels and the shipped TS helper inspect a configured canonical/alias catalog
- **THEN** normalized public fields SHALL remain compatible, Go and the helper SHALL retain matching configured facts, stock TS SHALL still strip the extension and provider inference counts SHALL remain zero
