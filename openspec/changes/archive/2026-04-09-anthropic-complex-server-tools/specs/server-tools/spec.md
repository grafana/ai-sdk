## MODIFIED Requirements

### Requirement: Provider-defined tool request building

The Anthropic provider's `convertTools()` function SHALL accept `[]provider.Tool` (interface) and use a type switch to dispatch on `provider.FunctionTool` and `provider.ProviderTool`. `provider.ProviderTool` entries SHALL be converted into the corresponding Anthropic SDK tool union variants, dispatching on the tool's `ID` field. `provider.FunctionTool` entries SHALL continue to use the existing `OfTool` path.

The `convertProviderTool` function SHALL return `(BetaToolUnionParam, []string, *provider.Warning)` where the `[]string` contains beta header strings required by the tool. The caller SHALL merge these betas into the request's beta set.

The following tool IDs SHALL be supported:
- `"anthropic.web_search_20250305"` -> `OfWebSearchTool20250305` with args: `maxUses`, `allowedDomains`, `blockedDomains`, `userLocation`
- `"anthropic.tool_search_bm25_20251119"` -> `OfToolSearchToolBm25_20251119`
- `"anthropic.tool_search_regex_20251119"` -> `OfToolSearchToolRegex20251119`
- `"anthropic.code_execution_20250522"` -> code execution tool with beta `code-execution-2025-05-22`
- `"anthropic.code_execution_20250825"` -> code execution tool with beta `code-execution-2025-08-25`
- `"anthropic.code_execution_20260120"` -> code execution tool (no beta)
- `"anthropic.computer_20241022"` -> computer tool with args and beta `computer-use-2024-10-22`
- `"anthropic.computer_20250124"` -> computer tool with args and beta `computer-use-2025-01-24`
- `"anthropic.computer_20251124"` -> computer tool with args and beta `computer-use-2025-11-24`
- `"anthropic.text_editor_20241022"` -> text editor tool with beta `computer-use-2024-10-22`
- `"anthropic.text_editor_20250124"` -> text editor tool with beta `computer-use-2025-01-24`
- `"anthropic.text_editor_20250429"` -> text editor tool with beta `computer-use-2025-01-24`
- `"anthropic.text_editor_20250728"` -> text editor tool with args (no beta)
- `"anthropic.bash_20241022"` -> bash tool with beta `computer-use-2024-10-22`
- `"anthropic.bash_20250124"` -> bash tool with beta `computer-use-2025-01-24`

Unrecognized provider tool IDs SHALL produce a warning (not an error) and be skipped.

Cache control for `FunctionTool` entries SHALL be read from `FunctionTool.ProviderOptions`. `ProviderTool` entries do not carry `ProviderOptions`; cache control for provider tools SHALL be handled via the provider-specific conversion path.

#### Scenario: Web search tool with configuration

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.web_search_20250305"` and `Args` containing `maxUses`, `allowedDomains`, and `blockedDomains`
- **THEN** it produces a `BetaToolUnionParam` with `OfWebSearchTool20250305` populated, including the `MaxUses`, `AllowedDomains`, and `BlockedDomains` fields from the args

#### Scenario: Web search tool with no configuration

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.web_search_20250305"` and empty `Args`
- **THEN** it produces a `BetaToolUnionParam` with `OfWebSearchTool20250305` populated with default/zero values

#### Scenario: Tool search BM25

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.tool_search_bm25_20251119"`
- **THEN** it produces a `BetaToolUnionParam` with `OfToolSearchToolBm25_20251119` populated

#### Scenario: Tool search regex

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.tool_search_regex_20251119"`
- **THEN** it produces a `BetaToolUnionParam` with `OfToolSearchToolRegex20251119` populated

#### Scenario: Unrecognized provider tool ID

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with an unrecognized `ID`
- **THEN** a warning is added and the tool is skipped (not included in the output)

#### Scenario: Mixed function and provider tools

- **WHEN** `convertTools()` receives a mix of `provider.FunctionTool` and `provider.ProviderTool` entries
- **THEN** both types are converted and included in the output slice

#### Scenario: Function tool InputExamples unwrapping

- **WHEN** `convertTools()` receives a `provider.FunctionTool` with `InputExamples` containing `InputExample` values
- **THEN** the Anthropic conversion SHALL unwrap the `Input` field from each `InputExample` for the Anthropic SDK's expected format

#### Scenario: hasFunctionTools uses type switch

- **WHEN** `hasFunctionTools()` checks a `[]provider.Tool` slice
- **THEN** it SHALL use a type switch on `provider.FunctionTool` (not string comparison on Type field) to determine if function tools are present

#### Scenario: Provider tool betas merged into request

- **WHEN** `convertProviderTool` returns a non-empty beta slice for a tool
- **THEN** `convertTools` SHALL merge those betas into the beta set returned to `buildParams`
- **AND** `buildParams` SHALL include them in the `Betas` field of the API request

#### Scenario: Provider tool with no beta

- **WHEN** `convertProviderTool` returns an empty or nil beta slice (e.g., for `code_execution_20260120`)
- **THEN** no additional betas are added for that tool
