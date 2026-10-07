## MODIFIED Requirements

### Requirement: Cancellation and timeouts
The configured total timeout SHALL begin immediately before authenticated host selection, after bounded protocol input validation, and cover selection/construction, logical `DoStream` setup, all credential attempts and stream consumption. An earlier request-context deadline SHALL remain effective. Logical invocation, credential attempts and stream establishment SHALL NOT reset or extend the shared deadline. Selection cancellation and late-result cleanup SHALL follow the shared bounded invocation contract.

After stream commitment, the configured idle timeout SHALL restart after each accepted provider part is successfully represented, including a consumed start and a written provider error. Request cancellation, total timeout, or idle timeout SHALL cancel selection/provider work before the corresponding safe terminal response or event is written. A valid finish written before another terminal outcome SHALL remain authoritative. When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence. Writer failure SHALL terminate immediately without another write.

#### Scenario: Terminal conditions race
- **WHEN** provider output, request cancellation, or a configured timeout become ready concurrently before terminal output
- **THEN** the handler SHALL produce at most one applicable bounded terminal outcome and SHALL not leak or clean up the stream twice

#### Scenario: Selection exhausts streaming time
- **WHEN** the total timeout expires during delayed or non-cooperative selection
- **THEN** handler latency SHALL remain bounded with a pre-commit JSON failure, zero logical model invocations and bounded cleanup of late selection results

#### Scenario: Attempts inherit the remaining budget
- **WHEN** selection consumes part of the total duration and the chosen model then tries multiple eligible credentials
- **THEN** every attempt and subsequent stream consumption SHALL use only the remaining shared budget without resetting it

#### Scenario: Total timeout cancels provider before output
- **WHEN** the total timeout expires during setup or stream consumption
- **THEN** the model context SHALL be canceled before the timeout response or event is encoded or written

#### Scenario: Idle timeout cancels provider before output
- **WHEN** the idle timeout expires after stream establishment
- **THEN** the model context SHALL be canceled before the idle-timeout event is encoded or written

#### Scenario: Provider finish arrives before termination
- **WHEN** a valid finish is written before cancellation or timeout terminates the stream
- **THEN** finish SHALL be the authoritative final event

#### Scenario: Accepted activity resets idle time
- **WHEN** accepted provider parts arrive within each idle interval while total duration remains available
- **THEN** the idle timeout SHALL restart after each represented part and SHALL not expire solely because total stream age exceeds one idle interval

### Requirement: Shared strict streaming request pipeline
The handler SHALL accept `ai-language-model-streaming` only when its single exact value is `true` or `false`. It SHALL route `false` to the existing unary path and `true` to the streaming path only after the same bounded body read, standard Go JSON and complete request-schema validation, supported request mapping and exactly one authenticated host selection. Selection SHALL return a non-nil V4 model and a non-empty valid-UTF-8 logical identity appropriate to its account-access policy. Configured access SHALL resolve the catalog exactly once and retain canonical identity; BYOK SHALL select from request data with zero catalog calls and use the request-scoped observation contract.

A supported streaming request SHALL invoke the selected logical model's `DoStream` exactly once and SHALL NOT invoke `DoGenerate`. The selected logical model MAY own bounded credential attempts under gateway-request-byok; these SHALL NOT cause repeated host selection or bypass existing logical SSE commitment and single-owner cleanup. Any failure before stream invocation SHALL select a bounded privacy-safe non-2xx JSON document and SHALL produce no SSE commitment.

#### Scenario: Supported streaming envelope executes once
- **WHEN** a valid configured-access request uses streaming value `true` and passes mapping and selection
- **THEN** host selection, configured resolution and logical `DoStream` SHALL each run once, and `DoGenerate` SHALL not run

#### Scenario: BYOK streaming envelope executes without a catalog
- **WHEN** a valid BYOK request uses streaming value `true` and passes mapping and request-only selection
- **THEN** host selection and logical `DoStream` SHALL each run once, with zero catalog calls and zero `DoGenerate` calls
- **AND** any eligible credential attempts SHALL remain inside the selected logical model's bounded execution

#### Scenario: Streaming request fails before invocation
- **WHEN** a streaming request fails envelope, body, standard JSON, schema, mapping or selection processing
- **THEN** selection or model invocation SHALL not run after an earlier failure and the response SHALL remain a bounded privacy-safe non-2xx JSON error rather than SSE

#### Scenario: Invalid streaming selector
- **WHEN** the streaming header is missing, empty, repeated, or has a value other than exact `true` or `false`
- **THEN** envelope validation SHALL fail before body mapping, selection or model invocation
