# gateway-openai-chat-completions-adapter Specification

## Purpose
Define the represented OpenAI Chat Completions codec over shared Gateway execution.
#483 shared lifecycle validation remains pending before readiness.

## Requirements

### Requirement: Independent unary handler

The AGPL Chat Completions package SHALL construct an HTTP handler with a
non-nil execution.Runtime and translate the represented unary subset to and
from the canonical provider domain. It SHALL accept relative POST
`/v1/chat/completions` with JSON content and SHALL reject `stream:true` before
runtime selection. This capability SHALL NOT register a production plugin
route or supply authentication.

#### Scenario: Unary request through the official Go client
- **WHEN** the official OpenAI Go client calls the mounted test handler with a
  valid text request and configured model alias
- **THEN** it receives one Chat completion choice with assistant content,
  valid identity and represented usage.

#### Scenario: Streaming is deferred
- **WHEN** a request sets `stream:true`
- **THEN** the handler returns a fixed OpenAI 400 error without resolving or
  invoking a model.

#### Scenario: Handler route boundary
- **WHEN** a request uses an unsupported method, path, query, encoded path,
  content encoding or non-JSON content type
- **THEN** the handler refuses it before model invocation, with an OpenAI error
  envelope and an Allow header for the method refusal.

### Requirement: Explicit unary request subset

The adapter SHALL validate a closed schema before typed mapping. It SHALL
represent text, client-owned functions and continuations, structured response
formats, reasoning effort and supported sampling settings. Unsupported fields,
multiple choices, storage true and invalid settings SHALL fail before model
resolution. Duplicate keys SHALL use their last value; incorrectly cased keys
SHALL fail.

#### Scenario: Duplicate and incorrectly cased fields
- **WHEN** a request repeats the model key or includes `Model` instead
- **THEN** the repeated exact key uses its last value and the incorrectly cased
  key fails validation.

#### Scenario: Function continuation
- **WHEN** history contains an assistant function call followed by its matching
  tool result
- **THEN** the adapter maps the result to that function's name and call ID;
  an orphan result is refused.

#### Scenario: Structured response name and restricted schema
- **WHEN** a supported JSON schema response format supplies its required name
- **THEN** the mapped provider response format preserves the name's presence;
  external references or unsupported schema vocabulary are refused.

### Requirement: Candidate-local Chat defaults

The adapter SHALL supply an execution.Translator per call, copying options and
mapping Chat storage, strictness and explicit parallel-tool settings for the
host-provided native target. Responses and compatible attempts SHALL receive
storage false; omitted/null Chat strict SHALL mean false. The host SHALL
apply translation separately to physical attempts, re-check policy and native
options, and preserve calls without a translator.

#### Scenario: Backend-specific parallel settings
- **WHEN** parallel tool calls are explicitly disabled
- **THEN** translation supplies Responses `parallelToolCalls:false`, Anthropic
  `disableParallelToolUse:true`, or compatible `parallel_tool_calls:false`
  under the target namespace without mutating source options.

#### Scenario: Another protocol uses the model
- **WHEN** a call supplies no Chat translator
- **THEN** the host preserves its options without applying Chat defaults.

### Requirement: Valid unary results and diagnostics

Unary output SHALL contain one choice, a represented finish reason and truthful
optional usage. Available native identity SHALL replace generated fallbacks.
Supported warnings SHALL remain successful `grafana.warnings` diagnostics.
Unsupported content or inconsistent tool output SHALL fail safely. Strict
structured output SHALL validate on successful stop, not length or
content-filter partial output.

#### Scenario: Native identity is absent
- **WHEN** the result omits native response ID, model ID or timestamp
- **THEN** the adapter supplies each missing field from a generated completion
  ID, the canonical catalog model ID or current time respectively.

#### Scenario: Native identity and warnings
- **WHEN** a valid result provides native response metadata and an unsupported
  setting warning
- **THEN** the response retains that identity and warning without turning the
  otherwise successful result into an error.

#### Scenario: Strict structured output
- **WHEN** a successful stop returns invalid JSON or violates an explicitly
  strict response schema
- **THEN** the adapter returns a fixed upstream error instead of a successful
  completion; partial output ending for length is not required to validate.

### Requirement: Host-owned resources and safe failures

The adapter SHALL obtain a Call from execution.Runtime and use Call.Generate
for unary execution. Selection, policy, execution, cancellation and failure
classification SHALL belong to the shared runtime; Chat SHALL map its closed
categories to fixed OpenAI documents. The adapter SHALL add no private
selection/invocation worker, drain, total deadline or transport-byte cap and
SHALL NOT copy raw provider errors, bodies or headers into its errors.

#### Scenario: No adapter-imposed size or duration
- **WHEN** the test host supplies a request and response above the old one MiB
  bound and supplies no deadline
- **THEN** the adapter adds no size cap or deadline and delegates execution to
  the same runtime contract as other adapters.

#### Scenario: Request deadline during generation
- **WHEN** the request deadline expires during cooperative generation
- **THEN** the provider receives the host context and the runtime outcome maps
  to the fixed timeout or cancellation category without resetting a deadline.

#### Scenario: Safe upstream failure
- **WHEN** the runtime classifies a provider failure containing private text
- **THEN** Chat maps its category to a fixed document without copying that text.
