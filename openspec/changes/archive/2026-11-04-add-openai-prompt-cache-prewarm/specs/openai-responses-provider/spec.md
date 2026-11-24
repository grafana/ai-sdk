## ADDED Requirements

### Requirement: Prompt-cache TTL and prewarming
The provider SHALL expose typed prompt-cache TTL values for 30m and 12h, optional request prewarm, and optional breakpoint TTL. Unsupported nonempty request TTLs SHALL fail before HTTP. Breakpoint TTLs SHALL retain upstream passthrough behavior. Shared adapters, including Mantle, SHALL use the same conversion and validation.

#### Scenario: Go zero-value TTL omission
- **WHEN** TTL is the Go zero value, including empty/null TTL decoded from raw JSON
- **THEN** it is omitted from the request
- **AND** this intentional Go decoding boundary does not claim upstream strict raw-option parsing parity

#### Scenario: Request TTL and explicit prewarm
- **WHEN** generate or stream is called with a supported TTL and prewarm true or false
- **THEN** prompt_cache_options preserves those values, including explicit false
- **AND** omitted options remain omitted

#### Scenario: Unsupported request TTL
- **WHEN** generate or stream receives promptCacheOptions.ttl of 24h
- **THEN** it returns a validation error before any HTTP request

#### Scenario: Per-part breakpoint TTL
- **WHEN** supported system, user text/image/file or tool-result text/image/file content carries a 12h breakpoint
- **THEN** its prompt_cache_breakpoint contains ttl 12h at the existing breakpoint location

#### Scenario: Empty prewarm response
- **WHEN** a completed provider DoGenerate or DoStream prewarm response has no output and reports cache writes
- **THEN** it finishes successfully without content
- **AND** normalized usage and raw cache_write_tokens_details are retained
