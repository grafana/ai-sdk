# deferred-tool-discovery Specification

## Purpose

Define opt-in generation-local discovery of deferred tools, including bounded keyword search, next-step eligibility, contextual descriptions, caller routing, lifecycle boundaries and upstream parity evidence.

## Requirements

### Requirement: Core discovery is explicitly opted into

The root package SHALL expose `ToolSearch() Tool` and an optional `Tool.DeferLoading` flag. The zero-value flag SHALL retain ordinary loading. The search tool SHALL be an internally marked function tool whose marker survives value copies, registration under arbitrary names, and changes to its ordinary tool fields. Execution without generation binding SHALL return an error, not perform global discovery. Tools marked deferred without an available search SHALL remain undiscovered for new model-step calls. Provider-hosted search SHALL remain a separate contract; discovery SHALL NOT change tool type or locally execute provider-executed calls.

#### Scenario: Customized copied search
- **WHEN** a caller copies a ToolSearch value, changes its description, and registers it under a different name
- **THEN** the generation SHALL still bind it as the core search tool

#### Scenario: Unbound search execution
- **WHEN** a caller invokes the original ToolSearch executor outside a generation binding
- **THEN** execution SHALL return an error and SHALL NOT discover tools

#### Scenario: No opt-in
- **WHEN** a generation has no marked search or deferred entries
- **THEN** tool loading, caller preparation and active-tool behavior SHALL retain the existing non-discovery path

#### Scenario: Provider-defined discovered entry
- **WHEN** a provider-defined entry is explicitly deferred and subsequently discovered
- **THEN** its next-step definition SHALL retain its provider type and arguments
- **AND** a provider-executed call SHALL NOT run a local executor as a consequence of discovery

#### Scenario: Provider-driven call presentation before discovery
- **WHEN** a provider emits an accepted provider-executed dynamic call for a registered static deferred entry before discovery
- **THEN** its streamed input and available input SHALL retain the registered static UI classification and assemble into one tool part
- **AND** presentation classification SHALL NOT restore input validation, callbacks or local execution eligibility

### Requirement: Search has a bounded schema-only public contract

ToolSearch SHALL accept a JSON object containing only the required `query` string of minimum length one. It SHALL return an object containing only `tools`, an array of objects with required `name` and optional `description`. Search results SHALL NOT include input/output schemas, tool definitions, titles or unrelated registry data. Its ordinary model description and schemas SHALL remain stable across steps unless explicitly customized by the caller.

#### Scenario: Matching result excludes schemas
- **WHEN** search finds a deferred tool with a description and a secret input-schema property
- **THEN** the result SHALL contain its name and description but SHALL NOT contain the schema or secret property

#### Scenario: Invalid query
- **WHEN** a model supplies an empty query or additional input properties
- **THEN** ordinary tool input validation SHALL reject the input without discovering tools

### Requirement: Discovery routes are validated on the original registry

Before a provider request, orchestration SHALL validate every deferred or marked search entry against the original registry and ToolRoutes, regardless of active selection. Each named caller SHALL be a local caller with a `PrepareModelMessage` callback; provider callers and local callers lacking that callback SHALL be rejected for these entries. A marked search SHALL NOT defer loading. An omitted route SHALL allow direct discovery, while an explicit zero-value route SHALL allow no callers and SHALL remain valid. Ordinary entries SHALL retain existing caller validation.

#### Scenario: Unsupported inactive route
- **WHEN** an inactive deferred tool names a provider caller or a local caller without PrepareModelMessage
- **THEN** orchestration SHALL surface a configuration error before making a provider request

#### Scenario: Deferred search
- **WHEN** a ToolSearch value also has DeferLoading enabled
- **THEN** orchestration SHALL surface a configuration error before making a provider request

#### Scenario: Empty route is valid
- **WHEN** a deferred tool or marked search has an explicit zero-value ToolRoute
- **THEN** route validation SHALL succeed but it SHALL NOT share an eligible caller with any candidate

### Requirement: Active selection and caller overlap constrain candidates

Discovery SHALL snapshot the effective active registry after per-step overrides. An omitted active selection SHALL include all configured tools; an explicit empty selection SHALL include none. Each bound search SHALL consider only active deferred non-search entries sharing at least one eligible caller with that search. Direct access SHALL count as shared caller access; named caller access SHALL count only when that caller is active. A search SHALL NOT cross direct/local or distinct named-caller boundaries. Previously discovered tools SHALL remain eligible search candidates when active.

#### Scenario: Explicitly empty selection
- **WHEN** WithActiveTools is explicitly empty or PrepareStep returns an empty non-nil ActiveTools slice
- **THEN** no search/candidate SHALL be advertised, bound or newly executed in that model step

