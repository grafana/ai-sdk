## MODIFIED Requirements

### Requirement: Prompt tool history reconstruction
As the first step of a call, before host policy, the context check and selection, the runtime SHALL rebuild tool history from the prompt. It SHALL fail inconsistent history as an invalid request with no reason, zero selections and zero model calls. The pending provider-executed calls, by ID and tool name, SHALL seed stream and unary validation. History SHALL stay internal to the runtime and SHALL NOT change after it is rebuilt; each validator SHALL use its own copy.

#### Scenario: Deferred result
- **WHEN** the prompt holds a provider-executed tool call without a result
- **THEN** the call SHALL proceed, and that call SHALL be pending with its tool name

#### Scenario: Orphan result
- **WHEN** the prompt holds a tool result with no earlier call of the same ID
- **THEN** the call SHALL proceed, and a later prompt call with that ID SHALL fail the call as an invalid request

#### Scenario: Conflicting duplicates
- **WHEN** a prompt call ID repeats after its result, a second time, with another tool name, or where either call is not provider-executed, or a result for a known call names another tool or repeats
- **THEN** the call SHALL fail as an invalid request with zero selections and zero model calls

#### Scenario: Approval re-emission
- **WHEN** a pending provider-executed call is re-emitted once with the same name in a later assistant turn, with or without its result
- **THEN** the call SHALL proceed, and the call SHALL be pending only if no result followed

#### Scenario: History is not shared
- **WHEN** two validators are seeded from one rebuilt history and one of them accepts a result for a pending call
- **THEN** the other validator SHALL still treat that call as pending, and the history SHALL be unchanged

#### Scenario: History precedes policy and context
- **WHEN** a call has inconsistent history together with a protected call header, or its context has already ended
- **THEN** the call SHALL fail as an invalid request with no reason

#### Scenario: ProviderWire document
- **WHEN** ProviderWire receives inconsistent history
- **THEN** it SHALL write the same fixed 400 invalid-request document as before this change

### Requirement: Adapter representation checks
Adapters SHALL NOT own or evaluate output lifecycle rules; they SHALL act on the runtime's outcome. They own checks about their own representation and MAY keep content-validity checks that are not lifecycle rules. An adapter that refuses a part as invalid SHALL end the stream. It MAY omit parts it does not represent only when they carry no lifecycle state beyond the start position.

#### Scenario: Representation rejection
- **WHEN** a part passes lifecycle validation but ProviderWire cannot represent it, such as invalid UTF-8, malformed metadata or a token count outside the JavaScript safe-integer range
- **THEN** ProviderWire SHALL reject that part with the same internal terminal error as before this change

#### Scenario: Content-validity checks stay in ProviderWire
- **WHEN** a part passes lifecycle validation but fails ProviderWire's source, custom-kind, file-data, non-negative token-count, response-identity or recovered-panic check
- **THEN** ProviderWire SHALL reject it with the same internal terminal error or 500 document as before this change

#### Scenario: Omitted raw parts
- **WHEN** ProviderWire receives a raw part that was not requested or that it cannot project
- **THEN** it SHALL omit the part and keep streaming, as before this change

#### Scenario: No second lifecycle evaluation
- **WHEN** ProviderWire receives a stream part or unary result that the runtime accepted
- **THEN** it SHALL apply only its representation and content-validity checks, and SHALL NOT reject the part or result for a lifecycle rule

#### Scenario: Agreement at the same part
- **WHEN** the runtime returns invalid output at a stream part, for any lifecycle rule class, or for a unary result
- **THEN** ProviderWire SHALL write the same response as for a content rejection it owns at that part or result: the frames of the earlier parts and then one internal terminal error, or the 500 internal-error document, with the same execution metadata
