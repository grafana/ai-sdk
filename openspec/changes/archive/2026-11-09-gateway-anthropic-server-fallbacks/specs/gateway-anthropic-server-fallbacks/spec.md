# Spec Delta

## Purpose

Define how configured Gateway candidates admit Anthropic server-side fallback (`providerOptions.anthropic.fallbacks`) only within an operator policy enforced at native consumption. Request-account (BYOK) invocation is out of scope: it forwards server-side fallback without a policy, and configured policies do not apply to it, as specified once in gateway-request-byok. Policy-admitted server-side fallback is not a bypass of resolved model ownership under gateway-native-provider-options "Concrete native bypass protections": the candidate's model stays the attempt's identity and a hop is reported only as the served model.

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
For configured candidates, the allowlist, distinctness and exact-byte rules SHALL be intentional Gateway support boundaries, because the operator account pays: registered `@ai-sdk/anthropic` 4.0.94 forwards any non-empty array, including repeated models, without an allowlist. Distinctness caps the server-side generations one request can bill to the operator account at the allowlist size, without a new count constant. Exact-byte matching treats model IDs as opaque.

#### Scenario: Requests upstream would forward
- **WHEN** a configured candidate receives a chain that repeats an allowlisted model with different `thinking`, or names a case or whitespace variant of an allowlisted model
- **THEN** the attempt SHALL be refused before its native request, as for any disallowed chain

### Requirement: Server-side fallback stays inside one Gateway attempt
Server-side fallback SHALL run within one Gateway attempt and one outbound request. Each configured candidate SHALL apply only its own policy. A policy refusal SHALL fail that candidate before its native request; the existing Gateway fallback decider SHALL then decide advancement as for any unsupported-request error. No new retry layer or cross-candidate pre-validation SHALL be added.

#### Scenario: Primary refuses and fallback candidate allows
- **WHEN** the primary has no policy, the next candidate allows the requested chain and the call sends that chain
- **THEN** the primary SHALL make no native request, the next candidate SHALL receive the chain, and the overview SHALL list a failed primary and a selected second candidate

#### Scenario: No candidate allows the request
- **WHEN** every candidate's policy refuses the requested fallbacks
- **THEN** the call SHALL return the fixed invalid-request error with no native request

### Requirement: Server-side fallback across routed attempts
A request routed under gateway-request-routing SHALL apply, at each attempt, the policy of the configured candidate that attempt was composed from, as that capability's candidate-native request semantics require. When routes share a destination, the policy of the candidate composed first SHALL apply, because a repeated destination is attempted once.

#### Scenario: Requested route refuses and a fallback model allows
- **WHEN** route `A` has no policy, route `B` has a different destination and allows the requested chain, and the call to `A` sends that chain with `models: [B]`
- **THEN** `A`'s candidates SHALL make no native request, `B`'s candidate SHALL receive the chain, and the overview SHALL list `A`'s failed attempts then `B`'s selected attempt

#### Scenario: Routes share a destination with different policies
- **WHEN** routes `A` (no policy) and `B` (allowing the chain) each have only one candidate, both on the same provider instance and backend model, and a call sends the chain to `A` with `models: [B]`, or to `B` with `models: [A]`
- **THEN** the call to `A` SHALL fail with the fixed invalid-request error and no native request, and the call to `B` SHALL send the chain in one native request through `B`'s candidate

### Requirement: Server-side fallback evidence boundary
Acceptance SHALL use synthetic native requests for policy cases. For allowed requests it SHALL replay the two recorded fixtures `anthropic/recorded/server-side-fallback-{chain,default}`, unchanged, through the real Gateway command with both registered clients. The test SHALL configure a fixed policy per fixture and SHALL NOT derive it from the request. Evidence SHALL NOT claim live refusal-triggered hops, Anthropic's `"default"` choice or billing of fallback-served turns.

#### Scenario: Recorded fixtures with an allowing policy
- **WHEN** the Gateway command serves a `claude-opus-5` Anthropic route whose policy allows `claude-opus-4-8` (chain fixture) or has `allowDefault: true` (default fixture), and a backend replays the fixture's unchanged input
- **THEN** the TypeScript and Go clients SHALL each receive the fixture's expected UI chunks, and the backend SHALL receive native requests matching the fixture's expected request snapshots

#### Scenario: Recorded fixtures without an allowing policy
- **WHEN** the same fixtures are sent through a route with no policy, or with the other fixture's policy
- **THEN** both clients SHALL receive the fixed invalid-request error and the backend SHALL receive no request
