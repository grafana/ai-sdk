## MODIFIED Requirements

### Requirement: Non-streaming response conversion
`DoGenerate` SHALL convert every Responses output item to provider content:
`message` text (with annotations -> `source` parts), `reasoning` summaries,
`function_call` / `custom_tool_call` tool-calls, and provider-executed built-in
calls (`web_search_call`, `file_search_call`, `code_interpreter_call`,
`image_generation_call`, `local_shell_call`, `shell_call` + `shell_call_output`,
`apply_patch_call`, `tool_search_call` + `tool_search_output`, `computer_call`,
`mcp_call`, `mcp_approval_request`, `compaction`). Shell and local-shell call
inputs SHALL preserve optional execution constraints while translating API
snake_case fields to the provider content model's camelCase fields so later
turns can reconstruct equivalent request items. The conversion SHALL map usage
and finish reason, set provider metadata (`responseId`, logprobs,
`serviceTier`), and carry warnings. Annotation-derived `source` parts SHALL carry
their display text in the canonical `Title` field: a URL citation's title, and
a document's filename, or its file id when the annotation carries no filename.
The legacy `Text` field SHALL NOT carry a source title. When logprobs were requested and an output-text
content part returns a non-null logprobs array, including an empty array,
`ProviderMetadata["openai"].logprobs` SHALL contain one outer entry for that
content part in response order. Each entry SHALL preserve token order and contain
`token`, `logprob`, and `top_logprobs` alternatives with `token` and `logprob`,
without provider-only byte arrays. Null or missing arrays SHALL NOT add outer
entries, and unrequested logprobs SHALL NOT add a `logprobs` metadata field.

#### Scenario: Generated output logprobs metadata
- **WHEN** logprobs are requested and output-text content returns token logprobs with top alternatives
- **THEN** `ProviderMetadata["openai"].logprobs` contains the content-part arrays in response order
- **AND** each token and top alternative preserves its `token` and `logprob` fields in order

#### Scenario: Generated empty logprobs array is retained
- **WHEN** logprobs are requested and output-text content returns an empty logprobs array
- **THEN** `ProviderMetadata["openai"].logprobs` contains an empty array entry for that content part

#### Scenario: Generated logprobs metadata is omitted
- **WHEN** logprobs were not requested or every output-text logprobs field is null or missing
- **THEN** `ProviderMetadata["openai"]` does not contain a `logprobs` field

#### Scenario: Text and url citation
- **WHEN** the response contains a `message` item with text and a `url_citation` annotation
- **THEN** the result contains a text content part and a `source` content part of type `url` whose `Title` is the annotation title and whose `Text` is empty

#### Scenario: Document annotations carry display titles
- **WHEN** the response contains `file_citation`, `container_file_citation` or `file_path` annotations
- **THEN** each `source` content part is of type `document` with `Title` and `Filename` taken from the annotation filename, or from the file id for `file_path`, alongside its media type and provider metadata
- **AND** no `source` content part carries its title in the `Text` field

#### Scenario: Provider-executed web search
- **WHEN** the response contains a `web_search_call` item
- **THEN** the result contains a tool-call and a tool-result content part with `ProviderExecuted` set

#### Scenario: MCP approval request
- **WHEN** the response contains an `mcp_approval_request` item
- **THEN** the result contains a provider-executed dynamic tool-call and a `tool-approval-request` content part

#### Scenario: Shell execution options survive response conversion
- **WHEN** a shell or local-shell response call contains timeout, output-length, user, working-directory, or environment fields
- **THEN** the generated tool-call input preserves every present field in provider content naming
- **AND** converting that content on a later turn produces equivalent Responses request fields

#### Scenario: Finish reason with function call
- **WHEN** the response has no incomplete reason but contains a function call
- **THEN** the unified finish reason is `tool-calls`

#### Scenario: Client-executed computer call
- **WHEN** a `computer_call` contains `call_id`
- **THEN** the result contains a client-executed `computer` tool call keyed by `call_id`, with mapped actions, safety checks, status, and the response item id in provider metadata

#### Scenario: Legacy computer call
- **WHEN** a `computer_call` does not contain `call_id`
- **THEN** the result preserves the provider-executed `computer_use` call and status result

#### Scenario: Missing Responses output
- **WHEN** a successful non-streaming response omits or nulls `output`
- **THEN** the provider returns a non-retryable status-500 `APICallError` naming any incomplete reason
