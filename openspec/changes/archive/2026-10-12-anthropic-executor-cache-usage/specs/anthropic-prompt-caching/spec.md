## MODIFIED Requirements

### Requirement: Streaming cache usage metrics
The Anthropic provider SHALL expose cache usage metrics through `Usage.InputTokens.CacheRead` and `Usage.InputTokens.CacheWrite`. Streaming finish usage SHALL follow the same accounting policy as non-streaming generation. `Usage.InputTokens.Total` SHALL include uncached input, cache reads and cache writes exactly once.

#### Scenario: streaming response with cache hits
- **WHEN** a streaming response includes `cache_read_input_tokens > 0` in usage data
- **THEN** the emitted `StreamPart` usage SHALL include `InputTokens.CacheRead` populated

#### Scenario: streaming response with cache writes
- **WHEN** a streaming response includes `cache_creation_input_tokens > 0` in usage data
- **THEN** the emitted `StreamPart` usage SHALL include `InputTokens.CacheWrite` populated

#### Scenario: streaming response with no cache activity
- **WHEN** the selected usage source has no cache activity
- **THEN** the emitted `StreamPart` usage SHALL report zero for both cache fields

## ADDED Requirements

### Requirement: Executor iteration cache accounting
When usage contains one or more `message` or `compaction` iterations and no `fallback_message`, the Anthropic provider SHALL sum uncached input, output, cache reads and cache writes from those executor iterations. Missing or null iteration cache counters SHALL contribute zero. It SHALL exclude `advisor_message` iterations and SHALL NOT add top-level usage to the iteration totals.

#### Scenario: On-demand compaction with zero top-level usage
- **WHEN** all top-level counters are zero and a `compaction` iteration has 112 input tokens, 281 output tokens and 79228 cache-read tokens
- **THEN** generation and streaming finish usage SHALL report 112 uncached input, 79228 cache reads, 79340 total input and 281 output tokens

#### Scenario: Threshold compaction replaces provisional streaming usage
- **WHEN** `message_start` reports compaction cache activity and `message_delta` reports zero top-level cache activity with `compaction` and `message` iterations
- **THEN** the finish usage SHALL sum the cache counters of both executor iterations
- **AND** the earlier provisional counters SHALL NOT be added again

#### Scenario: Server-tool loop already has complete top-level cache usage
- **WHEN** the top-level cache counters equal the sum of the executor iterations
- **THEN** the normalized cache counts SHALL remain equal to those totals

#### Scenario: Advisor usage is billed separately
- **WHEN** executor and `advisor_message` iterations both contain cache activity
- **THEN** only executor cache activity SHALL contribute to normalized usage
- **AND** `Usage.Raw` SHALL retain all iterations for separate attribution

#### Scenario: Fallback or absent executor iterations retain top-level usage
- **WHEN** any `fallback_message` iteration exists, or there are no executor iterations
- **THEN** all normalized input and output counters SHALL continue to use top-level usage
