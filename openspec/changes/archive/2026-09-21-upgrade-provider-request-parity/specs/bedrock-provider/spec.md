## ADDED Requirements

### Requirement: Bedrock-compatible document names
For user input and tool-result documents, including inline text documents, the provider SHALL derive names with the same sanitation and fallback behavior as `@ai-sdk/amazon-bedrock@5.0.87`. Conversion SHALL strip filename extension segments starting at the first dot, collapse ECMAScript whitespace to ASCII spaces, remove characters other than ASCII letters/digits, spaces, hyphens, parentheses and square brackets, trim, truncate to 200 characters, and trim again, in that order. Missing or empty sanitized names SHALL use a request-wide `document-N` counter starting at 1; only fallback allocation SHALL advance the counter. Conversion SHALL NOT alter document bytes, format, citation settings, or caller filenames.

#### Scenario: Invalid punctuation and Unicode
- **WHEN** filenames include `John's report.txt`, `invoice #123.txt`, `résumé.txt`, and `report (final) [v2]_draft.txt`
- **THEN** document names SHALL be `Johns report`, `invoice 123`, `rsum`, and `report (final) [v2]draft` respectively

#### Scenario: Whitespace and length normalization
- **WHEN** filenames contain repeated whitespace, tabs, or more than 200 allowed characters before the extension
- **THEN** whitespace runs SHALL collapse before invalid-character removal and names SHALL be trimmed and truncated according to the target order
- **AND** an already-valid filename-derived name SHALL remain unchanged

#### Scenario: Fallbacks across input and tool results
- **WHEN** input and tool-result filenames are missing, extension-only, whitespace-only, or sanitize to empty
- **THEN** their names SHALL be `document-1`, `document-2`, and subsequent values in request conversion order
- **AND** named documents between them SHALL NOT consume counter values

#### Scenario: Tool-result and text-document parity
- **WHEN** the same invalid filename is used in an input byte document, an inline text document, and a tool-result document
- **THEN** all three SHALL use the same sanitized name
- **AND** their respective content encoding and format SHALL remain unchanged

### Requirement: Bedrock strict-tool schema compatibility
For function tools on models supporting strict mode, the provider SHALL forward `strict: true` only when the schema is compatible with the recursive closed-object check in `@ai-sdk/amazon-bedrock@5.0.87`. Every visited schema declaring type `object`, including type arrays containing `object`, SHALL have `additionalProperties: false`. The check SHALL visit properties, patternProperties, definitions/$defs, schema-valued dependencies, propertyNames, contains, not, if/then/else, single/tuple items, and allOf/anyOf/oneOf. Boolean schemas SHALL be compatible and ordinary instance values SHALL NOT be interpreted as schemas. The check SHALL NOT introduce external-reference resolution.

An incompatible strict schema SHALL retain its original input schema, omit `strict`, and emit an unsupported warning with feature `strict` identifying the tool and the requirement for `additionalProperties: false`. An unsupported model SHALL retain its existing model-specific strict warning instead. `strict: false` SHALL pass through on supported models without the closed-object check, and an absent strict value SHALL remain absent. The provider SHALL NOT rewrite schemas to force compatibility.

#### Scenario: Top-level open object
- **WHEN** a supported model receives `strict: true` and an object schema with absent, true, or schema-valued `additionalProperties`
- **THEN** the request SHALL omit strict, preserve the schema, and emit the schema-compatibility warning

#### Scenario: Nested or referenced open object
- **WHEN** the top-level object is closed but a nested schema, composition branch, array item, or definition in `$defs` is open
- **THEN** strict SHALL be omitted with the schema-compatibility warning
- **AND** a `$ref` into a local definition SHALL NOT prevent that definition from being inspected

#### Scenario: Closed nested schemas
- **WHEN** every visited object is closed, including objects in union types, definitions, arrays, and conditionals
- **THEN** `strict: true` SHALL be preserved without a compatibility warning
- **AND** boolean subschemas and property-dependency name arrays SHALL NOT be treated as open objects

