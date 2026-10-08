## ADDED Requirements

### Requirement: Optional streaming execution overview and current failures

Streaming SHALL use the existing reader/commitment/drain ownership to optionally enrich finish metadata and classified error payloads. First-part selection SHALL NOT claim completion. Current protected native summaries SHALL remain event-local; finish SHALL NOT duplicate them. Existing complete-frame limits, original metadata preflight, error ordering, active-block state, authoritative finish and writer failure behavior SHALL remain unchanged. Optional enrichment failure SHALL preserve the original fitting frame, including native namespaces that cannot be relocated.

#### Scenario: Selected leading error followed by content
- **WHEN** the first provider part is an error followed by valid text and finish
- **THEN** no later candidate SHALL run, both clients SHALL consume all parts in order and finish SHALL contain only the optional compact execution overview rather than stream-error history

#### Scenario: Finish fits only without overview
- **WHEN** original finish encoding fits but enriched finish does not
- **THEN** the original finish SHALL remain authoritative without a new terminal error or read-ahead
