## ADDED Requirements

### Requirement: Selected-backend provider options

After resolution, the handler SHALL forward only the provider options the resolved backend reads, at call, message and content-part level, as described by the resolved model's `catalog.ProviderOptionPolicy`. A namespace outside the policy's namespaces SHALL NOT reach the model, and SHALL NOT fail the request, because an option for another backend is one every provider ignores. Where the policy lists fields for a namespace, other top-level fields SHALL be removed, compared after folding case and removing `_` and `-`; a namespace with nothing removed SHALL keep its bytes exactly. The zero-value policy SHALL forward no provider options.

The command SHALL set a policy for every provider type it constructs. For `anthropic`, the policy SHALL forward the `anthropic` namespace restricted to the fields `providers/anthropic` reads, excluding the fields the runtime refuses, and a test SHALL fail when the provider's typed option structs gain a field that is neither forwarded nor refused. For `openai-compatible`, the policy SHALL forward the namespaces the provider reads for its configured provider name, without restricting fields, and a test SHALL check those namespaces against the provider itself.

#### Scenario: Options for another backend
- **WHEN** a request to an openai-compatible model carries `anthropic` and `openai` provider options alongside its own namespace
- **THEN** the model SHALL receive only its own namespace, byte for byte
- **AND** the response SHALL succeed

#### Scenario: Unclassified Anthropic field
- **WHEN** a request to an Anthropic model carries an `anthropic` field the policy does not list
- **THEN** that field SHALL be removed before the model runs, and the listed fields SHALL keep their values

#### Scenario: Unclassified backend
- **WHEN** a resolved model carries the zero-value policy
- **THEN** it SHALL receive no caller provider options

## MODIFIED Requirements

### Requirement: Reserved provider options and protected call headers

The runtime SHALL reserve the `grafana` provider-option namespace for the host. A request carrying it SHALL be rejected with a stable invalid-request document before resolution or model invocation, rather than silently stripped, so nothing it carries reaches a backend and the caller learns the option was refused.

The runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering at least `authorization`, `proxy-authorization`, `x-access-token`, `x-grafana-id`, `x-api-key`, `api-key`, `openai-api-key` and `anthropic-api-key`. The openai and openai-compatible providers apply call headers after setting their own authorization header, so an accepted credential-bearing header would choose the credential presented to those backends. The refused set SHALL cover every header name the inbound authenticated edge refuses in outer headers, which a test SHALL assert by driving the edge with a valid stack assertion and each candidate name, with an accepted ordinary header as a negative control, and requiring the body mapper to refuse every name the edge refused.

The runtime SHALL refuse a provider-option namespace carrying a field that names a decision the runtime has already made, covering at least `model`, `fallbacks`, `messages`, `prompt`, `tools`, `toolChoice`, `functions`, `function_call`, `mcpServers`, `container`, `responseFormat`, `stream`, `streamOptions`, and the message and part fields `role`, `tool_calls`, `tool_call_id`, `type`, `image_url`, `input_audio` and `file`, which providers spread over the entry they build and which would otherwise restore tools or files. Field names SHALL be compared after folding case and removing `_` and `-`, because a provider may read a field under another spelling: `providers/anthropic` decodes options with `encoding/json`, which matches names case-insensitively, so `MCPServers` and `mcp_servers` reach the same field. The set SHALL cover every capability the runtime refuses at the wire level, so a refused capability cannot be restated as a provider option. Providers merge unknown option fields into the request body, so an accepted model, fallbacks or prompt field would redirect the call away from the resolved catalog model that telemetry reports, an accepted tool field, including the legacy `functions` pair, would run tools the runtime never mapped on the host's credentials, an accepted response-format field would restate the structured output the runtime refuses at the wire level, and an accepted stream field would answer in a transport the runtime is not reading.

These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a request carries the `grafana` provider-option namespace
- **THEN** the response SHALL be a stable invalid-request document naming neither the namespace contents nor the caller's values
- **AND** no model SHALL be resolved or invoked

#### Scenario: Protected provider option
- **WHEN** a request carries a provider-option field naming the model, the prompt, or server-side tools
- **THEN** the response SHALL be a stable invalid-request document
- **AND** no model SHALL be resolved or invoked

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
- **THEN** it SHALL be mapped like any other namespace, because namespace names are compared exactly
