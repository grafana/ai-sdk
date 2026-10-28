# Spec Delta

## ADDED Requirements

### Requirement: Access-aware selection
One selector SHALL serve every access mode, so each adapter needs one runtime. It SHALL branch on the verified caller's account access. Configured access SHALL resolve through the catalog and refuse call-level `gateway` controls. BYOK access SHALL decode request accounts from those controls. Any other access SHALL fail with the permission category.

#### Scenario: Configured access
- **WHEN** a configured caller calls a catalog model without `gateway` controls
- **THEN** the selector SHALL resolve it through the catalog and invoke only configured accounts

#### Scenario: BYOK access
- **WHEN** a BYOK caller sends valid `gateway.byok` accounts
- **THEN** the selector SHALL build the model from those accounts, with zero catalog resolutions

#### Scenario: No account access
- **WHEN** a request reaches the selector without configured or BYOK access
- **THEN** the call SHALL fail with the permission category, with zero catalog resolutions and zero provider requests

#### Scenario: BYOK account bound
- **WHEN** a BYOK request supplies more than 8 accounts for the selected provider
- **THEN** the call SHALL fail with the BYOK credentials category and the fixed BYOK 400 document, with zero provider requests

#### Scenario: BYOK disabled
- **WHEN** the Gateway runs in `self-hosted` mode
- **THEN** no caller SHALL reach BYOK selection

### Requirement: Per-request option translator
An adapter MAY supply one option translator with each call. The runtime SHALL make it available to every physical attempt of that call only. A call without a translator SHALL reach every attempt with unchanged options.

#### Scenario: No translator
- **WHEN** an adapter makes a call without a translator
- **THEN** every attempt SHALL receive the options the runtime passed to the model, unchanged

#### Scenario: Concurrent calls are isolated
- **WHEN** two concurrent calls on one runtime use different translators, or one uses none, over the same configured model or the same BYOK account shape
- **THEN** each attempt SHALL see only its own call's translation

### Requirement: Translation at the candidate boundary
The host SHALL wrap every configured candidate and every BYOK account once, when it is constructed. The wrapper SHALL sit directly outside that candidate's native option validation and inside fallback. Each attempt SHALL call the translator with its target and its own copy of the options. Every native option validation SHALL check the translated options.

#### Scenario: Heterogeneous fallback chain
- **WHEN** a configured model's Anthropic primary fails retryably and its OpenAI fallback succeeds, under a translator that writes target-specific options
- **THEN** the Anthropic request SHALL carry only the Anthropic translation, and the OpenAI request SHALL carry only the OpenAI translation

#### Scenario: BYOK accounts are translated per attempt
- **WHEN** a BYOK call's first account fails retryably and its second account succeeds, under a translator
- **THEN** each account's native request SHALL carry its own translation of the original options

#### Scenario: Translated Anthropic options are validated
- **WHEN** a translator produces Anthropic options on a configured or BYOK Anthropic candidate
- **THEN** native option validation SHALL accept valid translated options, and SHALL refuse invalid ones with the unsupported-request outcome and no provider request for that attempt

#### Scenario: Single-candidate model
- **WHEN** a configured model has no fallbacks
- **THEN** its only candidate SHALL still be translated

### Requirement: Host policy on translated options
The host SHALL re-check each attempt's translated options before the attempt reaches native option validation. Translated options that carry a call-level `gateway` namespace or a protected call header SHALL fail that attempt.

#### Scenario: Translator injects the reserved namespace
- **WHEN** a translator adds a call-level `gateway` namespace
- **THEN** the attempt SHALL fail before any provider contact

#### Scenario: Translator injects a protected header
- **WHEN** a translator adds `Authorization` or another protected call header, in any case
- **THEN** the attempt SHALL fail before any provider contact, and the outcome SHALL NOT carry the header name or value

### Requirement: Translation failures end the call
A translator panic, or translated options that fail host policy, SHALL fail that attempt as non-retryable: fallback SHALL NOT try later candidates, and the call SHALL fail with the internal category. An error the translator returns SHALL be an ordinary attempt failure, classified and retried as a native validation error would be.

#### Scenario: Panicking translator
- **WHEN** a translator panics on the first candidate of a two-candidate chain
- **THEN** the call SHALL fail with the internal category, with zero provider requests and no attempt on the second candidate, and other in-flight calls SHALL be unaffected

#### Scenario: Policy refusal does not fall back
- **WHEN** a translator injects a protected header for the first candidate only
- **THEN** the call SHALL fail with the internal category, with no attempt on the second candidate

#### Scenario: Translator error
- **WHEN** a translator returns the unsupported-request error for the first candidate and translates the second
- **THEN** the first attempt SHALL fail with no provider request, and the second candidate SHALL be attempted

## MODIFIED Requirements

### Requirement: Test-only adapter evidence
Tests SHALL include a minimal HTTP adapter, not ProviderWire, built only on the runtime and the host contract, with its own response and host-error format. For identical fake models, it SHALL show the same deadline, cancellation, drain, classification, policy and attempt-view outcomes as ProviderWire. It SHALL NOT be shipped.

#### Scenario: Matching outcomes
- **WHEN** the same fake model scenarios run through the test adapter and ProviderWire
- **THEN** both SHALL report the same category, reason, model-call count, drains and attempt view for every scenario

#### Scenario: Translation through the test adapter
- **WHEN** the test adapter supplies a translator on a call over a fallback model
- **THEN** each attempt SHALL see the translation for its own target, as specified for per-attempt translation
