## MODIFIED Requirements

### Requirement: Unary text and scalar mapping

The handler SHALL preserve ordered system messages and user or assistant text parts, including required empty strings. It SHALL map optional integer and continuous generation controls with ordinary Go JSON numeric range checks, preserve explicit zero values, preserve stop-sequence order, and map typed reasoning values. Omitted reasoning and wire `provider-default` SHALL both map to zero-valued `ReasoningProviderDefault`.

For an otherwise supported text request with absent or empty tools, the handler SHALL accept either an omitted tool choice or schema-valid `{"type":"auto"}`. Omission SHALL leave `provider.CallOptions.ToolChoice` nil; explicit auto SHALL be forwarded as `provider.ToolChoiceAuto`, not silently removed. This admission SHALL NOT bypass any other supported-subset check.

#### Scenario: Supported request
- **WHEN** a schema-valid unary request contains text messages and supported scalar controls
- **THEN** the model SHALL receive the same text order, scalar presence, zero values, stop-sequence order, and reasoning value

#### Scenario: Integer lexical form
- **WHEN** an integer control uses a plain integer token such as `1`, `0`, or `-1`
- **THEN** it SHALL map to that Go integer
- **WHEN** it uses `1.0`, `1e0`, `-0.0`, or exceeds the Go integer range
- **THEN** mapping SHALL return an invalid request before resolution or invocation

#### Scenario: Empty optional values
- **WHEN** tools, headers, and provider-options namespaces are empty, raw chunks are false, and response format is text
- **THEN** those values SHALL normalize to the supported ordinary text behavior

#### Scenario: Automatic choice without tools
- **WHEN** an otherwise supported unary request has `toolChoice: {"type":"auto"}` and its tools field is absent or an empty array
- **THEN** catalog resolution and `DoGenerate` SHALL each run exactly once
- **AND** `DoGenerate` SHALL receive no tools and a non-nil automatic choice
- **AND** `DoStream` SHALL not run

#### Scenario: Low-level omitted choice remains omitted
- **WHEN** an otherwise supported unary request omits tool choice and has absent or empty tools
- **THEN** `DoGenerate` SHALL receive nil `ToolChoice`

### Requirement: Unsupported capability families

Schema-valid files, reasoning content, custom content, tools, tool approvals, structured output, non-empty provider options, body headers, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. The tools family SHALL include nonempty tool declarations, tool-call/result content, and explicit none, required, or named choices even when tools are absent. A schema-valid automatic choice with no tools SHALL NOT by itself activate the tools family. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

#### Scenario: Explicit non-automatic choice without tools
- **WHEN** a request supplies none, required, or a named-tool choice with tools absent or empty
- **THEN** the handler SHALL return the fixed unsupported-tools response before resolution or model invocation

#### Scenario: Automatic choice does not enable tools
- **WHEN** a request supplies automatic choice and nonempty function or provider-tool declarations, or tool-call/result prompt content
- **THEN** the handler SHALL return the fixed unsupported-tools response before resolution or model invocation

#### Scenario: Automatic choice does not enable other unsupported families
- **WHEN** a request supplies automatic choice with empty tools and one other unsupported family such as approvals, body headers, provider options, or raw output
- **THEN** the handler SHALL return that family's existing fixed response before resolution or model invocation

#### Scenario: Invalid automatic choice remains a schema error
- **WHEN** tool choice is null, has an unknown discriminator, has an invalid shape, or adds a field to the closed automatic-choice object
- **THEN** complete-schema validation SHALL fail before supported-subset mapping, catalog resolution, or model invocation

### Requirement: Compatibility evidence

The runtime SHALL replay every committed ProviderWire request golden without modifying it. Cross-language integration tests SHALL call the production handler through the exact registered `@ai-sdk/gateway` version and verify minimal unary success, streaming text through clean EOF, representative errors, and cancellation. Raw Go tests SHALL remain authoritative for exact documents, privacy, sequencing, lifecycle, and byte bounds.

Equivalent high-level Go `StreamText` and registered TypeScript `streamText` calls with no tools or explicit choice SHALL additionally exercise actual Gateway HTTP requests. Tests SHALL observe each unmodified request's automatic choice and successful provider execution rather than reconstructing low-level options. The Go probe SHALL use the changed local core. Completion of this fix SHALL require focused core, HTTP, and fallback regressions, actual high-level cross-client command evidence, and passing direct conformance. The paired `anthropic/upstream/text-generation` replay SHALL be tracked as pending #201 integration follow-up rather than a prerequisite to shipping this fix. Provider inputs SHALL retain their recorded or pinned-upstream provenance.

#### Scenario: Registered client success
- **WHEN** the pinned Gateway client sends a supported unary request
- **THEN** it SHALL consume content, finish reason, and usage from the production handler and supply its own warnings/request/response fields

#### Scenario: Streaming request
- **WHEN** the registered client sends a supported streaming text request
- **THEN** the handler SHALL invoke `DoStream` once and the client SHALL consume the strict stream through clean EOF

#### Scenario: Equivalent high-level streaming defaults
- **WHEN** Go `StreamText` and registered TypeScript `streamText` independently send equivalent ordinary text calls with tools and choice omitted
- **THEN** each captured Gateway request SHALL contain `toolChoice: {"type":"auto"}`
- **AND** each call SHALL reach the provider and complete with the expected text
- **AND** harmless existing presence differences SHALL not be hidden by rewriting requests

#### Scenario: Deferred paired fixture verifies integration
- **WHEN** #201 integrates this fix and both Gateway conformance executors replay `anthropic/upstream/text-generation`
- **THEN** both SHALL reach the backend and pass the existing UI and backend-request expectations
- **AND** neither the harness nor fixture goldens SHALL strip automatic choice or be changed to force matching counts
- **AND** direct conformance SHALL remain passing without fabricated provider recordings

#### Scenario: Fix ships before the paired harness
- **WHEN** the core/HTTP/fallback regressions, actual high-level cross-client command tests, and direct conformance pass before #201 is integrated
- **THEN** this fix MAY ship without waiting for that harness
- **AND** validation SHALL explicitly report paired and broader matrix replay as pending #201 follow-up, not executed or passed
