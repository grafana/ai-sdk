# Spec Delta

## MODIFIED Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil execution runtime, which owns the request selector and the stream idle and drain durations under gateway-execution-runtime. Construction SHALL NOT accept a total model duration, or request, response, frame or stream-part size limits.

#### Scenario: Valid construction
- **WHEN** a caller supplies a constructed execution runtime
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the runtime is nil
- **THEN** construction SHALL fail before serving traffic

### Requirement: Host selection preserves mapped options
The selector SHALL consume validated call-level host controls under authenticated request policy and return a non-nil executable model and logical identity. The execution runtime SHALL retain the mapped native options after removing call-level Gateway controls; model selection SHALL NOT require returning or replacing those options. Configured catalog resolution and request-only BYOK selection SHALL be separate host implementations; neither the runtime nor the wire package SHALL require BYOK to implement a catalog.

#### Scenario: Host selection returns an executable model
- **WHEN** a configured or request-only selector returns model and identity
- **THEN** the runtime SHALL retain mapped native options after consuming Gateway controls without requiring the selector to return or replace them

### Requirement: Reserved provider options and protected call headers
Call-level `gateway` controls SHALL be consumed only by the selector, and protected call headers SHALL be refused, both by the execution runtime under gateway-execution-runtime. No other namespace or scope SHALL be reserved. Native-consumption controls SHALL prevent account/model/transport bypass before I/O without blanket filtering or value-echoing refusals.

#### Scenario: Host-control and header processing policy
- **WHEN** the handler maps host controls, native options and call headers
- **THEN** the handler SHALL map every provider-option namespace at every supported scope, including `gateway`, `grafana` and `grafana-ai-sdk`, as an ordinary opaque namespace.
- **AND** The execution runtime SHALL remove call-level `gateway` before selection and pass it to the selector; it SHALL never reach native adapters as a provider option.
- **AND** The configured selector SHALL reject call-level `gateway` controls with the fixed reserved-provider-options document, while request-only selection SHALL follow gateway-request-byok.
- **AND** Unknown ordinary namespaces and fields SHALL NOT be treated as host controls merely because they are unlisted.
- **AND** The execution runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering the set gateway-execution-runtime lists.
- **AND** The openai and openai-compatible providers apply call headers after setting their own authorization header, so an accepted credential-bearing header would choose the credential presented to those backends.
- **AND** Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.
- **AND** Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass.
- **AND** A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass.
- **AND** Native-specific checks SHALL fail before external provider I/O; schema and mapping refusals SHALL precede the protected-header refusal, and both SHALL precede selection.
- **AND** These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a configured request carries call-level `providerOptions.gateway`
- **THEN** the response SHALL be the fixed reserved-provider-options invalid-request document naming neither the namespace contents nor the caller's values
- **AND** no model SHALL be invoked

#### Scenario: Former host namespaces are ordinary
- **WHEN** a request carries `grafana` or `grafana-ai-sdk` at any supported scope, or `gateway` at a message, part or tool scope
- **THEN** the request SHALL succeed and the model SHALL receive the namespace unchanged at its scope

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

#### Scenario: Protected call header beside a mapping failure
- **WHEN** a request carries a credential-bearing body header and also fails a mapping rule
- **THEN** the response SHALL be the mapping failure's fixed invalid-request document, and no model SHALL be resolved or invoked

#### Scenario: Protocol header in the body
- **WHEN** a request carries a protocol header name such as `AI-Language-Model-Id` as a body-carried call header
- **THEN** the request SHALL be accepted, because the HTTP contract retains that name in the body
- **AND** the name SHALL NOT reach the selected backend

#### Scenario: Reserved namespace in a different case
- **WHEN** a request carries a call-level provider-option namespace such as `Gateway`
- **THEN** it SHALL be mapped like any other ordinary namespace and SHALL NOT reach the selector, because namespace names are compared exactly

### Requirement: Protection processing order and fixed refusal privacy

Native-specific checks SHALL fail before external provider I/O. Schema and mapping refusals SHALL precede the protected-header refusal, and all of them SHALL precede selection. Provider-option and protected-header refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Protection processing order and fixed refusal privacy
- **WHEN** a protected call header or native bypass is detected
- **THEN** schema, mapping and header refusals SHALL remain pre-resolution, native-specific checks SHALL precede external I/O and fixed documents SHALL echo no offending names or values

