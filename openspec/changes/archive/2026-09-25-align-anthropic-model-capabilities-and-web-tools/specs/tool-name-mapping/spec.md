## ADDED Requirements

### Requirement: Anthropic 20260318 web-tool aliases
The Anthropic provider's existing static provider-tool name table SHALL map provider-defined `anthropic.web_search_20260318` to the native name `web_search` and `anthropic.web_fetch_20260318` to the native name `web_fetch`. These aliases SHALL apply to named tool choice and provider tool-call/result name round trips in generate and stream paths; the caller-facing custom names SHALL remain unchanged. Function tools SHALL NOT create these provider-tool aliases.

#### Scenario: Custom search and fetch names in request choice
- **WHEN** a 20260318 provider-defined search or fetch tool has a caller-facing custom name used by a named tool choice
- **THEN** the request choice SHALL use `web_search` or `web_fetch` respectively

#### Scenario: Custom 20260318 web names in generated output
- **WHEN** a provider call or result arrives with native `web_search` or `web_fetch` name for a configured 20260318 provider-defined tool
- **THEN** the provider SHALL map that name to the configured custom name in both unary and stream output

#### Scenario: Function tool does not create a web alias
- **WHEN** the configured tools only contain function tools with web-like names
- **THEN** the mapping SHALL remain empty and native names SHALL retain identity passthrough
