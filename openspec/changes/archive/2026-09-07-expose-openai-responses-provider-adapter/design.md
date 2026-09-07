## Context

The registered upstream baseline, `@ai-sdk/openai@4.0.41` and
`@ai-sdk/amazon-bedrock@5.0.55` at Vercel AI SDK commit `d76eb85a`, exposes a
dedicated `@ai-sdk/amazon-bedrock/mantle` provider. That provider owns Mantle
routing and authentication while importing OpenAI's internal Chat and Responses
model implementations.

The Go providers are separate modules. `providers/bedrock` cannot import a Go
`internal` package beneath the sibling `providers/openai` module, and the
OpenAI model currently constructs its own API-key client. A narrow exported
composition seam is therefore required before a Bedrock-owned Mantle provider
can reuse Responses behavior without duplication.

## Goals / Non-Goals

**Goals:**

- Provide the Go equivalent of upstream's OpenAI-internal model reuse seam.
- Preserve provider-owned official SDK client configuration.
- Let integrating providers report their own model identity while preserving the OpenAI/Azure metadata namespace required for continuation state.
- Keep existing direct OpenAI behavior unchanged.

**Non-Goals:**

- Expose Bedrock Mantle through the OpenAI provider.
- Implement Mantle authentication, endpoint selection, or model policy here.
- Close the Mantle Chat or Responses parity gap in this change.
- Change request conversion, response mapping, or wire output.

## Decisions

### Accept the official top-level OpenAI client

Expose `NewResponsesWithClient(client openai.Client, modelID string, opts
...Option)`. Both standard and provider-specific official SDK constructors
return this value, and retaining its Responses service preserves client-level
configuration. Accepting `responses.ResponseService` was rejected because the
official SDK directs callers to construct services through the top-level
client. A `WithClient` option was rejected because it would leave callers
supplying a meaningless API key to the existing constructor.

Replaying `client.Options` through `WithRequestOptions` was tested and rejected:
the current model applies those options during client construction and again at
method invocation, which duplicates provider authentication finalizers.

### Separate provider identity from OpenAI metadata and call options

Add `WithProviderName` to control only `Provider()`, ignoring empty names to
preserve the valid default identity consistently with
`providers/openai-compatible`. This mirrors upstream's distinction between the
configured provider identity and its `providerOptionsName`. Request options and
response metadata remain under the resolved `"openai"` or `"azure"` namespace
because request conversion uses that namespace to recover item IDs, encrypted
reasoning, tool namespaces, and approval correlation on later calls.

### Keep the seam provider-focused

Do not add a Bedrock example to the OpenAI user guide or a Bedrock dependency to
the OpenAI module. The dedicated Mantle provider and its SigV4 tests belong in
`providers/bedrock/mantle` after this prerequisite is available as a resolvable
module version.

### Record, but do not claim, Mantle parity

Update the parity manifest and coverage map to identify the pinned upstream
Mantle subprovider as a gap. The follow-up provider change will narrow that gap
for Responses; Chat remains separate work unless implemented with equivalent
evidence.

## Risks / Trade-offs

- **The composition seam is exported public API** → Document it explicitly for
  provider integrations and keep its surface limited to the official client,
  model ID, and existing model options.
- **The concrete client couples the module to `openai-go`** → The module already
  publicly exposes that SDK's request options and pins its major version; using
  the top-level client is safer than exposing lower-level service internals.
- **Injected model request options can conflict with provider client policy** →
  Provider-specific clients remain responsible for validating protected
  endpoint and authentication settings at request finalization.
