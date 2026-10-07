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

### Requirement: Known-source protection and standard JSON normalization
Projection SHALL exclude known credential, tenant, signing and credential-location fields and configured credential/endpoint echoes before public retention. Ordinary identifiers, URLs, endpoints and token-looking application strings SHALL survive unless they contain a protected source. It SHALL use standard Go JSON decoding/encoding with numeric precision preserved by UseNumber. Duplicate members SHALL follow standard Go processing, string/key escapes and lone surrogates MAY normalize, and invalid original UTF-8 or incomplete JSON SHALL be rejected. Summary and details SHALL derive from the same protected decoded representation; original raw diagnostics SHALL NOT be republished after protection. Partial removal SHALL be explicit without mutating inputs. This owner-approved revision supersedes the diagnostic lexical-preservation and duplicate-rejection rules of #321, without changing opaque metadata or independent client raw retention.

#### Scenario: Useful details accompany credential material
- **WHEN** a native component contains protected fields alongside ordinary model/application strings
- **THEN** protected values SHALL be removed, surviving values SHALL remain available with redacted true, and the source SHALL remain unchanged

#### Scenario: Duplicate escaped member contains a credential
- **WHEN** a component contains duplicate members, including equivalent escaped keys
- **THEN** standard decoding SHALL select the surviving value and projection SHALL protect that value before encoding
- **AND** neither summary nor details SHALL publish discarded original raw members

#### Scenario: Numeric precision and ordinary escaped values
- **WHEN** a component contains large JSON numbers, escaped strings or lone surrogate escapes
- **THEN** standard encoding MAY normalize strings and keys without rounding numbers or changing null/false/empty distinctions

### Requirement: Complete bounded retention and snapshots
Collection and normalization SHALL bound retained attempts to 16, source bytes per component to 1,048,576, complete encoded components to 131,072 bytes and retained diagnostics to 262,144 bytes. The separate assembler SHALL enforce 262,144 success detail bytes, 32,768 essential bytes and 24,576 optional error detail bytes, without whole-state serialization on collector mutation. Beyond 16 actual invocations it SHALL retain an over-limit attemptCount/count disposition instead of a partial history. Components SHALL preserve complete available values including empty/null, or use unavailable, redacted, malformed or over-limit with approved closed reasons. Optional overflow SHALL collapse to dispositions; essential overflow SHALL return an assembly failure rather than truncate messages. Runtime document/frame/error limits remain the later integration layer's responsibility.

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
The Gateway SHALL define a JSON schema for its trusted routing/evidence/nativeMetadata namespace and test valid snapshots, overflow/disposition shapes and invalid values. The collector/schema change alone SHALL NOT wire the service or handlers, change public ProviderWire carriers or claim production evidence delivery. The implementation SHALL use ordinary encoding/json without jsontext or a new dependency. Gateway/workspace and independent Grafana client Go minimums SHALL remain 1.26.3.

#### Scenario: Foundation is introduced
- **WHEN** only this change and its prerequisites are applied
- **THEN** existing runtime calls SHALL retain their previous output behavior while package and schema tests exercise the new foundation explicitly

#### Scenario: Invalid evidence shape is validated
- **WHEN** schema validation receives unrun attempts, an invalid selected index, a partial over-limit history or an unavailable-state value
- **THEN** it SHALL reject the invalid shape
