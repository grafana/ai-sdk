## ADDED Requirements

### Requirement: Full gateway provider conformance matrix

The conformance harness SHALL discover the entire existing provider fixture corpus and create a gateway execution row for each fixture with both the registered upstream `@ai-sdk/gateway` client and the Go `providers/grafana` client. The default run SHALL NOT filter cases by supported providers or capabilities, skip unsupported cases, or convert incompatibilities into expected passes. Provider-independent `ui/` mock-model cases SHALL remain in the direct suite and outside the gateway provider inventory.

#### Scenario: Full inventory
- **WHEN** the default gateway suite discovers N provider fixtures
- **THEN** its report contains exactly 2N uniquely identified fixture/client rows
- **AND** every discovered provider and both fixture categories are represented

#### Scenario: A new fixture or provider appears
- **WHEN** a provider fixture is added to the existing corpus
- **THEN** both client rows are included automatically
- **AND** missing execution wiring produces a harness setup failure rather than silent omission

#### Scenario: Local selective execution
- **WHEN** a contributor selects a scenario or client for reproduction
- **THEN** the report identifies the selected scope
- **AND** the filtered result is not represented as a full matrix result

### Requirement: Gateway execution preserves scenario semantics

Gateway streaming scenarios SHALL execute the calling SDK's high-level text streaming and UI conversion pipeline using the fixture's existing inputs and options. Local tools and multi-step orchestration SHALL remain in the calling SDK. Existing unary fixtures SHALL execute the corresponding client model generation operation. The harness SHALL NOT strip options, bypass high-level defaults, substitute another backend protocol, or alter scenario behavior to accommodate gateway limitations.

#### Scenario: High-level default is rejected
- **WHEN** upstream text streaming sends an automatic tool choice and the gateway rejects it
- **THEN** the TypeScript gateway row fails with the rejection evidence
- **AND** the harness does not remove the tool choice or replace the call with low-level streaming

#### Scenario: Multi-step tool execution
- **WHEN** a fixture configures local tool results and multiple model steps
- **THEN** each calling SDK executes its configured tool loop
- **AND** successive gateway backend requests consume the fixture's recorded responses in order

#### Scenario: Unary fixture
- **WHEN** a fixture declares the existing generate operation
- **THEN** both gateway client rows exercise generation rather than substituting streaming
- **AND** applicable unary and request expectations are checked

### Requirement: Gateway replay uses the production container boundary

Gateway conformance SHALL target the actual production gateway image and its real provider adapters, with provider responses supplied by deterministic local replay servers. CI SHALL build the image from the checked-out source using the production Dockerfile and pinned gateway module dependencies. The harness SHALL use isolated test credentials and networking, SHALL NOT require live provider credentials or send model requests to live providers, and SHALL NOT import the gateway implementation into reusable SDK or conformance modules. Reports SHALL identify the image, source, dependency pins, client versions, and registered upstream baseline.

#### Scenario: Offline image execution
- **WHEN** gateway conformance runs without provider credentials
- **THEN** clients call the gateway container and the gateway calls only configured replay backends for model operations
- **AND** production provider conversion and gateway wire handling execute without fabricated provider input files

#### Scenario: Image and checkout provider versions differ
- **WHEN** the gateway module pins older providers than the direct checkout
- **THEN** the image retains its pinned dependencies
- **AND** the report preserves the version distinction for diagnosis

### Requirement: Independent bounded gateway attempts

The harness SHALL isolate replay counters, request capture, tool mock state, and provider configuration between fixture/client attempts. It SHALL bound startup, readiness, execution, and teardown and release owned resources on success, failure, timeout, and cancellation. A provider setup failure or scenario failure SHALL NOT terminate unrelated rows. A provider unavailable before client invocation SHALL remain a failed row with explicit setup-stage evidence, not a claimed client execution.

#### Scenario: Missing backend support
- **WHEN** production gateway setup reports an explicit rejection of a fixture's backend provider configuration
- **THEN** both affected client rows fail at provider setup with the reported cause recorded
- **AND** the report states that client invocation did not occur
- **AND** other provider scenarios continue

#### Scenario: Startup cause is not exposed
- **WHEN** the gateway exits before readiness without exposing a verifiable provider-configuration rejection
- **THEN** affected rows fail as unclassified harness startup failures
- **AND** the report retains Docker state, logs, attempted configuration, and the fact that client invocation did not occur
- **AND** it does not infer provider incompatibility from an opaque process failure

