## Why

The repository had executable ProviderWire V4 contract evidence but no production Go handler that could serve unary text requests from the registered Gateway client.

## What Changes

- Add a production unary `POST /language-model` handler with exact envelope checks, bounded UTF-8 body processing, complete request-schema validation, explicit text/scalar mapping, catalog resolution, and bounded model invocation.
- Reject schema-valid deferred capability families before resolution without defining precedence for requests that activate several families.
- Normalize runtime failures into fixed privacy-safe Gateway error documents.
- Emit only unary fields consumed semantically by supported SDK clients: content, finish reason, and usage.
- Normalize provider-default reasoning to the provider-domain zero value.
- Replay committed request goldens and add pinned Gateway client integration while leaving streaming deferred.

## Capabilities

### New Capabilities
- `providerwire-v4-unary-runtime`: Production unary text execution and compatibility evidence.

### Modified Capabilities
- `effort-level`: Provider-default reasoning uses zero-value semantics.
- `provider-v4-core-types`: Provider calls receive value-typed reasoning.
- `provider-v4-content-model`: Existing reasoning content remains distinct from reasoning effort.
- `providerwire-v4-http-contract`: Production replay and client integration supplement the recorded contract.
- `typed-string-enums`: Reasoning remains a typed enum with an empty provider-default value.

## Impact

- Primary code: `gateway/providerwire/v4`, provider reasoning call sites, and catalog integration.
- Tests: raw Go runtime tests, committed ProviderWire golden replay, and cross-language integration through `@ai-sdk/gateway@4.0.52`.
- Protocol scope: unary text only. Streaming, tools, files, structured output, provider options, body-header forwarding, and raw output execution remain deferred.
