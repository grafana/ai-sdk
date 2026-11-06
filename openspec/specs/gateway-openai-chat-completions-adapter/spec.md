# gateway-openai-chat-completions-adapter Specification

## Purpose
Define Chat request mapping, wire responses, error documents and protocol state
over the shared Gateway runtime and host contract. The runtime owns selection,
execution, attempt capture and provider output lifecycle validation. The host
owns access and transport policy. Sequential native segments are refused explicitly,
not silently truncated according to a different protocol's semantics.

## Requirements

### Requirement: Explicit unary request subset

The adapter SHALL validate a closed schema before typed mapping. It SHALL
represent text, client-owned functions and continuations, structured response
formats, reasoning effort and supported sampling settings. Unsupported fields,
storage true and invalid settings SHALL fail before model
resolution. Duplicate keys SHALL use their last value; incorrectly cased keys
SHALL fail.

#### Scenario: Native alternative count
- **WHEN** Chat requests n between one and 128
- **THEN** compatible candidates receive that native count in one request;
  other candidates reject n greater than one before native I/O.

#### Scenario: Duplicate and incorrectly cased fields
- **WHEN** a request repeats the model key or includes `Model` instead
- **THEN** the repeated exact key uses its last value and the incorrectly cased
  key fails validation.

#### Scenario: Function continuation
- **WHEN** history contains an assistant function call followed by its matching
  tool result
- **THEN** the adapter maps the result to that function's name and call ID;
  an orphan result is refused.

#### Scenario: Structured response name and self-contained schema
- **WHEN** a supported JSON schema response format supplies its required name
- **THEN** the mapped provider response format preserves the name's presence;
  local references, unions and nested schema vocabulary are retained. External
  resource loading is disabled during compilation.

#### Scenario: Function with no parameters
- **WHEN** a function declaration omits parameters
- **THEN** the adapter supplies an empty object parameter schema.

#### Scenario: Standard message variants
- **WHEN** history contains developer instructions, names, refusal history or multipart text tool results
- **THEN** developer instructions map to system messages, refusal text remains in history, and tool text parts concatenate in order; compatible Chat candidates retain native developer, name and refusal fields.

### Requirement: User multimodal inputs

The adapter SHALL map user image URLs, inline audio and file data or IDs into
common file parts, retaining order and inline filenames. Per-attempt translation
SHALL select the native reference namespace without mutating the source prompt.
Image detail SHALL use a typed compatible option, never a raw URL override.
A native transport unable to represent the input SHALL fail before native I/O.

#### Scenario: Images and PDF across backends
- **WHEN** a user sends images, PDF data and an uploaded file ID
- **THEN** Responses, Anthropic and compatible requests retain their native media
  representations; Responses and compatible requests retain image detail.

#### Scenario: Audio capability
- **WHEN** a user sends WAV or MP3 audio
- **THEN** a compatible Chat attempt forwards input_audio and a Responses or
  Anthropic attempt fails without sending or discarding the audio.

#### Scenario: Protected image URL
- **WHEN** a compatible input supplies typed image detail
- **THEN** the native image URL still comes from the common file part and raw
  image_url, type, file and input_audio overrides remain refused by Gateway policy.

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

Unary output SHALL contain the requested indexed choices, each with a represented
finish reason, and truthful aggregate optional usage. Available native identity SHALL replace generated fallbacks.
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

#### Scenario: Empty and interrupted completions
- **WHEN** a valid result is empty or a function call ends with length or content_filter
- **THEN** empty text remains a successful completion and partial argument strings retain their terminal reason; complete function output still requires valid arguments and strict-schema compliance.

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

### Requirement: Independent Chat handler

The AGPL Chat package SHALL construct an HTTP handler with a non-nil execution.Runtime and translate supported unary and streaming requests to the provider
domain. It SHALL accept relative POST `/v1/chat/completions` with JSON content;
`stream:true` SHALL select streaming. Handler construction SHALL NOT register a production route or supply
authentication; the host owns registration and access.

