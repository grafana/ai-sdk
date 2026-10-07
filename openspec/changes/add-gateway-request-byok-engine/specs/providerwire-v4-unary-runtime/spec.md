## MODIFIED Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil host-owned request selector independent of catalog interfaces and positive limits for request bytes, unary response bytes, provider stream-part count, complete SSE frame bytes, total model duration, stream idle duration, and bounded post-cancellation drain duration. Request and unary byte limits and the stream-part limit SHALL support safe `limit+1` arithmetic, and the frame limit SHALL contain the fixed start and stream-error frames.

#### Scenario: Valid construction
- **WHEN** a caller supplies a request selector and valid limits
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the selector is nil, a limit is non-positive, or a byte limit cannot safely use `limit+1`
- **THEN** construction SHALL fail before serving traffic

The selector SHALL consume validated call-level host controls under authenticated request policy and return a non-nil executable model, credential-free native options and logical identity. Configured catalog resolution and request-only BYOK selection SHALL be separate host implementations; the wire package SHALL NOT require BYOK to implement a catalog.

### Requirement: Reserved provider options and protected call headers

The runtime SHALL reserve the `grafana`, `gateway`, and `grafana-ai-sdk` provider-option namespaces for the host. A request carrying any of them SHALL be rejected with a stable invalid-request document before resolution or model invocation unless an owning host feature explicitly consumes it. Such controls SHALL never reach native adapters as provider options. Call-level gateway controls SHALL be consumed by the host selector before native middleware; the catalog selector SHALL reject them, while request-only selection SHALL follow gateway-request-byok. Nested host namespaces and unsupported controls SHALL remain rejected before provider I/O. Unknown ordinary namespaces and fields SHALL NOT be treated as host controls merely because they are unlisted.

The runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering at least `authorization`, `proxy-authorization`, `x-access-token`, `x-grafana-id`, `x-api-key`, `api-key`, `openai-api-key` and `anthropic-api-key`. The openai and openai-compatible providers apply call headers after setting their own authorization header, so an accepted credential-bearing header would choose the credential presented to those backends. The refused set SHALL cover every header name the inbound authenticated edge refuses in outer headers, which a test SHALL assert by driving the edge with a valid stack assertion and each candidate name, with an accepted ordinary header as a negative control, and requiring the body mapper to refuse every name the edge refused. Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.

Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass. A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass. Native-specific checks SHALL fail before external provider I/O; schema, reserved-host and protected-body-header refusals SHALL retain their pre-resolution processing order.

These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a request carries the `grafana`, `gateway`, or `grafana-ai-sdk` provider-option namespace without an owning feature consuming it
- **THEN** the response SHALL be a stable invalid-request document naming neither the namespace contents nor the caller's values
- **AND** no model SHALL be resolved or invoked

#### Scenario: Protected provider option
- **WHEN** an option can cause a concrete native model/prompt, credential/account/destination, role/union, tool-ownership or transport/execution bypass
- **THEN** the request SHALL fail with a stable invalid-request document before external provider I/O unless native precedence already prevents the override

#### Scenario: Harmless field with the same spelling
- **WHEN** an ordinary option field has a spelling used by a protected field but its namespace and scope cannot cause the native bypass
- **THEN** it SHALL remain intact for native interpretation rather than be refused under a universal blacklist

#### Scenario: Protected call header
- **WHEN** a request carries a credential-bearing body header in any letter case
- **THEN** the response SHALL be a stable invalid-request document
- **AND** no model SHALL be resolved or invoked

#### Scenario: Protocol header in the body
- **WHEN** a request carries a protocol header name such as `AI-Language-Model-Id` as a body-carried call header
- **THEN** the request SHALL be accepted, because the HTTP contract retains that name in the body
- **AND** the name SHALL NOT reach the selected backend

#### Scenario: Reserved namespace in a different case
- **WHEN** a request carries a provider-option namespace such as `Grafana`
- **THEN** it SHALL be mapped like any other ordinary namespace, because namespace names are compared exactly


[159 more lines in file. Use offset=138 to continue.]


### Requirement: Resolution and bounded model invocation
For a supported request, the handler SHALL select execution once under authenticated request policy and invoke the resulting logical model once. Configured selection SHALL resolve the exact catalog ID/alias; BYOK selection SHALL use only request provider/model/credentials. Selection SHALL return a nonempty valid-UTF-8 logical identity and a non-nil V4 model. Logical identity SHALL NOT substitute for native response identity, including when a native modelId equals it.

One request execution deadline SHALL begin immediately before host selection, after bounded protocol input validation, and SHALL cover selection/construction, logical invocation and all underlying credential attempts; streaming consumption SHALL use the same deadline. Selection, invocation and attempt boundaries SHALL NOT reset or extend it. An earlier request-context deadline SHALL remain effective. Selection and invocation SHALL retain context cancellation, panic containment, buffered completion and bounded handler latency when work ignores cancellation. Late selection completion SHALL NOT invoke a model and SHALL retain bounded request-owned cleanup. A permanently blocked selection or native function may retain its worker; no exactly-once or guaranteed-zeroization claim SHALL be made.

#### Scenario: Supported execution
- **WHEN** configured selection returns a valid model
- **THEN** catalog resolution and logical DoGenerate SHALL each run once

#### Scenario: BYOK execution
- **WHEN** BYOK selection returns a valid credential-attempt model
- **THEN** the logical invocation SHALL run once, the catalog SHALL not run and bounded physical attempts SHALL follow the BYOK contract

#### Scenario: Invalid resolution
- **WHEN** selection fails, panics or returns invalid identity/model values
- **THEN** the handler SHALL return a safe error without invoking an invalid model

#### Scenario: Cancellation or timeout
- **WHEN** cancellation or the execution deadline becomes observable during selection or any attempt
- **THEN** handler latency SHALL remain bounded and no later credential attempt SHALL begin

#### Scenario: Selector consumes the execution budget
- **WHEN** selection ignores cancellation or completes only after the shared deadline expires
- **THEN** unary and streaming handler latency SHALL remain bounded, with a pre-commit JSON failure, zero model invocations and bounded cleanup of any late selection result

#### Scenario: Selection leaves only a partial budget
- **WHEN** selection consumes part of the configured execution duration before returning a credential-attempt model
- **THEN** logical invocation, every credential attempt and streaming consumption SHALL receive only the remaining time, without a fresh deadline
