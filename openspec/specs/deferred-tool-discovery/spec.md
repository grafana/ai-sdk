# deferred-tool-discovery Specification

## Purpose

Define opt-in generation-local discovery of deferred tools, including bounded keyword search, next-step eligibility, contextual descriptions, caller routing, lifecycle boundaries and upstream parity evidence.

## Requirements

### Requirement: Core discovery is explicitly opted into

The root package SHALL expose ToolSearch() Tool and optional Tool.DeferLoading, whose zero value retains ordinary loading. Search SHALL be an internally marked function tool; the marker SHALL survive copies, arbitrary registration names and ordinary field changes. Unbound execution SHALL error without global discovery. Deferred tools without available search SHALL remain undiscovered for new model-step calls.

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

### Requirement: Core discovery does not replace provider search

Provider-hosted search SHALL remain a separate contract. Discovery SHALL NOT change tool type or locally execute provider-executed calls.

#### Scenario: Provider search remains provider-owned
- **WHEN** a deferred provider-defined tool is discovered
- **THEN** it SHALL retain its provider type, and provider-executed calls SHALL NOT become local executions.

### Requirement: Search has a bounded schema-only public contract

ToolSearch SHALL accept a JSON object containing only the required `query` string of minimum length one. It SHALL return an object containing only `tools`, an array of objects with required `name` and optional `description`. Search results SHALL NOT include input/output schemas, tool definitions, titles or unrelated registry data. Its ordinary model description and schemas SHALL remain stable across steps unless explicitly customized by the caller.

#### Scenario: Matching result excludes schemas
- **WHEN** search finds a deferred tool with a description and a secret input-schema property
- **THEN** the result SHALL contain its name and description but SHALL NOT contain the schema or secret property

#### Scenario: Invalid query
- **WHEN** a model supplies an empty query or additional input properties
- **THEN** ordinary tool input validation SHALL reject the input without discovering tools

### Requirement: Discovery routes are validated on the original registry

Before provider requests, every deferred/marked search entry SHALL be validated against the original registry and ToolRoutes regardless of active selection. Named callers SHALL be local with PrepareModelMessage; provider callers and local callers lacking it SHALL be rejected. Marked search SHALL NOT defer. Omitted routes SHALL allow direct discovery; explicit zero-value routes SHALL allow no callers and remain valid. Ordinary caller validation SHALL remain unchanged.

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

Discovery SHALL snapshot effective active registry after step overrides: omitted selection includes all tools; explicit empty includes none. Bound searches SHALL consider only active deferred non-search entries sharing at least one eligible caller with the search: direct access counts; named callers count only if active. Search SHALL NOT cross direct/local or distinct named-caller boundaries. Previously discovered tools SHALL remain candidates when active.

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

Search SHALL tokenize names/descriptions/queries by splitting ASCII lowercase-letter/digit followed by ASCII uppercase-letter, lowercasing and collecting Unicode letter/number runs. Deduplicated query terms SHALL score two points for name-token membership and one for description-token membership; only positive scores SHALL match.

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

### Requirement: Search result limits and tie ordering

Results SHALL sort by descending score, then ascending tool name for ties, and contain at most five matches. Exactly returned names SHALL be discovered. Sorted-name ties SHALL be documented as the Go map-order adaptation to upstream stable insertion-order ties.

#### Scenario: Tie ordering limits discovered names
- **WHEN** six positive-score candidates tie for five result slots
- **THEN** ascending tool name SHALL select exactly the five returned discoveries, with this ordering documented as the Go adaptation.

### Requirement: Descriptions resolve against the current runtime context

Tool SHALL retain static Description and optional DescriptionFunc receiving ToolDescriptionOptions.Context. Non-nil callbacks SHALL override static strings even with empty results. Search SHALL resolve candidates with captured effective step context; eligible model definitions and caller catalogs SHALL use the same contract. Resolution SHALL NOT mutate the registry or introduce sandbox/per-tool context APIs.

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

### Requirement: Search description presence is explicit

Search SHALL omit absent static descriptions and preserve explicitly empty callback results.

#### Scenario: Empty callback description is present
- **WHEN** a matched tool has an empty static description without a callback or a callback returning empty
- **THEN** search SHALL omit the static description but retain an explicitly empty callback description.

### Requirement: Discoveries activate only in the next model preparation

Generations SHALL snapshot step tools before execution, excluding undiscovered deferred entries from model definitions, caller bindings/catalogs and new-local-call parsing/approval/execution lookup. Search SHALL update only generation state, not widen the current snapshot. Later preparation SHALL admit discoveries only if active and route-eligible. Direct discovery SHALL NOT inject catalog messages. Stop conditions SHALL remain unchanged; search SHALL NOT force another step.

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

Bound search SHALL use ordinary local execution/approval rules: pending or denied approval SHALL NOT execute or discover; automatic approval SHALL discover only when executed. Eligible deferred tools SHALL retain validation/approval/output behavior. Nested invocation SHALL remain caller-callback-owned without a second approval layer. Canceled generations SHALL NOT continue steps or leak discoveries; completed-search rollback SHALL NOT be guaranteed.

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

### Requirement: Historical discovery approval resume uses the original registry

Historical approval resume SHALL retain original-registry execution before step binding and signature/policy checks. Resumed marked search SHALL remain unbound, emit ordinary tool-output error and discover nothing. Historically approved deferred callees SHALL retain resume execution without seeding discovery or new-step advertising. New-call eligibility SHALL NOT reject already-approved historical executions.

#### Scenario: Historical approved execution does not advertise a tool
- **WHEN** an approved historical deferred callee resumes in a new generation
- **THEN** original-registry execution and signature/policy checks SHALL apply without seeding discovery or advertising it in new steps.

### Requirement: Core and frontend evidence remain distinct from provider evidence

Discovery SHALL preserve UI chunk types/SSE framing. Coverage SHALL pair provider-independent multi-step UI chunks with captured core request definitions/prompts and validate frontend schema parsing/assembly against the registered baseline. Synthetic core/UI fixtures SHALL NOT be called real recorded events or imported upstream provider fixtures. Evidence SHALL distinguish Go tie ordering/core mocks from unsupported live-provider acceptance claims.

#### Scenario: Cross-language discovery flow
- **WHEN** a deterministic Go discovery scenario is consumed by the pinned TypeScript frontend
- **THEN** parseJsonEventStream and uiMessageChunkSchema SHALL accept search output, early-call rejection, subsequent tool output and step/finish chunks
- **AND** assembled messages SHALL reflect those tool outcomes and final text without a new protocol discriminator

#### Scenario: Request evidence before and after discovery
- **WHEN** provider-neutral request projections are captured for an exact-baseline upstream mock scenario and the matching Go scenario
- **THEN** the evidence SHALL show undiscovered definitions absent before search, discovered schemas present only on subsequent steps, and unchanged direct prompts or stable caller definitions with next-step announcements
