## ADDED Requirements

### Requirement: Request-local observation foundation
The Gateway SHALL provide an internal collector with context-local state and immutable configured model wrappers. When invoked by callers, candidate entry SHALL record one-based actual invocation indexes and configured provider/model/instance identity; existing fallback decisions SHALL supply selection and willFallback without changing execution policy. Requested, canonical, configured and native response identities SHALL remain separate. The collector SHALL NOT create a stream reader, retry, cleanup owner or operator-capture dependency.

#### Scenario: Fallback intent does not invoke a candidate
- **WHEN** a failed candidate decision records willFallback and no later candidate enters
- **THEN** the snapshot SHALL retain that intent without inventing a second invocation

#### Scenario: Selection and completion differ
- **WHEN** the collector observes a first part without a terminal generation outcome
- **THEN** it SHALL record selection without completed generation

### Requirement: Sealed and isolated snapshots
Collector operations SHALL be synchronized and request-local. Sealing SHALL reject later mutations. Metadata assembly SHALL NOT mutate provider-owned maps/raw values, SHALL preserve other namespaces, and SHALL relocate the complete original gateway namespace under gateway.nativeMetadata rather than merge native values into trusted routing/evidence.

#### Scenario: Shared model configuration is used concurrently
- **WHEN** separate request contexts observe the same configured model
- **THEN** their attempts and snapshots SHALL remain isolated without source mutation

#### Scenario: Late result arrives after sealing
- **WHEN** a wrapper returns after its state is sealed
- **THEN** retained evidence SHALL remain unchanged

### Requirement: Candidate-local native error projection
Projection SHALL retain available bounded structured native message, type, string/number code, status and retryability without inferring attribution from joined errors or arbitrary prose. Missing producer fields SHALL remain absent or explicitly unavailable. Current stream error projection SHALL be separate from retained prior failures, without an unbounded per-event history.

#### Scenario: Joined errors have no candidate identity
- **WHEN** projection receives an aggregate rather than attributable single-error evidence
- **THEN** it SHALL NOT assign an arbitrary aggregate member to a candidate

#### Scenario: Producer omits native fields
- **WHEN** an adapter exposes no code, type or actual retry count
- **THEN** projection SHALL NOT manufacture those facts

### Requirement: Known-source protection and raw fidelity
Projection SHALL exclude known credential, tenant, signing and credential-location fields and configured credential/endpoint echoes before public retention. Ordinary identifiers and token-looking application strings SHALL survive. Valid surviving raw string/key escapes and numeric lexemes SHALL remain intact; invalid UTF-8 and ambiguous duplicate members, including protected subtrees, SHALL be rejected. Partial removal SHALL be represented explicitly without mutating inputs.

#### Scenario: Useful details accompany credential material
- **WHEN** a native component contains protected fields alongside ordinary model/application strings
- **THEN** protected values SHALL be removed, surviving values SHALL remain available with redacted true, and the source SHALL remain unchanged

#### Scenario: Duplicate escaped member conceals a credential
- **WHEN** a component contains duplicate members, including equivalent escaped keys
- **THEN** projection SHALL reject the component rather than publish unchecked raw JSON

### Requirement: Complete bounded retention and snapshots
The collector SHALL enforce 16 retained attempts, 1,048,576 source bytes per component, 131,072 complete encoded bytes per component, 262,144 success detail bytes, 32,768 essential bytes and 24,576 optional error detail bytes. Beyond 16 actual invocations it SHALL retain an over-limit attemptCount/count disposition instead of a partial history. Components SHALL preserve complete available values including empty/null, or use unavailable, redacted, malformed or over-limit with approved closed reasons. Optional overflow SHALL collapse to dispositions; essential overflow SHALL return an assembly failure rather than truncate messages. Runtime document/frame/error limits remain the later integration layer's responsibility.

#### Scenario: Sixteen versus seventeen actual entries
- **WHEN** collection observes 16 or 17 candidates
- **THEN** snapshots SHALL respectively contain the full array or the explicit observed-count disposition without preventing execution

#### Scenario: Escaping expands a component
- **WHEN** complete encoding, including the component wrapper and redaction flag, crosses its allocation
- **THEN** the component SHALL become over-limit rather than exceed the budget or emit truncated JSON

#### Scenario: Optional aggregate exceeds its allocation
- **WHEN** details collectively exceed the success or error allocation
- **THEN** fitting essential facts SHALL remain and overflowing optional details SHALL use bounded dispositions

### Requirement: Namespace schema without runtime activation
The Gateway SHALL define a JSON schema for its trusted routing/evidence/nativeMetadata namespace and test valid snapshots, overflow/disposition shapes and invalid values. The collector/schema change alone SHALL NOT wire the service or handlers, change public ProviderWire carriers or claim production evidence delivery. Gateway compilation SHALL require Go 1.27 for standard jsontext; the independent Grafana client's Go baseline SHALL remain 1.26.3.

#### Scenario: Foundation is introduced
- **WHEN** only this change and its prerequisites are applied
- **THEN** existing runtime calls SHALL retain their previous output behavior while package and schema tests exercise the new foundation explicitly

#### Scenario: Invalid evidence shape is validated
- **WHEN** schema validation receives unrun attempts, an invalid selected index, a partial over-limit history or an unavailable-state value
- **THEN** it SHALL reject the invalid shape
