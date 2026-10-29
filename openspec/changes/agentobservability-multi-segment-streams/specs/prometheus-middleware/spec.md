## MODIFIED Requirements

### Requirement: Token usage metrics

`aisdk_model_tokens_total` SHALL increment only for positive provider usage counts. Generate SHALL use `provider.GenerateResult.Usage`; stream SHALL observe every part with non-nil `Usage` and use shared streaming usage aggregation, which takes the last finish's segment usage and, before any finish, retains the greatest normalized values. `Usage.Raw` SHALL NOT be a metric label. Token types SHALL use only the specified normalized field mappings.

#### Scenario: Stream usage preserves strongest values

- **GIVEN** usage is split across multiple stream parts
- **AND** the finish part omits some normalized counters
- **WHEN** the stream completes
- **THEN** token counters SHALL increment from the counters the finish reports and, for the counters it omits, the strongest values observed before it

#### Scenario: Stream usage follows the last finish

- **GIVEN** a stream emits several finish parts, each reporting its own usage
- **WHEN** the stream completes
- **THEN** token counters SHALL increment once from the last finish's segment usage, not a sum or the greatest value across segments

#### Scenario: Positive usage increments fixed token types

- **WHEN** a successful generate or stream call reports positive input and output usage fields
- **THEN** `aisdk_model_tokens_total` SHALL increment for the corresponding fixed `token_type` labels

#### Scenario: Missing or zero usage is ignored

- **WHEN** a usage field is nil, zero, or negative
- **THEN** the middleware SHALL NOT increment `aisdk_model_tokens_total` for that field

#### Scenario: Normalized usage fields select fixed token labels
- **WHEN** positive normalized token fields are recorded
- **THEN** `token_type` SHALL be limited to `input` from `Usage.InputTokens.Total`, `input_no_cache` from `Usage.InputTokens.NoCache`, `input_cache_read` from `Usage.InputTokens.CacheRead`, and `input_cache_write` from `Usage.InputTokens.CacheWrite`
- **AND** output labels SHALL be `output` from `Usage.OutputTokens.Total`, `output_text` from `Usage.OutputTokens.Text`, and `output_reasoning` from `Usage.OutputTokens.Reasoning`