#### Scenario: Unary request through the official Go client
- **WHEN** the official OpenAI Go client calls the mounted test handler with a
  valid text request and configured model alias
- **THEN** it receives one Chat completion choice with assistant content,
  valid identity and represented usage.

#### Scenario: Streaming through the official Go client
- **WHEN** the official OpenAI Go client requests streaming with usage enabled
- **THEN** it consumes Chat chunks, one finish and one usage-only chunk without
  changing the unary contract.

#### Scenario: Handler route boundary
- **WHEN** a request uses an unsupported method, path, query, encoded path,
  content encoding or non-JSON content type
- **THEN** the handler refuses it before model invocation, with an OpenAI error
  envelope and an Allow header for the method refusal.

### Requirement: Streaming Chat representation

The handler SHALL emit SSE data frames and flush represented text and function
progress without buffering the entire response. It SHALL open with an
assistant-role chunk, emit a valid finish, optionally emit usage with empty
choices, and terminate successful output with `[DONE]`. It SHALL NOT replay
the model call after output is written. Failure SHALL omit `[DONE]` and use a
fixed Chat error where the writer remains usable.

#### Scenario: Text and optional usage
- **WHEN** the provider supplies valid text followed by finish and closure
- **THEN** the adapter emits text chunks, finish, a usage-only chunk when
  requested, and `[DONE]` in that order.

#### Scenario: Premature termination
- **WHEN** the provider stream closes before a valid finish
- **THEN** the adapter emits a fixed upstream error where writable and does not
  emit `[DONE]`.

#### Scenario: Additional native segment is refused
- **WHEN** another part, metadata, start or finish follows the first finish
  instead of channel closure
- **THEN** Chat refuses the unsupported continuation without a successful
  finish or `[DONE]`; it does not silently truncate or replay the response.

### Requirement: Streaming native identity and diagnostics

The adapter SHALL use available native identity before its first role/content
chunk and fill absent fields with unary fallbacks. Public id/model/created
SHALL remain stable after opening. Later native identity SHALL appear in
`grafana.native_response`; supported stream-start warnings SHALL appear in
`grafana.warnings` without making otherwise valid output fail.

#### Scenario: Early and late identity
- **WHEN** response metadata arrives before or after the first content
- **THEN** early metadata supplies public identity, while late metadata uses
  the extension without changing the identity of later chunks.

### Requirement: Streaming function and structured-output rules

The adapter SHALL assign each function one stable tool index, emit its initial
ID/name/type once and forward argument fragments once. Final arguments SHALL
match the represented stream and the declared tool. Chat-specific tool-choice,
parallelism, JSON argument and strict structured-output rules SHALL remain
enforced. Unsupported text-encoded function input SHALL be rejected.

#### Scenario: Fragmented and empty arguments
- **WHEN** a function arrives as start, fragments, end and matching final call
- **THEN** fragments are emitted once; an empty fragment sequence finalized as
  `{}` emits that normalization once.

#### Scenario: Contradicting final call
- **WHEN** the final function input differs from its streamed fragments
- **THEN** the adapter fails safely without a successful terminator.

### Requirement: Streaming runtime ownership

The handler SHALL use Call.Stream, Stream.Next and Stream.Close for setup,
wait outcomes, cancellation and drain. It SHALL NOT implement a private setup
worker, idle timer or drain loop. Runtime idle accounting SHALL follow native
part activity, including metadata; Chat SHALL retain framing, commitment and
its representation state. The shared runtime SHALL validate provider output
lifecycle rules; Chat SHALL retain its protocol representation checks.

#### Scenario: Runtime activity owns idle
- **WHEN** native metadata or reasoning parts arrive within each runtime idle
  interval but produce no Chat text
- **THEN** the runtime activity policy applies without a second Chat-visible
  progress timer; the host total deadline remains effective.

#### Scenario: Cleanup through the runtime
- **WHEN** streaming setup fails or the handler ends after acquiring a stream
- **THEN** runtime setup handling and Stream.Close own provider cancellation
  and bounded drain without a second consumer in Chat.

