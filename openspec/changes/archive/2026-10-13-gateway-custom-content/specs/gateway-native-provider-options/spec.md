# Spec Delta

## MODIFIED Requirements

### Requirement: Opaque options reach native adapters at supported scopes
The Gateway SHALL forward ordinary provider-option namespace objects to native adapters at their original supported call, message, content-part (including assistant custom parts), function-tool and nested tool-result file-entry and custom-entry scopes. It SHALL preserve namespace spelling, opaque namespace JSON bytes and meaningful empty/nested values without mutation, field inventories, namespace filtering, hoisting or cross-provider translation.

#### Scenario: Empty and nested options survive mapping
- **WHEN** a supported direct unary or streaming request carries namespace objects containing null, false, zero, empty strings, empty arrays, empty objects and nested JSON at the supported scopes
- **THEN** the mapped model SHALL receive the original namespaces and namespace JSON at those exact scopes, including explicitly empty namespace objects
- **AND** the input SHALL remain unchanged

#### Scenario: Irrelevant and future fields are not inventoried
- **WHEN** a direct request carries an ordinary namespace irrelevant to the selected adapter or an ordinary field absent from a Gateway inventory
- **THEN** the Gateway SHALL retain it for native interpretation rather than silently remove it or refuse it merely for being unlisted

#### Scenario: Native namespace precedence remains authoritative
- **WHEN** a request supplies competing supported native namespace aliases or a differently cased namespace
- **THEN** actual native consumption SHALL follow the adapter's existing exact matching and precedence without Gateway merging or spelling normalization

## ADDED Requirements

### Requirement: Custom-scope consumption audit
Admitting custom history and custom tool-result entries SHALL record the consumption-backed audit for each newly caller-reachable native path, with a disposition and paired bypass/harmless cases. The paths are Anthropic tool-reference `toolName` and fallback replay, OpenAI compaction `itemId`/`encryptedContent`, OpenAI-compatible stringification of custom entries, and the OpenAI Responses custom-entry drop.

#### Scenario: Compaction item reference through a configured account
- **WHEN** a caller supplies `openai.compaction` history with an `itemId` on a route whose OpenAI adapter stores responses
- **THEN** the audit SHALL record that the adapter sends an item reference within the configured account, its disposition relative to the accepted reasoning `itemId` precedent, and a regression case