#### Scenario: False and absent strict controls
- **WHEN** a supported model receives an open schema with strict false or absent
- **THEN** false SHALL remain false and absence SHALL remain absence, without a strict-schema warning

#### Scenario: Unsupported model warning precedence
- **WHEN** a model that rejects strict receives either explicit strict value, including true with an open schema
- **THEN** strict SHALL be omitted with the existing model-specific warning
- **AND** conversion SHALL NOT add a second schema-compatibility warning

#### Scenario: Input immutability
- **WHEN** strict compatibility is evaluated for a schema containing instance objects in defaults or examples
- **THEN** those values SHALL NOT affect compatibility
- **AND** neither the supplied schema nor tool definition SHALL be mutated

### Requirement: Model-specific OpenAI reasoning on Bedrock
For existing root reasoning and provider `reasoningConfig` settings, Bedrock request conversion SHALL recognize OpenAI model IDs of the form `openai.<model>` or a single dot-delimited prefix followed by `openai.<model>`. Other IDs merely containing `openai.` SHALL NOT be classified as OpenAI models. GPT-OSS models SHALL use flat `additionalModelRequestFields.reasoning_effort`; other recognized OpenAI models SHALL use `additionalModelRequestFields.reasoning.effort`, preserving unrelated fields in the caller's nested reasoning object. This behavior SHALL apply equally to Converse and ConverseStream, without introducing a model-family constructor option.

#### Scenario: GPT-OSS flat reasoning
- **WHEN** a native or single-prefix GPT-OSS model receives an effective reasoning effort
- **THEN** the request SHALL encode the effort as `reasoning_effort`
- **AND** it SHALL NOT generate nested `reasoning` or other-provider `reasoningConfig` for that effort

#### Scenario: Cross-region OpenAI nested reasoning
- **WHEN** `us.openai.gpt-5.6-luna`, `global.openai.gpt-5.6-luna`, or its unprefixed form receives effort `medium`
- **THEN** the request SHALL encode `reasoning: {effort: "medium"}` under additional model fields
- **AND** it SHALL preserve unrelated existing nested reasoning fields rather than replacing the whole object

#### Scenario: Custom substring is not a model family
- **WHEN** `custom-openai.gpt-5.6-luna` receives an effective reasoning effort
- **THEN** conversion SHALL retain the other-provider `reasoningConfig.maxReasoningEffort` shape rather than OpenAI effort shapes

#### Scenario: Existing non-OpenAI controls
- **WHEN** the call targets Anthropic or another Bedrock provider, or no effective OpenAI effort is supplied
- **THEN** this correction SHALL NOT change the existing reasoning request behavior

### Requirement: Budget-based Anthropic application profile inference
Bedrock SHALL treat application-inference-profile ARNs as Anthropic requests when the selected provider namespace supplies a non-null reasoning budget, matching the target's presence check rather than testing for a positive value. Raw explicit zero SHALL remain distinguishable from absence internally, and explicitly null budget options SHALL fail before HTTP as required by the target option schema; the existing typed Go zero-as-omitted contract SHALL remain unchanged without migrating public fields to pointers. Modern namespace precedence and legacy fallback SHALL be preserved.

Classification SHALL follow the target stages: the explicit budget informs initial reasoning resolution and request settings, and the effective budget informs tool preparation. Relevant reasoning, thinking serialization, max-token adjustment, sampling warnings, structured-output routing, tool conversion, non-Anthropic effort exclusion, and additional response-field paths SHALL use request-aware classification. Model identity SHALL remain unchanged. This requirement SHALL NOT introduce an explicit model-family override.

#### Scenario: Opaque profile with explicit thinking budget
- **WHEN** an opaque application profile ARN receives enabled thinking with budget 1024 through the active namespace
- **THEN** generate and stream requests SHALL serialize Anthropic thinking, adjust max tokens, suppress unsupported sampling, and include the Anthropic additional response-field paths
- **AND** the request SHALL NOT emit a non-Anthropic budget warning or a Nova reasoning shape