#### Scenario: No duplicate transport bounds
- **WHEN** valid output contains a large frame or many parts
- **THEN** Chat restores no frame, total-byte or part-count limits and uses
  shared runtime waits and host context.

### Requirement: Response writers without socket deadlines

The handler SHALL flush SSE through the response controller and accept a
writer that does not support socket write deadlines. Where supported, write
deadlines SHALL honor the host request deadline when present and react
to cancellation. Unsupported write deadlines SHALL NOT alone reject a valid
stream; host transport cancellation remains required for plugin writers.

#### Scenario: Flush-capable writer without deadline support
- **WHEN** the response writer supports flushing but reports unsupported write
  deadlines
- **THEN** a valid stream is flushed and completes with `[DONE]`.

#### Scenario: Supported socket deadlines
- **WHEN** the writer supports socket deadlines
- **THEN** the handler retains host deadline and cancellation behavior without adding or
  clearing a host timeout.

### Requirement: Represented Anthropic start metadata

The adapter SHALL discard a custom part whose kind is exactly
`anthropic.message_start` without emitting its raw payload or treating it as
represented progress. Canonical response metadata SHALL remain the source of
native identity. Other custom kinds SHALL continue to fail safely without a
successful stream terminator.

#### Scenario: Anthropic start metadata beside canonical identity
- **WHEN** canonical response metadata and an `anthropic.message_start` custom
  part precede valid text and finish
- **THEN** the adapter preserves native identity, emits the text and completes
  with `[DONE]` without exposing the custom payload.

#### Scenario: Unknown custom kind
- **WHEN** a part uses `anthropic.other` or another provider's custom kind
- **THEN** the adapter emits a fixed error where writable and omits `[DONE]`.

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
Configured callers SHALL use the catalog.
BYOK-only callers without required request accounts SHALL receive the shared invalid-BYOK
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

### Requirement: Shared provider options

Chat SHALL accept object-valued `providerOptions` namespaces, including `gateway`
controls. The runtime SHALL remove controls before native invocation and apply
its existing access, validation, routing, fallback and redaction policy.
Native guards SHALL run after Chat defaults are merged into the selected namespace.

#### Scenario: BYOK accounts and fallback
- **WHEN** a cloud BYOK caller sends valid `providerOptions.gateway.byok` accounts
- **THEN** the shared selector executes those accounts, including eligible fallback
  attempts, without placing credentials in native JSON bodies or public errors.

#### Scenario: Native options retain shared guards
- **WHEN** Chat includes native provider options alongside its standard fields
- **THEN** unrelated native fields survive translation, Chat defaults take precedence,
  and prohibited native routing or schema overrides are rejected before native I/O.

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

### Requirement: Native multi-segment streaming
The adapter SHALL consume all runtime-validated native response segments before
emitting a single Chat terminal finish and DONE sentinel. It SHALL preserve public
identity and aggregate known per-segment token usage without inventing missing counts.

#### Scenario: Reused native item identifiers
- **WHEN** consecutive native segments reuse a text identifier
- **THEN** text continues on the same Chat response and new tool calls receive distinct indices
- **AND** raw native transport events do not terminate the response

#### Scenario: Error after native finish
- **WHEN** a native error follows a segment finish before stream closure
- **THEN** the adapter emits an error and no successful DONE sentinel

### Requirement: Shared execution diagnostics
The adapter SHALL use the shared execution view for attribution and native failure
redaction. Successful unary and streaming output SHALL preserve native provider
metadata and SHALL relocate a native gateway namespace when adding execution metadata.

#### Scenario: Credential in a failed attempt
- **WHEN** a configured provider credential appears in a native failure summary
- **THEN** the published execution diagnostics redact that credential
- **AND** the standard Chat error envelope remains available to official clients

#### Scenario: Native gateway metadata collision
- **WHEN** native output contains a gateway metadata namespace
- **THEN** execution attribution preserves it under gateway.nativeMetadata

### Requirement: Sources and citation output
The adapter SHALL preserve retrieved sources and citations in optional diagnostics.
URL citations with native spans SHALL also map to standard Chat annotations without
inventing positions for other citations.

