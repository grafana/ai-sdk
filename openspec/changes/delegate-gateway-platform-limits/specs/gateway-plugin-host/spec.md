## MODIFIED Requirements

### Requirement: Host-owned lifecycle and bounded final flush
Draining in-flight requests SHALL belong to the host; the Gateway SHALL NOT depend on receiving a termination signal and SHALL NOT cancel requests on its own when shutdown begins. After the plugin protocol server stops, the Gateway SHALL flush Agent Observability and attempt records concurrently under one shared deadline of 1 second, then exit. Records emitted after the flush starts SHALL be dropped and counted, not block or fail.

#### Scenario: Host stops the plugin
- **WHEN** the host stops the plugin process through the plugin protocol, with host tracing unconfigured or its shutdown completing promptly
- **THEN** the Gateway SHALL run its final flush without a termination signal and exit before the host's forced-kill delay

#### Scenario: Final flush exceeds budget
- **WHEN** queued telemetry cannot be delivered within the final-flush budget
- **THEN** the Gateway SHALL abandon it, account for the loss with bounded counters or fixed diagnostics, and exit before the budget ends

#### Scenario: Deployment guidance matches request bounds
- **WHEN** operator documentation describes deployment
- **THEN** it SHALL state that the host's termination grace period must exceed the platform request deadline plus drain time, and that host tracer shutdown shares the final-flush window

### Requirement: Protocol test host
Deterministic tests SHALL serve the Gateway through the real plugin route protocol, from HTTP through the plugin route RPC and route adapter to the Gateway handlers, with test-signed delegated tokens and fake native providers. Cross-language suites SHALL launch the built plugin executable as a plugin host does. The test host SHALL be test-only and SHALL NOT be built, packaged or documented as a supported server, or as evidence of the platform's route deadline value or host draining. It MAY apply a test route deadline to the route RPC, to prove that a host deadline reaches the plugin.

#### Scenario: Test host exercises the protocol hop
- **WHEN** a client test sends a request to the test host
- **THEN** the request SHALL reach the Gateway through the same route protocol and identity delegation the plugin host uses, rather than calling Gateway handlers directly

#### Scenario: Real plugin process under test
- **WHEN** a cross-language suite starts the test host
- **THEN** the Gateway SHALL run as the built plugin executable, and health, metrics collection, relayed logs and stop SHALL be observed through the plugin protocol

#### Scenario: Legacy command evidence
- **WHEN** an existing requirement cites authenticated command evidence
- **THEN** that evidence SHALL be produced through this test host

#### Scenario: Test host is not shipped
- **WHEN** the plugin package is built
- **THEN** it SHALL contain no test host executable or configuration

## ADDED Requirements

### Requirement: Host deadline reaches the plugin
A deadline the host sets on a route RPC SHALL reach the Gateway as the deadline of the route request's context, and its expiry SHALL cancel selection and the model call. The Gateway SHALL classify the outcome from the request context's error, which the route RPC reports as a cancellation. Without a host deadline, the request context SHALL carry none, and the Gateway SHALL NOT add one.

#### Scenario: Route deadline carried into the plugin
- **WHEN** the test host forwards a language-model request with a route deadline
- **THEN** the Gateway's request context SHALL carry a deadline no later than the route deadline

#### Scenario: Route deadline ends a blocked call
- **WHEN** a fake native provider blocks past the route deadline, in unary and streaming mode
- **THEN** the native request SHALL be canceled once the deadline expires, and the plugin SHALL start no further attempt
- **AND** the Gateway SHALL record the outcome as the request context's error reports it, without inferring a timeout from the deadline

#### Scenario: Route without a deadline
- **WHEN** the test host forwards a request without a route deadline
- **THEN** the Gateway's request context SHALL carry no deadline, and caller cancellation SHALL still cancel the native request

