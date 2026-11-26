## MODIFIED Requirements

### Requirement: Core discovery is explicitly opted into

The root package SHALL expose ToolSearch(opts ...ToolSearchOption) Tool and optional Tool.DeferLoading, whose zero value retains ordinary loading. Search SHALL be an internally marked function tool; the marker SHALL survive copies, arbitrary registration names and ordinary field changes. Unbound execution SHALL error without global discovery. Deferred tools without available search SHALL remain undiscovered for new model-step calls.

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

### Requirement: Search ranking is deterministic and limited

The default keyword ranker SHALL tokenize names/descriptions/queries by splitting ASCII lowercase-letter/digit followed by ASCII uppercase-letter, lowercasing and collecting Unicode letter/number runs. Deduplicated query terms SHALL score two points for name-token membership and one for description-token membership; only positive scores SHALL match.

#### Scenario: Name score and five-result limit
- **WHEN** search uses the default keyword ranker and limit, and one candidate matches a query term in its name while six match only their descriptions
- **THEN** the name match SHALL precede the description-only matches and the result SHALL contain at most five names
- **AND** positive-score matches outside the returned five SHALL remain undiscovered

#### Scenario: Tokenization and duplicate terms
- **WHEN** the default keyword ranker processes a query with mixed case, repeated terms, camel-case names and Unicode letter/number words
- **THEN** matching SHALL apply the specified tokenization and each unique term SHALL contribute at most its name/description membership scores

#### Scenario: Empty token set
- **WHEN** the default keyword ranker receives a nonempty query containing only whitespace or punctuation
- **THEN** search SHALL return `{ "tools": [] }` and SHALL discover nothing

#### Scenario: Stable ties and repeat search
- **WHEN** equal-score candidates straddle the default keyword ranker's result limit and search is repeated
- **THEN** both searches SHALL return the same ascending-name tie selection, including already discovered matching entries

### Requirement: Search result limits and tie ordering

The SDK SHALL use a default limit of five matches per search. An explicit limit
SHALL be a positive Go `int` and SHALL be validated on the original registry
before model I/O even for inactive search tools. Default keyword results SHALL
sort by descending score, then ascending tool name for ties. Sorted-name ties
are the Go map-order adaptation to upstream stable insertion-order ties.
Exactly returned names SHALL be discovered.

#### Scenario: Tie ordering limits discovered names
- **WHEN** six positive-score candidates tie for five result slots using the default keyword ranker and limit
- **THEN** ascending tool name SHALL select exactly the five returned discoveries, with this ordering documented as the Go adaptation.

#### Scenario: Different caps on copied and renamed tools
- **WHEN** a generation registers search tools with different positive limits, including copied tools under new names
- **THEN** each search SHALL enforce its own cap without changing the other search's options or results.

#### Scenario: Non-positive configured limit
- **WHEN** a registered search has an explicit limit of zero or less, including when it is not active
- **THEN** the SDK SHALL return a configuration error without invoking the model.

### Requirement: Descriptions resolve against the current runtime context

Tool SHALL retain static Description and optional DescriptionFunc receiving ToolDescriptionOptions.Context. Non-nil callbacks SHALL override static strings even with empty results. Search SHALL resolve candidates with captured effective step context; eligible model definitions and caller catalogs SHALL use the same contract. Resolution SHALL NOT mutate the registry or introduce sandbox/per-tool context APIs.

#### Scenario: Context-dependent match
- **WHEN** a candidate's callback returns a capability keyword from the effective PrepareStep context and the default keyword ranker searches for that keyword
- **THEN** search SHALL match and return the resolved description
- **AND** subsequent eligible model definitions and caller catalogs SHALL resolve descriptions against their own effective step context

#### Scenario: Inactive descriptions are not evaluated
- **WHEN** an explicit active selection excludes an entry, including an empty selection without discovery controls
- **THEN** model and caller-catalog preparation SHALL NOT evaluate that entry's DescriptionFunc
- **AND** a new model call to the excluded entry SHALL be rejected as unavailable without execution

#### Scenario: Empty versus absent description
- **WHEN** one matching candidate has an empty static Description with no callback and another callback returns an empty string
- **THEN** search SHALL omit description for the first candidate and SHALL include an empty description for the second

## ADDED Requirements

### Requirement: Search supports caller-defined ranking

A configured ranker SHALL receive the execution context, unmodified query and
detached name-sorted catalog of eligible deferred candidates. Each entry SHALL
contain only its name and resolved optional description. The SDK SHALL filter
unknown, ineligible and duplicate names before limiting results in rank order.
Returning no names SHALL NOT fall back to keyword search. Result metadata
SHALL come from the original catalog.

#### Scenario: Ranker returns more names than the cap
- **WHEN** a ranker returns duplicates, ineligible names and more eligible names than the limit
- **THEN** only the first eligible unique names up to the limit SHALL appear in output or activate on later steps.

#### Scenario: Ranker mutates its catalog copy
- **WHEN** a ranker changes a candidate name or description in its catalog
- **THEN** the original eligibility and descriptions SHALL still determine output and discovery.

### Requirement: Failed and canceled searches do not discover tools

A ranker error SHALL use the ordinary tool execution error path and SHALL NOT
discover any tools from that invocation. If the SDK observes cancellation
before committing matches, the invocation SHALL NOT discover any tools. This
does not require rolling back completed searches.

#### Scenario: Ranker fails or is canceled
- **WHEN** a ranker returns an error with names or cancellation is observed before matches are committed
- **THEN** that invocation SHALL NOT publish any discoveries.
