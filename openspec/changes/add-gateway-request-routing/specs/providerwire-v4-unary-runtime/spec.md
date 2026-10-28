# Spec Delta

## MODIFIED Requirements

### Requirement: Reserved provider options and protected call headers
Call-level `gateway` controls SHALL be consumed only by the selector, and protected call headers SHALL be refused, both by the execution runtime under gateway-execution-runtime. No other namespace or scope SHALL be reserved. Native-consumption controls SHALL prevent account/model/transport bypass before I/O without blanket filtering or value-echoing refusals.

#### Scenario: Host-control and header processing policy
- **WHEN** the handler maps host controls, native options and call headers
- **THEN** the handler SHALL map every provider-option namespace at every supported scope, including `gateway`, `grafana` and `grafana-ai-sdk`, as an ordinary opaque namespace.
- **AND** The execution runtime SHALL remove call-level `gateway` before selection and pass it to the selector; it SHALL never reach native adapters as a provider option.
- **AND** Configured and request-only selection SHALL apply call-level `gateway` controls under gateway-request-routing and gateway-request-byok, and SHALL refuse any other control with the fixed unsupported-gateway-control document.
- **AND** Unknown ordinary namespaces and fields SHALL NOT be treated as host controls merely because they are unlisted.
- **AND** The execution runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering the set gateway-execution-runtime lists.
- **AND** The openai and openai-compatible providers apply call headers after setting their own authorization header, so an accepted credential-bearing header would choose the credential presented to those backends.
- **AND** Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.
- **AND** Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass.
- **AND** A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass.
- **AND** Native-specific checks SHALL fail before external provider I/O; schema and mapping refusals SHALL precede the protected-header refusal, and both SHALL precede selection.
- **AND** These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a configured request carries a call-level `providerOptions.gateway` member other than `models`, `order` or `only`
- **THEN** the response SHALL be the fixed unsupported-gateway-control document naming neither the namespace contents nor the caller's values
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

## ADDED Requirements

### Requirement: Fixed routing error documents
Routing and gateway-control failures SHALL use fixed ProviderWire documents with `param: null`, not retryable, chosen from category and reason alone. Their messages SHALL never echo supplied model IDs, slugs, numbers or control names.

#### Scenario: Routing documents
- **WHEN** selection fails with each routing outcome
- **THEN** the documents SHALL be:
  - unsupported gateway control: HTTP 400, `invalid_request_error`, `invalid_request`, "unsupported gateway control; supported controls are models, order and only, plus byok and providerTimeouts.byok for request credentials"
  - routing controls: HTTP 400, `invalid_request_error`, `invalid_request`, "providerOptions.gateway models, order and only must be arrays of nonempty strings; conditional models entries are unsupported"
  - provider timeouts: HTTP 400, `invalid_request_error`, `invalid_request`, "providerOptions.gateway.providerTimeouts.byok must map anthropic or openai to whole milliseconds from 1 to 600000"
  - no candidates: HTTP 400, `invalid_request_error`, `invalid_request`, "providerOptions.gateway.only excludes every candidate"
  - attempt bound: HTTP 400, `invalid_request_error`, `invalid_request`, "routing exceeds the bound of 8 provider attempts"
  - fallback model not found: HTTP 404, `model_not_found`, `model_not_found`, "a providerOptions.gateway.models entry is not an available model"

#### Scenario: Client classification of routing documents
- **WHEN** the pinned TypeScript client and the Go client receive each routing document
- **THEN** each SHALL classify it with its existing invalid-request or model-not-found error category, as not retryable