#### Scenario: Native source after text
- **WHEN** valid native text is followed by a retrieved URL or document source
- **THEN** the response retains the text and publishes the source
- **AND** streaming completes successfully

#### Scenario: Citation without span
- **WHEN** a native citation does not include text positions
- **THEN** the citation remains in diagnostics without a fabricated Chat annotation

### Requirement: Native Chat response fields
The adapter SHALL project typed compatible-provider refusal, log-probability, and
usage-detail metadata into standard Chat fields. A refusal SHALL not be rejected
solely because it lacks requested structured JSON content.

#### Scenario: Structured-output refusal
- **WHEN** a compatible backend refuses a structured-output request
- **THEN** unary and streaming clients receive refusal without a codec JSON error

#### Scenario: Detailed token accounting
- **WHEN** native output reports audio or prediction token counters
- **THEN** standard Chat usage details preserve them including explicit zero

### Requirement: Aggregate stream semantics
The adapter SHALL validate structured output and tool cardinality across the
complete public response, not only each native segment. Tool identity validation SHALL remain owned by the shared runtime.

#### Scenario: Individually valid JSON segments
- **WHEN** concatenating native segments produces invalid requested JSON
- **THEN** the stream terminates with an error without DONE

#### Scenario: Tool calls precede a final text segment
- **WHEN** earlier native segments contain client-executed tool calls
- **THEN** a completed public response retains tool_calls as its finish reason
### Requirement: Citation offsets in concatenated text

The adapter SHALL shift native URL citation spans by the number of Unicode code
points in preceding Chat text, including preceding native stream segments. It
SHALL preserve the original block-local spans in diagnostic citations.

#### Scenario: Unicode text precedes a cited block

- **WHEN** an uncited block contains `é😀` and the next block has native span 0–2
- **THEN** the Chat annotation spans 2–4 and diagnostic citation retains 0–2
### Requirement: Legacy function compatibility

Chat SHALL normalize deprecated functions and function_call request controls and
function-role history to common function tools. Legacy requests SHALL receive
function_call message/delta fields and finish reasons. Conflicting modern tool
controls or parallel legacy calls SHALL be rejected.

#### Scenario: Legacy function continuation
- **WHEN** a client declares functions, receives a function_call and supplies the
  corresponding assistant history and function-role result
- **THEN** each native backend receives matching common tool-call/result IDs,
  while the client can continue using the legacy wire shape.

#### Scenario: Legacy streaming
- **WHEN** a legacy request streams a function call
- **THEN** name and argument fragments appear under delta.function_call and the
  final reason is function_call, without tool_calls fields or synthetic IDs.

#### Scenario: Conflicting controls
- **WHEN** functions or function_call occur with tools, tool_choice or enabled
  parallel_tool_calls
- **THEN** Chat returns an invalid request before native I/O.
### Requirement: Native Chat audio request mapping
The adapter SHALL accept Chat output modalities/audio parameters and assistant
audio history references. It SHALL preserve other assistant content and metadata.
Targets lacking native Chat audio capability SHALL reject such requests before
provider I/O rather than silently dropping audio.

#### Scenario: Compatible audio generation with history
- **WHEN** a request contains audio generation options and an assistant audio ID
- **THEN** the compatible native request retains modalities, format, voice and audio ID
- **AND** assistant text, participant name and refusal remain present

#### Scenario: Unsupported audio transport
- **WHEN** the selected Responses or Anthropic transport receives audio generation or audio history
- **THEN** translation returns an invalid-request error before invoking the native model

### Requirement: Native audio response projection
Compatible audio output SHALL be preserved in standard Chat message.audio and
delta.audio fields, including native identity, expiration, data and transcript.

#### Scenario: Audio-only completion
- **WHEN** the native compatible response contains audio without text
- **THEN** the unary message retains null content and complete audio
- **AND** streaming clients receive native incremental audio fields
### Requirement: Native request controls
The adapter SHALL map user, safety identifier, service tier, prompt cache identity
and retention, metadata and logprob controls into native compatible and Responses
options. Nonempty logit bias SHALL require a compatible Chat target. Unsupported
Anthropic controls SHALL fail explicitly instead of being ignored.

