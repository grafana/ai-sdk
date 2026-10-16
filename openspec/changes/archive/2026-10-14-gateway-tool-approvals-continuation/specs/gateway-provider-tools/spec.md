## MODIFIED Requirements

### Requirement: Tool option consuming authority and deferred content boundaries
Native-consuming adapters SHALL reject tool identity, shape or authority overrides at their consuming scopes without projecting unknown extensions away. Registered text/json/error-text/error-json/execution-denied output options and nested text/file options SHALL reach native consumers. The content output wrapper SHALL NOT accept providerOptions; deferred content families SHALL remain unsupported.

#### Scenario: Tool option consuming authority and deferred content boundaries
- **WHEN** an ordinary tool option at its consuming scope would override validated tool identity or authority
- **THEN** the adapter SHALL reject that bypass without projecting unknown extensions away, while supported nested output/content options SHALL remain available

#### Scenario: Nested continuation options
- **WHEN** a continuation contains registered options on tool result output and its supported text/file content
- **THEN** both clients and both HTTP modes SHALL preserve the options into the next native request

### Requirement: History-aware deferred result correlation
Output results SHALL match current calls or pending provider-owned history. One matching pending native call re-emission SHALL be accepted in output and accumulated history. Unknown, mismatched, completed, client-owned or repeated duplicates and out-of-order results SHALL fail safely. Truncated unrelated results SHALL NOT seed calls. State SHALL retain only bounded identity/lifecycle data per request.

#### Scenario: Deferred success or error
- **WHEN** a provider call finishes without a result and the next request includes it unresolved in history
- **THEN** unary and streaming success/error results SHALL be accepted without repeating the call

#### Scenario: Invalid historical match
- **WHEN** an output matches only unknown, completed, out-of-order, mismatched or client-owned history
- **THEN** unary output SHALL fail before commitment and streaming SHALL use the bounded safe terminal path

#### Scenario: Native approval call re-emission
- **WHEN** a native approval continuation repeats the matching pending provider-owned call before its result and the consumer appends that returned history
- **THEN** both that response and the next request SHALL accept the coherent single re-emission while rejecting conflicting, completed or repeated duplicate calls

#### Scenario: History accounting
- **WHEN** bounded request history seeds deferred state
- **THEN** its entries SHALL NOT consume the current provider stream-part budget or retain history payloads
