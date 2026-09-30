## ADDED Requirements

### Requirement: Versioned Anthropic web-tool request projection
The Anthropic provider SHALL accept provider-defined IDs `anthropic.web_search_20260318` and `anthropic.web_fetch_20260318` in the same tool conversion path as previously supported web versions. Each accepted web tool SHALL emit the matching dated native tool type and fixed wire name (`web_search` or `web_fetch`) in both generate and stream requests. Only declared fields SHALL be projected from camelCase arguments to snake_case wire fields:
- Search 20250305 and 20260209: `maxUses`, `allowedDomains`, `blockedDomains`, `userLocation`.
- Search 20260318: the same fields plus `responseInclusion` (`full` or `excluded`).
- Fetch 20250910 and 20260209: `maxUses`, `allowedDomains`, `blockedDomains`, `citations.enabled`, `maxContentTokens`.
- Fetch 20260318: the same fields plus `useCache` (including explicit `false`) and `responseInclusion` (`full` or `excluded`).

Web fetch 20250910 SHALL select beta `web-fetch-2025-09-10`. Web search/fetch 20260209 SHALL select beta `code-execution-web-tools-2026-02-09`. Web search/fetch 20260318 and search 20250305 SHALL select no version-specific beta; any betas from other tools or explicit Anthropic options SHALL still be merged and deduplicated as before.

#### Scenario: Every supported dated web variant
- **WHEN** a request declares any of the six supported web search/fetch IDs with valid arguments
- **THEN** exactly that versioned native type and its declared fields SHALL appear in the request body, together with the specified version-specific beta if any

#### Scenario: Fetch 20260318 optional fields
- **WHEN** `anthropic.web_fetch_20260318` has `useCache: false`, `responseInclusion: "excluded"`, citations and other valid fetch fields
- **THEN** the native request SHALL include `use_cache: false`, `response_inclusion: "excluded"`, the projected citations and other fields, and SHALL NOT add a beta for that web version

#### Scenario: Search 20260318 optional fields
- **WHEN** `anthropic.web_search_20260318` has `responseInclusion: "full"`, a valid approximate user location and other search fields
- **THEN** the native request SHALL include `response_inclusion: "full"`, the projected location and other fields, and SHALL NOT add a beta for that web version

#### Scenario: Version-specific fields are not forwarded to older variants
- **WHEN** `useCache` or `responseInclusion` is present on an older web variant that does not declare that field
- **THEN** that field SHALL be omitted from the resulting request, preserving the version's own beta rule

### Requirement: Validate declared web-tool arguments before a request
For each supported web ID, the Anthropic provider SHALL validate its declared optional argument fields before HTTP on both generate and stream paths. If a present field has an invalid type, is null, or has a disallowed enum value, the provider SHALL return an error and SHALL NOT send an HTTP request. Optional omitted fields SHALL remain omitted. Valid finite numeric `maxUses` and `maxContentTokens` values, including fractions, SHALL be serialized without integer truncation; the SDK's typed integer fields SHALL NOT narrow the registered upstream `z.number()` contract. Validation SHALL include string arrays for domain lists, a `citations` object that requires boolean `enabled` when present, an approximate `userLocation` object that requires `type: "approximate"` when present and permits optional string `city`, `region`, `country`, `timezone`, boolean `useCache`, and the 20260318-only `responseInclusion` enumeration. Unknown fields SHALL be stripped rather than forwarded, matching upstream object-schema projection. Unsupported provider tool IDs SHALL retain their warning-and-skip behavior.

#### Scenario: Fractional and large finite web-tool numbers
- **WHEN** a supported web tool supplies a finite fractional or out-of-int64 `maxUses` or `maxContentTokens` value
- **THEN** the HTTP request SHALL contain that numeric value in the corresponding wire field without truncation, including when unsupported tools precede it in the declared tools list; no override SHALL reinsert a tool removed by tool choice `none`

#### Scenario: Invalid numeric and domain fields
- **WHEN** a supported web tool supplies a non-number `maxUses` or `maxContentTokens`, `null` for a declared field, or non-string elements in a domain list
- **THEN** request preparation SHALL fail before any HTTP request

#### Scenario: Invalid nested objects or enum
- **WHEN** a declared web tool supplies `citations` without `enabled` or with a non-boolean `enabled`, `userLocation` without `type` or with a type other than `approximate` or non-string location fields, invalid `useCache`, or `responseInclusion` outside `full` and `excluded`
- **THEN** request preparation SHALL fail before any HTTP request

#### Scenario: Unrecognized arguments and unsupported provider tool IDs
- **WHEN** a supported web tool has an extra unknown argument, or a separately declared provider tool ID is unsupported
- **THEN** the unknown argument SHALL be stripped for the supported tool and the unsupported ID SHALL still be skipped with a warning; neither SHALL become a function tool

### Requirement: Web-tool requests preserve function-tool and no-tool boundaries
Adding versioned provider-defined tools and their validation SHALL NOT interpret a server tool's `Strict`, `InputExamples` or `ProviderOptions` as function settings. Function tools SHALL retain their own strict support/warnings, input-example conversion and Anthropic provider-option behavior. Existing auto/required/named/none tool-choice behavior with an empty tool list SHALL remain unchanged.

#### Scenario: Mixed function and provider-defined web tools
- **WHEN** a function tool with `Strict`, `InputExamples` and Anthropic tool provider options is declared alongside a provider-defined web tool
- **THEN** each SHALL be converted through its own path, preserving applicable function-only betas and warnings and web-version beta rules without leaking function-only fields onto the server tool

#### Scenario: Invalid function input examples and options retain existing handling
- **WHEN** malformed function-tool input examples or Anthropic provider options accompany a valid server web tool
- **THEN** their existing skip/warning behavior SHALL remain unchanged, and the server tool SHALL NOT inherit those fields

#### Scenario: Empty tool choice
- **WHEN** a request has no tools and an auto, required, named, or none tool choice
- **THEN** it SHALL preserve the existing selection/omission semantics for that choice, without fabricating a web tool
