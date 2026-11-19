## MODIFIED Requirements

### Requirement: Candidate-local Chat defaults

The adapter SHALL supply an execution.Translator per call, copying options after request transforms and
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

#### Scenario: Fallback after tenant defaulting
- **WHEN** a Chat call with tenant defaults falls back to a different native
  protocol
- **THEN** each attempt receives the same resolved generic settings and
  instructions, with its own native translation and no options carried over
  from the previous candidate.

### Requirement: Chat registration through the host contract

The host SHALL construct Chat on its shared execution.Runtime and register
POST `/v1/chat/completions` and POST
`/modelcatalogs/{name}/v1/chat/completions` under the namespaced API prefix.
Both entries SHALL use the same handler and Chat host.ErrorWriter, with
distinct telemetry labels `chat_completions` and `catalog_chat_completions`.
Generic registration SHALL supply access, telemetry and the catalog parent
check for the catalog entry. Chat SHALL NOT install a private selector,
catalog wrapper, access dispatcher, execution worker or drain loop.

#### Scenario: Official SDK addresses the namespaced route
- **WHEN** an official OpenAI client uses the namespaced API prefix ending in
  `/v1` as baseURL and calls Chat Completions
- **THEN** its request reaches the registered Chat handler through the shared
  host middleware and runtime.

#### Scenario: Official SDK addresses a stored catalog
- **WHEN** an official OpenAI client uses the catalog API prefix ending in
  `/modelcatalogs/{name}/v1` as its base URL and calls Chat Completions
- **THEN** its request reaches the same Chat handler through the catalog
  parent check and shared runtime.

### Requirement: Shared access and error boundaries

Namespace Chat SHALL preserve the host's configured/BYOK/refused access
decisions. Configured callers SHALL use the operator catalog. The catalog
Chat route SHALL use the same declared authorization role, parent check and
catalog access decision as the catalog language-model route, selecting only
from the named catalog with its supplied decrypted keys. Chat SHALL expose no BYOK controls;
BYOK-only callers without those controls SHALL receive the shared invalid-BYOK
outcome with zero native I/O. Host failures SHALL use the adapter's host-error
writer; pre-plugin failures MAY use the platform format.

#### Scenario: Configured callers
- **WHEN** a self-hosted user/service account or a cloud allowlisted internal
  caller makes a valid Chat request
- **THEN** the shared selector uses configured catalog models.

#### Scenario: BYOK-only caller cannot obtain the catalog
- **WHEN** a cloud user/service account classified as BYOK sends a namespace Chat request
  without request-scoped provider controls
- **THEN** the shared selector returns invalid BYOK mapped to HTTP 400 and
  makes no native provider request.

#### Scenario: Host refuses account access
- **WHEN** host policy refuses the caller before reading its body
- **THEN** Chat's host-error writer returns its fixed permission document.

#### Scenario: Platform rejects before the plugin
- **WHEN** platform authentication or transport limits reject a request
- **THEN** the response need not use a Chat error envelope.

#### Scenario: Catalog and operator catalog share a model alias
- **WHEN** a caller permitted to invoke the named catalog requests the alias
  on the catalog Chat route
- **THEN** only the named catalog's candidates and keys are used, regardless
  of Gateway access mode.

#### Scenario: Plugin receives a missing or mismatched parent identity
- **WHEN** the platform-supplied route parent is missing or does not identify
  the requested catalog
- **THEN** the plugin returns the fixed Chat 500 document before reading
  the request body.

#### Scenario: Invalid catalog or disallowed native endpoint
- **WHEN** a stored catalog fails validation, has a missing decrypted key,
  or specifies an endpoint disallowed by stack-catalog policy
- **THEN** Chat returns the fixed 424 catalog-configuration outcome, with no
  provider I/O or catalog details.

### Requirement: Official Chat clients through the plugin protocol

Official OpenAI Go and TypeScript clients SHALL exercise the built plugin via
the test-only platform host using delegated test identities and synthetic
native providers. Evidence SHALL cover unary/streaming, tools/structured
output, native diagnostics, fallback and cancellation, and SHALL distinguish
mapping/protocol-hop proof from live-provider or deployed-Grafana acceptance.

#### Scenario: Go and TypeScript SDK consumption
- **WHEN** the exact pinned clients invoke the represented Chat subset through
  the protocol test host
- **THEN** they consume unary responses and streamed chunks and the native
  providers receive backend-specific per-attempt Chat translations.

#### Scenario: Incrementality and cancellation
- **WHEN** the synthetic provider delays between text fragments or blocks
  until the caller cancels
- **THEN** the client sees incremental output and cancellation reaches the
  provider through the shared runtime and plugin host.

#### Scenario: Official SDK calls catalog in both Gateway modes
- **WHEN** an official OpenAI client uses a delegated service-account identity
  to call a saved catalog through the built plugin in self-hosted and cloud
  access modes
- **THEN** unary and streamed completions succeed and the synthetic native
  provider receives the catalog model and key, not the operator's. The test
  does not attest to secure value availability in a deployed Grafana Cloud.

## ADDED Requirements

### Requirement: Extension outcomes on Chat

Chat calls SHALL run the shared extension pipeline for the source's key
ownership and the call's namespace. The Chat adapter SHALL use fixed error
envelopes for extension denial and fail-closed unavailability without
publishing extension details.

#### Scenario: Applicable operator and tenant extensions
- **WHEN** an operator extension applies to the key ownership on either Chat
  route, or a customer-key tenant extension is enabled for a catalog call
- **THEN** the applicable extensions run through the shared pipeline; a
  customer-key-only extension does not run for an operator-key call.

#### Scenario: Extension stops the call before model invocation
- **WHEN** an applicable extension denies the request or fails closed before
  unary invocation or streaming setup
- **THEN** Chat returns HTTP 403 or 503, respectively, before provider I/O.

#### Scenario: Extension stops a stream after visible output
- **WHEN** a response inspector denies a part or fails closed after Chat has
  streamed visible output
- **THEN** Chat withholds that part, emits a fixed SSE error with code
  `permission_denied` or `unavailable` respectively, and ends without
  replay, a successful finish, usage chunk or `[DONE]`.

### Requirement: No Chat execution overview

Chat SHALL NOT publish the Gateway execution overview on either mount. This
does not remove existing Chat diagnostics or change ProviderWire.

#### Scenario: Chat fallback succeeds
- **WHEN** Chat produces a unary or streamed completion after a configured
  fallback
- **THEN** it keeps the existing Chat success representation, without
  an attempt list or routing overview.