#### Scenario: Unlisted routes use direct access
- **WHEN** search and candidate have omitted routes and the candidate is active
- **THEN** direct discovery SHALL be eligible

#### Scenario: Caller boundaries
- **WHEN** search and candidate have no overlapping active caller, including when the sole shared named caller is inactive
- **THEN** search SHALL return no match for that candidate and SHALL NOT discover it

#### Scenario: Filtered candidate
- **WHEN** effective active selection excludes an otherwise matching deferred candidate
- **THEN** search SHALL NOT return or discover that candidate

### Requirement: Search ranking is deterministic and limited

Search SHALL tokenize names, descriptions and queries by splitting an ASCII lowercase-letter/digit followed by an ASCII uppercase-letter, lowercasing and collecting Unicode letter/number runs. Query terms SHALL be deduplicated. Each unique query term SHALL score two points for name-token membership and one for description-token membership. Only positive-score candidates SHALL match. Results SHALL sort by descending score, then ascending tool name for ties, and SHALL contain at most five matches. Exactly the returned names SHALL be discovered. Sorted-name ties SHALL be documented as the Go map-order adaptation to upstream's stable insertion-order ties.

#### Scenario: Name score and five-result limit
- **WHEN** one candidate matches a query term in its name and six match only their descriptions
- **THEN** the name match SHALL precede the description-only matches and the result SHALL contain at most five names
- **AND** positive-score matches outside the returned five SHALL remain undiscovered

#### Scenario: Tokenization and duplicate terms
- **WHEN** a query uses mixed case, repeated terms, camel-case names and Unicode letter/number words
- **THEN** matching SHALL apply the specified tokenization and each unique term SHALL contribute at most its name/description membership scores

#### Scenario: Empty token set
- **WHEN** a nonempty query contains only whitespace or punctuation
- **THEN** search SHALL return `{ "tools": [] }` and SHALL discover nothing

#### Scenario: Stable ties and repeat search
- **WHEN** equal-score candidates straddle the five-match limit and search is repeated
- **THEN** both searches SHALL return the same ascending-name tie selection, including already discovered matching entries

### Requirement: Descriptions resolve against the current runtime context

Tool SHALL preserve its static Description string and accept an optional DescriptionFunc callback receiving ToolDescriptionOptions.Context. A non-nil callback SHALL override the static string, including an empty result. Search execution SHALL resolve candidate descriptions against the captured effective runtime context for its step. Model definitions and caller catalog preparation SHALL use the same description-resolution contract for eligible tools. Resolution SHALL NOT mutate the original registry or introduce sandbox/per-tool context APIs. Search SHALL omit an absent static description and SHALL preserve the presence of an explicitly empty callback result.

#### Scenario: Context-dependent match
- **WHEN** a candidate's callback returns a capability keyword from the effective PrepareStep context and search queries that keyword
- **THEN** search SHALL match and return the resolved description
- **AND** subsequent eligible model definitions and caller catalogs SHALL resolve descriptions against their own effective step context

#### Scenario: Inactive descriptions are not evaluated
- **WHEN** an explicit active selection excludes an entry, including an empty selection without discovery controls
- **THEN** model and caller-catalog preparation SHALL NOT evaluate that entry's DescriptionFunc
- **AND** existing non-discovery execution lookup behavior SHALL remain unchanged

#### Scenario: Empty versus absent description
- **WHEN** one matching candidate has an empty static Description with no callback and another callback returns an empty string
- **THEN** search SHALL omit description for the first candidate and SHALL include an empty description for the second

### Requirement: Discoveries activate only in the next model preparation

A generation SHALL snapshot step tools before execution and SHALL exclude undiscovered deferred entries from model definitions, caller bindings/catalogs, and parsing/approval/execution lookup for newly generated local calls. Search executions SHALL update only generation discovery state, without widening the current step snapshot. Subsequent preparation SHALL admit discovered tools only when active and route-eligible. Direct discovery SHALL NOT inject new catalog messages. Stop conditions SHALL remain unchanged; search SHALL NOT force another step.

#### Scenario: Direct search with an early call
- **WHEN** one model step calls search and then attempts a static deferred tool call before the next preparation
- **THEN** search SHALL return matching names but the early call SHALL produce the existing tool error lifecycle without executing the deferred callback
- **AND** UI error classification SHALL use the original registry so that a registered static tool SHALL NOT become a dynamic tool merely because it is undiscovered
- **AND** only the next step SHALL advertise and permit an active discovered tool

#### Scenario: Same-step nested call
- **WHEN** a bound local caller invokes search and subsequently attempts to invoke a newly found callee within the same step
- **THEN** its binding and catalog SHALL remain unchanged and SHALL NOT contain that callee until the next preparation

