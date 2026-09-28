## MODIFIED Requirements

### Requirement: Basic streamed calls and results
Private DTOs SHALL encode function calls and correlated basic results with non-null JSON result and optional isError. Duplicate calls/results and unmatched results SHALL fail safely. Client-executed calls SHALL not require a result before finish. Provider-executed, dynamic and preliminary enabled behavior SHALL remain deferred to WP13; only the closed OpenAI itemId or Anthropic direct caller metadata projection SHALL be carried on supported calls/results; arbitrary metadata SHALL be omitted.

#### Scenario: Standalone call awaits client execution
- **WHEN** a complete function call with no incremental input events is followed by valid finish
- **THEN** the client SHALL receive the call and finish without requiring a server result

#### Scenario: Caller-bearing function continuation
- **WHEN** supported Anthropic basic tool-call/result parts carry a direct caller and a local client tool is continued in a second request
- **THEN** both clients SHALL receive the approved caller metadata on the call and correlated output and include it on the continued assistant tool-use; unrelated request differences SHALL not be marked resolved

#### Scenario: Basic result transport
- **WHEN** a recording model emits a call followed by one matching basic JSON result
- **THEN** both clients SHALL receive equivalent call/result values in order

#### Scenario: Deferred execution marker
- **WHEN** providerExecuted, dynamic or preliminary is true in provider output
- **THEN** unsupported output SHALL produce a safe terminal failure without exposing provider metadata

### Requirement: Stateless multi-step client loops
Registered Vercel and Go client orchestration SHALL complete multi-step local function loops through independent real-handler requests with full continuation history. The Gateway SHALL execute no local function and retain no cross-request workflow state. Each request SHALL produce one canonical metadata-only logical observation. WP9 fallback routes SHALL remain effect-disabled.

#### Scenario: Two-client multi-step acceptance
- **WHEN** each client receives a function call, executes its deterministic local function once and sends the result in the next request
- **THEN** both SHALL receive final text, native request history SHALL match and logical records SHALL count one generation per request

#### Scenario: Secret-bearing tool data
- **WHEN** tool input/result/name/ID/schema or provider metadata contains hostile markers
- **THEN** public allowlisted tool content SHALL retain required semantics but exported logs/metrics/metadata-only AO SHALL omit those markers and all provider metadata; the public tool result SHALL contain only eligible approved provider-part metadata under the closed projection

#### Scenario: Streaming tool on fallback route
- **WHEN** a tool request or continuation targets a WP9 fallback-configured route
- **THEN** no physical candidate SHALL execute and the fixed safe unsupported-request error SHALL be returned