#### Scenario: Compatible scalar controls
- **WHEN** a Chat request supplies supported scalar controls
- **THEN** the native compatible body retains their canonical Chat field names
- **AND** provider-local metadata preservation controls do not reach the native body

#### Scenario: Selected-token probability adaptation
- **WHEN** Responses is asked for logprobs with zero alternatives
- **THEN** translation requests the minimum positive count required by the pinned normalizer
- **AND** the projection helper removes alternatives beyond the client's requested count without changing native metadata

### Requirement: Native response identity details
The adapter SHALL preserve reported service tier, system fingerprint and native
Chat URL annotations. It SHALL NOT synthesize these fields from request controls.

#### Scenario: Actual tier differs from requested tier
- **WHEN** the provider reports an execution tier
- **THEN** the Chat response exposes that reported tier

#### Scenario: Malformed native annotation
- **WHEN** a native annotation omits required text positions
- **THEN** typed output decoding fails instead of fabricating a span
### Requirement: Custom text tool calls
The adapter SHALL accept custom text/grammar tools, named custom choices and
custom tool-call history. It SHALL preserve raw text in Chat custom calls while
using the common domain's canonical JSON-string final representation internally.
Streamed custom input SHALL use text encoding and match the final decoded input.

#### Scenario: Official client manual execution
- **WHEN** an official client receives a custom tool call and manually returns its result
- **THEN** the follow-up retains the custom call type and common call identity even without tool declarations

#### Scenario: Native custom payload cannot override history
- **WHEN** a compatible provider option attempts to replace a custom call's identity or custom payload
- **THEN** the shared native guard rejects it
- **AND** the consumed custom-history marker alone cannot replace role, call ID, tool name or input

#### Scenario: Inconsistent final custom input
- **WHEN** the decoded final custom input differs from previously emitted text fragments
- **THEN** the adapter emits an error instead of a successful terminal completion

### Requirement: Native generation controls
The adapter SHALL preserve validated prediction and web-search controls on native
compatible routes and map verbosity to compatible and Responses text verbosity.
Other transports SHALL reject unsupported controls before provider I/O.

#### Scenario: Official client predicts static output
- **WHEN** an official Go client submits predicted text, search context and verbosity
- **THEN** the native compatible request retains all three values

#### Scenario: Unsupported transport
- **WHEN** a Responses or Anthropic route receives native Chat prediction or search controls
- **THEN** it returns a capability error rather than dropping the controls

### Requirement: Public stream obfuscation
The adapter SHALL include cryptographically random padding on public completion
chunks by default and when explicitly enabled. Explicit false SHALL omit padding.
Padding SHALL be regenerated after projection and normalize serialized sizes to
64-byte boundaries without changing completion fields.

#### Scenario: Official client disables padding
- **WHEN** a client streams with include_obfuscation false
- **THEN** no completion or usage chunk includes obfuscation

#### Scenario: Enabled padding
- **WHEN** two identical public chunks are serialized with obfuscation enabled
- **THEN** they contain different random padding and retain identical completion data

### Requirement: Native moderation request routing
The adapter SHALL validate and preserve native inline moderation model/policies
on compatible routes and enable typed Chat result metadata preservation.

#### Scenario: No equivalent moderation transport
- **WHEN** Responses or Anthropic is selected for an inline moderation request
- **THEN** the adapter returns a capability error before provider I/O

### Requirement: Preserve explicit Chat text cache boundaries
The adapter SHALL retain request cache options and explicit text breakpoints
across system, developer, user, assistant and tool messages on compatible routes.
Translation SHALL not mutate caller prompt content or options.

#### Scenario: Official client continues a cached tool conversation
- **WHEN** an official Go client submits cache-marked text arrays including tool results
- **THEN** unary and streaming native requests preserve every text boundary and tool ID