### Requirement: Resolution and bounded model invocation
The handler SHALL select and invoke one logical model through the execution runtime under authenticated request policy. Configured access SHALL resolve its catalog once; BYOK SHALL use request accounts without a catalog. Both SHALL retain logical identity and run selection and execution synchronously under the request context's deadline.

#### Scenario: Selection and shared execution policy
- **WHEN** a supported request enters host selection and invocation
- **THEN** For a supported request, the runtime SHALL select execution once under authenticated request policy and invoke the resulting logical model once.
- **AND** Configured selection SHALL resolve the exact catalog ID/alias; BYOK selection SHALL use only request provider/model/credentials.
- **AND** Selection SHALL return a nonempty valid-UTF-8 logical identity and a non-nil V4 model.
- **AND** Logical identity SHALL NOT substitute for native response identity, including when a native modelId equals it.
- **AND** The request context's deadline, set by the host, SHALL be the one execution deadline. It SHALL cover selection/construction, logical invocation and all underlying credential attempts, and streaming consumption SHALL use the same deadline.
- **AND** The handler SHALL NOT add a deadline of its own, and selection, invocation and attempt boundaries SHALL NOT reset or extend the request deadline.
- **AND** When the request context's deadline expires, the handler SHALL classify the outcome as a timeout; when the request context is canceled, it SHALL classify it as a cancellation.
- **AND** Selection and invocation SHALL run on the request goroutine with panic containment; a result returned after the request context ended SHALL be discarded.
- **AND** A selection returned after the request context ended SHALL NOT invoke a model.
- **AND** A selector or native function that ignores cancellation SHALL hold its own request until it returns; no exactly-once or guaranteed-zeroization claim SHALL be made.

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
- **WHEN** cancellation or the request deadline becomes observable during selection or any attempt
- **THEN** no later credential attempt SHALL begin and the handler SHALL return the context outcome once the work returns

#### Scenario: Request deadline is a timeout
- **WHEN** the request context's deadline expires during a unary attempt
- **THEN** the handler SHALL return the fixed timeout error, not the cancellation error

#### Scenario: Selector consumes the execution budget
- **WHEN** selection completes only after the request deadline expired
- **THEN** unary and streaming requests SHALL receive a pre-commit timeout JSON failure with zero model invocations

#### Scenario: Model returns after the deadline
- **WHEN** `DoGenerate` returns a valid result only after the request deadline expired
- **THEN** the handler SHALL return the fixed timeout error and SHALL NOT write the result

#### Scenario: Selection leaves only a partial budget
- **WHEN** selection consumes part of the time before the request deadline and returns a credential-attempt model
- **THEN** logical invocation, every credential attempt and streaming consumption SHALL receive only the remaining time, without a fresh deadline

#### Scenario: Request without a deadline
- **WHEN** the request context carries no deadline
- **THEN** the handler SHALL run selection and invocation without a deadline of its own, and caller cancellation SHALL still end them

### Requirement: Protected-header edge evidence and outer authentication isolation

The plugin host authenticates outer requests under gateway-plugin-host, and the Gateway refuses no outer header names of its own. The credential-header refusal set SHALL cover the platform credential and tenant headers the host delegates, and the native provider credential headers. Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.

#### Scenario: Protected-header edge evidence and outer authentication isolation
- **WHEN** a test sends each listed credential header name as a body call header, plus an ordinary header as a negative control
- **THEN** the runtime SHALL refuse every listed name and accept the ordinary header, without forwarding unrelated outer headers or coupling outer auth to body headers

## REMOVED Requirements

### Requirement: Bounded unary invocation worker and provider-owned blockage
**Reason**: The handler no longer runs `DoGenerate` on a worker goroutine. Returning before a model that ignores cancellation did only helped callers the platform's route deadline already answers, and the worker forced late-result and capture handoff code.
**Migration**: None for callers. Invocation runs synchronously under gateway-execution-runtime's "Synchronous unary invocation": panics and `nil, nil` results still fail as internal, and late results are discarded.
