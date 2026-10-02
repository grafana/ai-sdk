## ADDED Requirements

### Requirement: Registered baseline and design-only evidence

The #321 delivery SHALL consist of a compact reviewed decision and deterministic access probes against the versions registered in `test/conformance/upstream.yaml`. It MUST distinguish pinned-client behavior, current Vercel service documentation, synthetic access witnesses and native producer availability. It MUST NOT modify production codecs, exported APIs, stream lifecycle, execution/retry policy, routing, BYOK or credential capture merely to make probes pass, and MUST NOT depend on stopped #303/#309 implementations.

#### Scenario: Baseline is confirmed
- **WHEN** access probes are implemented or run
- **THEN** their installed package versions and source reference SHALL match the registered baseline
- **AND** any mismatch SHALL be reported and resolved rather than silently using upstream main or a different version

#### Scenario: A client discards injected evidence
- **WHEN** a supported client does not retain a proposed evidence field
- **THEN** the probe SHALL record that limitation without changing production decoding
- **AND** the decision SHALL classify it as an access/implementation gap or explicitly approved support boundary

### Requirement: Actual client and normalization access matrix

Probes SHALL cover unary success, stream setup success/failure, stream success, committed errors followed by valid parts, direct failures, all-failed heterogeneous envelopes and discovery normalization through pinned TS and independent Go clients. They SHALL exercise relevant high-level entry points and consumer middleware with executable public access expressions, not only raw HTTP acceptance. Feasibility demonstrated by test-only access helpers MUST be labeled as a proposed extension, not stock-client support.

#### Scenario: Unary transport is replaced
- **WHEN** an injected unary result contains native request/response fields and distinct Gateway hop headers/body
- **THEN** probes SHALL show client-owned transport replacement and the exact remaining access to native values, including any raw-body-only access
- **AND** they SHALL separately show result metadata accessibility and current Go losses

#### Scenario: Stream error is normalized
- **WHEN** a selected candidate's structured stream error precedes later valid content and finish
- **THEN** probes SHALL show low-level ordering and relevant high-level normalization/termination independently
- **AND** they MUST NOT treat a passing low-level test as proof that the high-level consumer retains every field

#### Scenario: Go high-level access is examined
- **WHEN** Go `GenerateText` or `StreamText` access is claimed
- **THEN** the probe SHALL exercise the actual streaming orchestration path rather than substitute a `DoGenerate` test

#### Scenario: Discovery strips additive fields
- **WHEN** configured aliases, candidates, order and provider-model facts are injected into `/config`
- **THEN** probes SHALL identify fields retained or discarded by stock normalized discovery
- **AND** SHALL demonstrate or identify the minimal explicit authenticated non-inference access needed for discarded facts

### Requirement: Concrete identity, schema and provenance decision

The final decision SHALL specify exact field placement, shapes, event placement, access expressions and replacement/merge behavior for selected/attempt/error evidence, native transport and configured discovery. It SHALL distinguish requested/canonical route, configured candidate, selected provider/model, native response identity, actual invocation, fall-through intent, selection, completion, native retry facts, fallback decisions and Gateway status/classification/client retryability. It SHALL define collision/provenance behavior between service-owned evidence and opaque native metadata, preserve useful authorized identities and unknown valid metadata, and source-link compatible documented Vercel routing semantics separately from pinned consumption proof.

#### Scenario: A secondary candidate is selected
- **WHEN** a synthetic scenario describes a failed primary and selected secondary with differing provider-reported response identity
- **THEN** the decision SHALL attribute each failure and selection to its actual candidate without replacing native identity with a canonical route
- **AND** selection at a first stream part SHALL NOT be reported as completed generation

#### Scenario: Retry facts differ
- **WHEN** native status/retryability differs from Gateway HTTP status or client retryability
- **THEN** the contract SHALL retain those as distinct facts and identify unknown native retry counts explicitly
- **AND** it SHALL distinguish adaptation-after-generation replay risk without promising exactly-once execution

