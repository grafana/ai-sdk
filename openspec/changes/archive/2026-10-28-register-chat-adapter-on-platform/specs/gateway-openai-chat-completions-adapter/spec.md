## ADDED Requirements

### Requirement: Chat registration through the host contract

The host SHALL construct Chat on its shared execution.Runtime and register one
namespaced POST `/v1/chat/completions` route-table entry with the handler,
`chat_completions` label and Chat host.ErrorWriter. Generic registration SHALL
supply access and telemetry. Chat SHALL NOT install a private selector,
catalog wrapper, access dispatcher, execution worker or drain loop.

#### Scenario: Official SDK addresses the namespaced route
- **WHEN** an official OpenAI client uses the namespaced API prefix ending in
  `/v1` as baseURL and calls Chat Completions
- **THEN** its request reaches the registered Chat handler through the shared
  host middleware and runtime.

### Requirement: Shared access and error boundaries

Chat SHALL preserve the host's configured/BYOK/refused access decisions.
Configured callers SHALL use the catalog. Chat SHALL expose no BYOK controls;
BYOK-only callers without those controls SHALL receive the shared invalid-BYOK
outcome with zero native I/O. Host failures SHALL use the adapter's host-error
writer; pre-plugin failures MAY use the platform format.

#### Scenario: Configured callers
- **WHEN** a self-hosted user/service account or a cloud allowlisted internal
  caller makes a valid Chat request
- **THEN** the shared selector uses configured catalog models.

#### Scenario: BYOK-only caller cannot obtain the catalog
- **WHEN** a cloud user/service account classified as BYOK sends a Chat request
  without request-scoped provider controls
- **THEN** the shared selector returns invalid BYOK mapped to HTTP 400 and
  makes no native provider request.

#### Scenario: Host refuses account access
- **WHEN** host policy refuses the caller before reading its body
- **THEN** Chat's host-error writer returns its fixed permission document.

#### Scenario: Platform rejects before the plugin
- **WHEN** platform authentication or transport limits reject a request
- **THEN** the response need not use a Chat error envelope.

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