#### Scenario: Discovered tool is deactivated and reactivated
- **WHEN** later step overrides exclude and then include a previously discovered tool within one generation
- **THEN** the excluded step SHALL hide it from definitions and execution bindings and the reactivated step SHALL restore eligibility without another search

#### Scenario: Default step limit
- **WHEN** search completes in a generation using the default one-step stop condition
- **THEN** the generation SHALL finish without automatically adding a discovery-execution step

### Requirement: Discovery state is isolated and race safe

Each StreamText generation, including GenerateText and ToolLoopAgent generate/stream entry points, SHALL own independent discovery state. Shared Tool values or ToolSet maps SHALL NOT carry discovery changes into another generation. Independent searches in one step SHALL safely union their returned names while preserving fixed step snapshots. Discovery SHALL NOT be reconstructed from prior messages or persisted in approval history.

#### Scenario: Shared registry across requests
- **WHEN** two concurrent generations share a ToolSet and only one executes a matching search
- **THEN** only that generation's later preparation SHALL include the discovered tool

#### Scenario: Concurrent sibling searches
- **WHEN** two searches in one step discover different tools concurrently
- **THEN** both names SHALL be eligible on the next step without race conditions and neither SHALL become eligible in the current step

#### Scenario: All high-level entry points
- **WHEN** equivalent direct or local-caller discovery is run through StreamText, GenerateText, Agent.Stream and Agent.Generate
- **THEN** each SHALL satisfy the same next-step activation and generation-isolation contract

### Requirement: Approvals and cancellation preserve generation boundaries

Bound search SHALL use ordinary local execution and approval rules. Pending or denied approval SHALL NOT run search or discover names; automatic approval SHALL discover only when execution runs. Deferred tools SHALL retain existing validation/approval/output behavior once eligible. Nested caller invocation SHALL remain owned by the caller callback, without a second orchestration approval layer. Canceled generations SHALL NOT continue model steps or leak discoveries to later generations; completed-search rollback SHALL NOT be guaranteed.

Historical approval resume SHALL retain original-registry execution before step binding and existing signature/policy checks. A resumed marked search SHALL remain unbound, produce the ordinary tool-output error and discover nothing. A historically approved deferred callee SHALL retain existing resume execution behavior without seeding discovery or becoming advertised in new model steps. The new-call eligibility guarantee SHALL NOT be used to reject already-approved historical executions.

#### Scenario: Pending, denied and automatically approved search
- **WHEN** ordinary approval handling leaves a bound search pending, denies it, or automatically approves it
- **THEN** pending/denied executions SHALL discover nothing and automatic approval SHALL activate executed matches only on the next preparation

#### Scenario: Resumed search approval
- **WHEN** a new generation resumes an approved historical search call
- **THEN** original-registry execution SHALL report an unbound-search tool-output error and new step definitions SHALL contain no discoveries from that call

#### Scenario: Resumed deferred callee approval
- **WHEN** a new generation resumes an approved historical deferred callee call
- **THEN** it SHALL follow existing signature/policy/execution handling and SHALL NOT seed discovery
- **AND** that callee SHALL remain absent from new step definitions until newly searched

#### Scenario: Cancellation during discovery generation
- **WHEN** a discovery-enabled generation is canceled while tools execute or before its next step
- **THEN** existing abort handling SHALL prevent further steps and a later generation using the same registry SHALL start undiscovered

### Requirement: Core and frontend evidence remain distinct from provider evidence

Discovery SHALL preserve existing UI chunk types and SSE framing. Regression coverage SHALL pair provider-independent multi-step UI chunks with captured core provider-request definitions/prompts, and SHALL validate frontend schema parsing and assembled messages against the registered frontend baseline. Synthetic core/UI fixtures SHALL NOT be presented as real recorded provider events or imported upstream provider fixtures. Evidence documentation SHALL distinguish deterministic Go tie ordering and core mock proof from unsupported live-provider acceptance claims.

#### Scenario: Cross-language discovery flow
- **WHEN** a deterministic Go discovery scenario is consumed by the pinned TypeScript frontend
- **THEN** parseJsonEventStream and uiMessageChunkSchema SHALL accept search output, early-call rejection, subsequent tool output and step/finish chunks
- **AND** assembled messages SHALL reflect those tool outcomes and final text without a new protocol discriminator

#### Scenario: Request evidence before and after discovery
- **WHEN** provider-neutral request projections are captured for an exact-baseline upstream mock scenario and the matching Go scenario
- **THEN** the evidence SHALL show undiscovered definitions absent before search, discovered schemas present only on subsequent steps, and unchanged direct prompts or stable caller definitions with next-step announcements