#### Scenario: Budget presence and namespace precedence
- **WHEN** raw options distinguish budget absent, null, and explicit zero, or modern and legacy namespaces provide conflicting budgets
- **THEN** a null budget SHALL be rejected by provider-option validation, while valid options SHALL follow the target non-null presence check in the selected namespace
- **AND** a non-null modern namespace SHALL NOT inherit the legacy budget
- **AND** typed zero without raw presence SHALL retain its existing omission behavior

#### Scenario: Root reasoning overrides
- **WHEN** root reasoning overrides or supplements explicit profile reasoning options
- **THEN** derived and cleared thinking, effort, sampling, tool configuration, and response-field paths SHALL follow the target's classification and resolution order
- **AND** caller options SHALL remain unchanged

#### Scenario: Profile without a budget
- **WHEN** an opaque profile has no non-null budget and no recognized Anthropic model identity
- **THEN** this inference SHALL NOT manufacture an Anthropic family or native-output capability

### Requirement: Default JSON-tool routing for unreliable native-output models
For Bedrock Sonnet 4.6 and Haiku 4.5, JSON responses with a schema SHALL default to the synthetic JSON-tool route rather than native output_config.format, including when thinking is enabled. Strict-tool support SHALL remain independent of this routing decision. Existing JSON-tool response normalization SHALL produce text, stop finish reason for a completed synthetic tool, and the existing isJsonResponseFromTool metadata in both generate and stream operations.

#### Scenario: Actual-model generate and stream fallback
- **WHEN** Sonnet 4.6 or Haiku 4.5 receives a JSON response schema through generate or stream
- **THEN** the request SHALL contain the synthetic json tool and effective required choice without native output_config.format
- **AND** a synthetic json response SHALL become JSON text with the existing stop/metadata semantics, not a user-executable tool call

#### Scenario: Thinking and strict are independent controls
- **WHEN** one of these models receives thinking and a compatible strict user tool alongside the response schema
- **THEN** thinking and strict support SHALL remain intact while output uses the JSON-tool route

#### Scenario: Other model controls
- **WHEN** another model receives the same response schema
- **THEN** its existing native, instruction, or synthetic-tool default SHALL remain unchanged by this correction

### Requirement: Bedrock forwarding of existing Anthropic parallel-tool control
For Anthropic Bedrock requests, including budget-inferred profiles, the provider SHALL honor the existing anthropic.disableParallelToolUse option using the target's additionalModelRequestFields.tool_choice representation. Eligible function-tool requests with true SHALL carry disable_parallel_tool_use=true and type auto, any, or tool according to the effective choice, and SHALL omit duplicate Converse toolConfig.toolChoice. Existing Anthropic provider-tool preparation SHALL receive the option as well. Bedrock SHALL NOT depend on the separate Anthropic Go module to decode this option.

The synthetic JSON tool SHALL be added before shared tool preparation, and its effective required choice SHALL participate in parallel-tool disabling. False/unset, no-tool, none-choice, and non-Anthropic requests SHALL retain their target behavior without manufacturing an Anthropic choice.

#### Scenario: Effective function-tool choice
- **WHEN** an eligible Anthropic request disables parallel tool use with absent, auto, required, or named tool choice
- **THEN** additional model fields SHALL contain respectively auto, auto, any, or tool with the selected name, plus disable_parallel_tool_use=true
- **AND** Converse toolConfig SHALL NOT contain a duplicate toolChoice

#### Scenario: Synthetic and mixed tools
- **WHEN** JSON-tool fallback is active with parallel tool use disabled, with or without additional user tools
- **THEN** shared preparation SHALL see the synthetic tool and effective required choice
- **AND** the request SHALL contain one Anthropic any choice with parallel use disabled, retaining the target tool declarations

#### Scenario: Inactive and non-Anthropic controls
- **WHEN** the flag is false/unset, tools are absent, choice is none, or the model is non-Anthropic
- **THEN** conversion SHALL NOT introduce an ineligible parallel-disabled Anthropic choice