#### Scenario: Non-native cache transport
- **WHEN** a request with explicit Chat cache controls selects another transport
- **THEN** it fails with a capability error before provider I/O

### Requirement: Preserve multimodal cache options
Compatible translation SHALL retain explicit breakpoints on user text, image,
audio and file parts without replacing typed image fidelity or unrelated message
metadata.

#### Scenario: Original image detail with a cache boundary
- **WHEN** a user image requests original detail and an explicit cache breakpoint
- **THEN** unary and streaming native HTTP requests retain both properties
- **AND** an unmarked named user message retains its name

### Requirement: Extended native usage projection
The adapter SHALL project native prompt cache-write/image/text counts and
completion text counts, preserving field presence and validating each against
its corresponding total.

#### Scenario: Explicit zero and absent counts
- **WHEN** a native completion reports zero text tokens and omits other extended counts
- **THEN** the public result contains text_tokens zero and omits the missing fields

#### Scenario: Impossible detail count
- **WHEN** an extended count is negative or exceeds its corresponding total
- **THEN** the adapter rejects the provider output


### Requirement: Preserve the native completion-token limit field

Compatible Chat requests SHALL preserve whether the client supplied max_tokens
or max_completion_tokens. The selected field SHALL carry the effective common
MaxOutputTokens value. Raw provider-option overrides of either native token key
SHALL be rejected by the shared guard, using exact case-sensitive native keys.

#### Scenario: Reasoning client supplies a completion-token limit

- **WHEN** the Chat request supplies max_completion_tokens
- **THEN** unary and streaming compatible calls emit that field only
- **AND** a local selector is consumed without reaching the endpoint

### Requirement: Restrict callable tools with a validated subset

Chat SHALL accept allowed_tools selection with auto or required mode and validate
each function/custom reference against declared names and types without duplicate
references. Compatible and Responses attempts SHALL retain all definitions and
send typed native subsets. Anthropic attempts SHALL filter current definitions
without changing message history. The adapter SHALL reject output outside the
subset before publishing tool starts, arguments, or complete calls.

#### Scenario: A forbidden tool starts streaming

- **WHEN** a native provider starts a tool outside the selected subset
- **THEN** the adapter emits an error before exposing its name, ID or arguments

#### Scenario: Only the current callable subset changes

- **WHEN** historical calls reference tools outside the current subset
- **THEN** their historical messages remain intact
- **AND** current Anthropic tool definitions contain only selected functions

### Requirement: Preserve Responses refusals as Chat refusals

The adapter SHALL enable native Responses refusal preservation and project the
typed metadata into Chat refusal fields. Refusal-only unary output SHALL retain
null content. Streaming SHALL expose each refusal delta once and SHALL NOT append
the aggregate finish metadata again. Refusal SHALL exempt absent ordinary text
from structured-output validation and absent calls from required-tool validation.

#### Scenario: Structured output is refused

- **WHEN** Responses returns refusal instead of the requested JSON schema output
- **THEN** the official Chat client receives the refusal without a gateway error
- **AND** stream completion remains successful with one terminal finish

#### Scenario: Refusal appears only in the final native response

- **WHEN** the final native snapshot first provides refusal text
- **THEN** Chat emits that refusal before completion without duplicating it

### Requirement: Indexed stream diagnostic and refusal isolation

Content diagnostics SHALL remain associated with the choice that produced them,
including when another choice emits content before that choice next emits a chunk.
A refusal present only in an indexed finish's native metadata SHALL exempt that
choice from JSON and required-tool completion checks and appear once as a refusal
delta. A previously streamed refusal SHALL NOT be duplicated by final metadata.

#### Scenario: Source before another choice's delta
- **WHEN** choice zero emits a source and choice one emits the next text delta
- **THEN** the source remains attached to a choice-zero chunk

#### Scenario: Final-only refusal with strict output controls
- **WHEN** a choice finishes with native refusal metadata without a prior refusal delta
- **THEN** Chat emits the refusal once and can finish successfully despite requested JSON or required tool output