#### Scenario: Metadata collides
- **WHEN** native opaque metadata contains a proposed service-owned path
- **THEN** the decision SHALL define an evidence-backed rule that prevents spoofed service attribution and silent native-value loss
- **AND** native metadata SHALL NOT become trusted authorization or routing input

### Requirement: Source-specific protection and bounded dispositions

The decision SHALL enumerate actual credential-bearing sources/fields and justified numeric input, retention, encoded-byte and cardinality limits, including complete unary/error/SSE/discovery envelopes and encoding overhead. It SHALL define absence, valid empty values, unavailable, redacted, malformed and over-limit dispositions per evidence component. Missing producer information MUST NOT be invented. Optional evidence loss MUST be explicit and MUST NOT silently trim supported content or ordinary metadata, byte-truncate JSON, blanket-ban topology/transport or censor secret-looking application strings.

#### Scenario: A producer lacks native headers
- **WHEN** its current result/stream/error surface does not expose requested native header evidence
- **THEN** the decision SHALL identify that producer boundary and unavailable disposition rather than inventing captured headers or adding a capture framework

#### Scenario: Credential and ordinary application markers coexist
- **WHEN** witnesses contain dummy credentials in known auth/signing/session/cookie/URL/BYOK sources alongside ordinary token-looking application values
- **THEN** the decision SHALL specify source-aware exclusions for credentials and preservation for ordinary supported values
- **AND** it SHALL distinguish secret credential references from authorized provider/model/resource identifiers

#### Scenario: Optional evidence exceeds its allocation
- **WHEN** input or final encoded optional evidence would exceed the chosen numeric/count budget
- **THEN** the decision SHALL specify bounded work and explicit unavailable/over-limit behavior without truncating JSON or displacing ordinary supported data
- **AND** exact-boundary and over-boundary witnesses SHALL justify the disposition and required-envelope behavior

### Requirement: Independent consumer and operator observation

Consumer access SHALL be proved through existing independently configured middleware/wrappers around the clients, including relevant errors. The decision SHALL keep operator recording/capture settings independent from consumer access/capture/destinations, without adding a production observability framework. It MUST identify caller-owned request-body credentials that server projection cannot sanitize and leave known BYOK capture handling to #317.

#### Scenario: Operator capture is disabled
- **WHEN** a consumer wrapper observes injected available evidence under its own configuration with no server capture enabled
- **THEN** its access SHALL not require or implicitly enable operator capture
- **AND** unavailable stock middleware fields SHALL be recorded honestly instead of claimed accessible through raw HTTP alone

#### Scenario: Caller submits BYOK
- **WHEN** the pinned client constructs request metadata from options containing `gateway.byok`
- **THEN** the decision SHALL identify caller-owned credential exposure and #317's consumer capture responsibility
- **AND** SHALL NOT claim server-side redaction retroactively sanitizes the client's request object

### Requirement: Single seam ownership and explicit owner approval

Before #321 is complete, the decision SHALL record an acyclic dependency map with one owner per metadata codec, candidate attribution/capture, error projection, native evidence projection, discovery/access and any approved shared client addition. It SHALL resolve conditional #280/#322 prerequisites for #323 and independent #324 prerequisites, while #316 reuses those delivered seams. The owner MUST explicitly approve the concrete decision, numeric bounds, dispositions and every public API/extension implication before downstream implementation; proposal generation or passing probes alone MUST NOT count as approval.

#### Scenario: A metadata carrier is selected
- **WHEN** the approved contract carries attempt or native evidence through ordinary provider metadata
- **THEN** #280 SHALL own the generic transport/independent decoding and the relevant feature issue SHALL own its evidence fields
- **AND** neither prerequisite SHALL wait on the downstream consumer that requires it

#### Scenario: Approval is still pending
- **WHEN** probes and decision are prepared but the owner has not approved the concrete schema/access/API implications
- **THEN** #321 SHALL remain pending approval and #322–#324 MUST NOT be represented as authorized for contract implementation
- **AND** no sentinel/error-tree protocol, extra stream reader, new collector or error API change SHALL be inferred as approved
