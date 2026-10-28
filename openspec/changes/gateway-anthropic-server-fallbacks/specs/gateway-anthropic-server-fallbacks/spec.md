# Spec Delta

## Purpose

Define how configured Gateway candidates admit Anthropic server-side fallback (`providerOptions.anthropic.fallbacks`) only within an operator policy enforced at native consumption. Configured policies SHALL NOT apply to request-account (BYOK) invocation; its refusal is specified once, in gateway-request-byok.

## ADDED Requirements

### Requirement: Candidate server-fallback policy configuration
A configured candidate (route primary or ordered fallback entry) SHALL accept an optional `serverFallbacks` policy with `models`, a list of allowed server-side fallback model IDs, and `allowDefault`, a boolean. An absent or null policy SHALL allow no server-side fallback. The policy SHALL be operator configuration only and SHALL NOT be exposed through discovery, errors or caller-supplied controls.

#### Scenario: Policy on a direct Anthropic candidate
- **WHEN** an `anthropic` primary declares `serverFallbacks` with `models: [claude-opus-4-8]` and `allowDefault: true`
- **THEN** configuration loading SHALL succeed, preserve the policy for that candidate only and leave discovery rows and configured candidate facts identical to the same catalog without the policy

#### Scenario: Policy is absent
- **WHEN** a candidate omits `serverFallbacks` or sets it to null
- **THEN** that candidate SHALL refuse every active server-side fallback exactly as before this change

### Requirement: Strict server-fallback policy validation
Configuration loading SHALL fail before secret resolution or provider construction when a policy is on a candidate whose provider type is not `anthropic`, when it allows neither `"default"` nor any model, or when a model entry is empty, blank, invalid UTF-8, longer than the existing per-value text bound or repeated. Strict YAML decoding SHALL reject unknown policy fields. Errors SHALL be secret-safe.

#### Scenario: Policy on an unsupported provider type
- **WHEN** an `openai` or `openai-compatible` candidate declares `serverFallbacks`
- **THEN** startup SHALL fail with a diagnostic naming the candidate path, before serving requests

#### Scenario: Empty or repeated policy
- **WHEN** a policy has `allowDefault: false` and no models, or lists the same model twice, or lists an empty model
- **THEN** startup SHALL fail with a diagnostic naming the candidate path

### Requirement: Programmatic catalog policy check
Programmatic catalog construction, which does not run configuration validation, SHALL fail when a policy is on a candidate whose resolved provider type is not `anthropic`, rather than ignore the policy. The error SHALL be secret-safe.

#### Scenario: Programmatic catalog with a policy on an unsupported provider type
- **WHEN** a catalog is constructed directly from a configuration whose `openai` or `openai-compatible` candidate carries `serverFallbacks`
- **THEN** construction SHALL fail with an error that echoes no policy values or secrets, and no catalog SHALL be returned

### Requirement: Policy-gated native server-side fallback
Each native Anthropic attempt of a configured candidate SHALL resolve the `AnthropicOptions` the adapter consumes and admit active fallbacks only within that candidate's policy, after per-attempt translation and before native I/O, in unary and streaming modes. A chain SHALL be admitted only when every entry's model matches an allowlisted model byte for byte and entry models are distinct; `"default"` only when `allowDefault` is true. Other entry fields SHALL be forwarded unchanged.

#### Scenario: Allowed explicit chain
- **WHEN** a candidate allows `claude-opus-4-8` and the call sends `fallbacks: [{model: claude-opus-4-8, max_tokens: 2048, thinking: {type: disabled}, output_config: {effort: high}}]`
- **THEN** the native request SHALL carry that chain unchanged with the explicit server-side fallback beta

#### Scenario: Allowed default
- **WHEN** a candidate has `allowDefault: true` and the call sends `fallbacks: "default"`
- **THEN** the native request SHALL carry `"fallbacks": "default"` with the default server-side fallback beta

#### Scenario: Disallowed model, repeated entry or default
- **WHEN** a chain names a model outside the candidate's allowlist or repeats an entry model, or the call sends `"default"` without `allowDefault`
- **THEN** that attempt SHALL fail with the existing unsupported-request error before its native request, without echoing the requested model names

#### Scenario: Inactive fallbacks remain no-ops
- **WHEN** `fallbacks` is null, absent or an empty array
- **THEN** only the policy check SHALL be skipped; the existing MCP and container-skill checks SHALL still apply

#### Scenario: Translation adds fallbacks
- **WHEN** per-attempt translation adds fallbacks to a candidate's options
- **THEN** the translated options SHALL be checked against that candidate's policy before its native request

### Requirement: Gateway server-fallback support boundaries
The allowlist, distinctness and exact-byte rules SHALL be intentional Gateway support boundaries: registered `@ai-sdk/anthropic` 4.0.82 forwards any non-empty array, including repeated models, without an allowlist. Distinctness caps the server-side generations one request can bill to the operator account at the allowlist size, without a new count constant. Exact-byte matching treats model IDs as opaque.

#### Scenario: Requests upstream would forward
- **WHEN** a chain repeats an allowlisted model with different `thinking`, or names a case or whitespace variant of an allowlisted model
- **THEN** the attempt SHALL be refused before its native request, as for any disallowed chain

### Requirement: Server-side fallback stays inside one Gateway attempt
Server-side fallback SHALL run within one Gateway attempt and one outbound request. Each configured candidate SHALL apply only its own policy, including candidates chosen by any future request router. A policy refusal SHALL fail that candidate before its native request; the existing Gateway fallback decider SHALL then decide advancement as for any unsupported-request error. No new retry layer or cross-candidate pre-validation SHALL be added.

#### Scenario: Primary refuses and fallback candidate allows
- **WHEN** the primary has no policy, the next candidate allows the requested chain and the call sends that chain
- **THEN** the primary SHALL make no native request, the next candidate SHALL receive the chain, and the overview SHALL list a failed primary and a selected second candidate

#### Scenario: No candidate allows the request
- **WHEN** every candidate's policy refuses the requested fallbacks
- **THEN** the call SHALL return the fixed invalid-request error with no native request

### Requirement: Server-side fallback evidence boundary
Acceptance SHALL use synthetic native requests for policy cases and the two recorded `anthropic/recorded/server-side-fallback-{chain,default}` fixture cases (four client rows) for allowed requests, with recorded inputs unchanged. The harness SHALL key its fixed allowlist by fixture case name, not client-suffixed row ID, and not derive policies from a row's request. Evidence SHALL NOT claim live refusal-triggered hops, Anthropic's `"default"` choice or billing of fallback-served turns.

#### Scenario: Matrix rows with an allowing policy
- **WHEN** the Gateway conformance matrix configures the `anthropic/recorded/server-side-fallback-chain` and `anthropic/recorded/server-side-fallback-default` fixture cases from its fixed allowlist
- **THEN** both client rows of each case SHALL receive the case's policy and pass against their unchanged recorded inputs, and rows of any case outside the allowlist SHALL receive no policy