#### Scenario: Separate client replay state
- **WHEN** both clients run the same multi-step fixture
- **THEN** each starts with the first provider response and fresh tool results
- **AND** requests from one attempt cannot consume another attempt's responses

#### Scenario: Scenario timeout
- **WHEN** a client or stream exceeds its execution deadline
- **THEN** that row fails with its stage and available evidence
- **AND** owned resources are cleaned up and subsequent attempts continue

### Requirement: Shared upstream conformance oracle

Both gateway clients SHALL be checked against the existing direct upstream expectations for UI chunks, backend provider requests, and applicable structured-output, usage, and unary artifacts. Gateway replay SHALL NOT write fixture inputs or expectations. Comparisons SHALL preserve the established direct conformance normalization rules and SHALL NOT introduce gateway-specific suppression to obtain passing results. Agreement between the two gateway clients alone SHALL NOT establish conformance. Intentional gateway differences SHALL remain reported discrepancies unless a separate reviewed contract change defines an equivalence.

#### Scenario: Provider request differs despite matching text
- **WHEN** emitted UI chunks match but the gateway changes a behavior-affecting backend request field
- **THEN** the row fails with a request diff identifying the request and field

#### Scenario: Both clients share a defect
- **WHEN** both gateway clients produce the same output but it differs from the direct upstream expectation
- **THEN** both rows fail against that expectation

#### Scenario: Early rejection prevents backend requests
- **WHEN** a gateway call fails before reaching the replay server
- **THEN** the report retains the call error and missing expected backend requests
- **AND** assertions that could not execute are not reported as passing

#### Scenario: Existing normalization remains valid
- **WHEN** outputs differ only in an already permitted direct conformance equivalence, such as adjacent locally executed tool output order
- **THEN** both gateway client comparisons apply the same existing equivalence
- **AND** unrelated chunk ordering and fields remain significant

### Requirement: Complete gateway failure reporting

The gateway suite SHALL produce a machine-readable report and human-readable summary with stable fixture/client identities, execution stages, outcomes, assertion diffs, and diagnostic artifact references. It SHALL reconcile results with the discovered inventory and distinguish compatibility failures, provider setup failures, and harness/infrastructure failures. It SHALL exit nonzero for any failing or unexecuted row, missing or duplicate result, empty default inventory, or infrastructure failure. Diagnostics SHALL redact credentials and bound captured logs.

#### Scenario: Multiple independent failures
- **WHEN** several scenarios fail for different reasons
- **THEN** the suite attempts all independently runnable rows without fail-fast
- **AND** reports totals and per-provider/per-client results with available diffs

#### Scenario: Global infrastructure failure
- **WHEN** image startup infrastructure or the runner fails before all rows can execute
- **THEN** the report identifies the infrastructure failure and affected unexecuted rows
- **AND** the command exits nonzero without claiming provider compatibility results for those rows

#### Scenario: Missing result
- **WHEN** a discovered fixture/client pair has no result record
- **THEN** inventory reconciliation fails the run as incomplete

### Requirement: Independent advisory gateway CI

The repository SHALL provide `mise run test-conformance-gateway` and an independent gateway CI job that runs the full two-client provider matrix. Initially the gateway check SHALL remain non-required for merging while retaining a failing conclusion when the suite fails. It SHALL NOT be a prerequisite of required direct checks or image publication/deployment. Existing required direct conformance and parity commands SHALL retain their enforcement and SHALL NOT gain a dependency on gateway conformance. CI SHALL publish available summaries and diagnostics after failures. Promotion to a required check SHALL be an explicit later policy change.

#### Scenario: Direct conformance passes and gateway conformance fails
- **WHEN** the required direct suite passes and gateway rows fail
- **THEN** the gateway check is visibly unsuccessful with its report available
- **AND** its advisory status does not independently block merging or existing image publication/deployment prerequisites

#### Scenario: Gateway failure is not disguised
- **WHEN** the gateway suite exits nonzero
- **THEN** final CI status preserves that failure after diagnostic collection
- **AND** expected-failure annotations, ignored exit codes, and continue-on-error do not convert the check to success

#### Scenario: Direct regression remains blocking
- **WHEN** an existing direct conformance scenario regresses
- **THEN** the existing required check fails regardless of the advisory gateway result
