# Phase 2 provider-contract delta

Baseline: `@ai-sdk/provider@4.0.7`, `@ai-sdk/gateway@4.0.52`, upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`.

The semantic paths below refer to `artifacts/semantic-requests.json`. A provider-domain loss exists only when the transport-neutral Go request values cannot preserve a valid pinned distinction for an explicit V4 mapper. Generic `encoding/json` output and current struct tags are not protocol authority.

Each witness is a passing external-package Phase 1 test. Phase 2 should invert the relevant assertion before changing the provider model.

| Valid pinned distinction | Evidence scenario and path | Current Go semantic loss | Executable witness | Required Phase 2 change |
| --- | --- | --- | --- | --- |
| Fractional `maxOutputTokens`, `topK`, and `seed` numbers | `presence-losses` `/requests/0/body/maxOutputTokens`, `/topK`, and `/seed` | Go narrows each upstream `number` to `*int`; no `CallOptions` value can carry a fraction. | `TestProviderWireV4Loss_FractionalNumericSettings` | Represent the exact numeric domain or intentionally narrow it in a separately approved contract change. |
| Explicit `includeRawChunks: false` versus absence | `unary-settings` `/requests/0/body/includeRawChunks` | The non-pointer boolean has one zero value for both semantic states. | `TestProviderWireV4Loss_ExplicitFalseIncludeRawChunks` | Use a presence-aware boolean. |
| Explicit empty optional strings versus absence for response-format name/description, function description, prompt and tool-result filenames, approval reason, and execution-denied reason | `presence-losses` and `streaming-prompt-tools` at the corresponding nested paths | Plain string fields have one zero value for both semantic states. | `TestProviderWireV4Loss_ExplicitEmptyOptionalStrings` | Use pointers or another presence-preserving representation for optional strings. |
| Explicit `providerExecuted: false` versus absence on a tool call | `presence-losses` `/requests/0/body/prompt/2/content/1/providerExecuted` | The non-pointer boolean has one zero value for both semantic states. | `TestProviderWireV4Loss_ExplicitFalseToolCallProviderExecuted` | Use a presence-aware boolean on the tool-call request arm. |
| Required inline-text file data `{type:"text",text:""}` | `presence-losses` `/requests/0/body/prompt/1/content/1/data` | Public typed fields and constructors cannot select the empty text variant: `DataContent{Text:""}` fails validation, while public JSON decoding creates a valid value with the same exported fields by setting private variant state. Provider JSON decoding is not a transport-neutral construction contract for an external V4 mapper. | `TestProviderWireV4Loss_RequiredEmptyInlineTextFileData` | Expose a presence-preserving tagged file-data representation or constructor that an external V4 mapper can inspect without treating provider JSON output as authority. |

## Explicit codec responsibilities, not provider-model deltas

An explicit V4 request mapper can preserve the following existing in-memory distinctions without changing provider types:

- nil versus non-nil empty slices and maps, including tools, stop sequences, headers, provider options, and input examples;
- required `prompt`, message content, provider-tool `args`, and other arrays or objects, emitted independently of collection length;
- required empty arm fields, emitted according to the message, content, tool, tool-choice, file-data, or result discriminator;
- required empty forced-tool names and other required strings;
- opaque provider JSON already carried by `json.RawMessage` or provider-option maps.

The future V4 codec must map these values explicitly. It must not delegate protocol output to provider-type `encoding/json` behavior.

## Other non-deltas

- JSON object ordering, whitespace, and other byte-format differences are not model losses.
- Reserved content-type collision probes are invalid strict-envelope requests and do not justify provider-model changes.
- Flat Go unions permitting inactive-arm fields are not losses when every valid arm remains representable.
- `Tool.ProviderOptions` on a provider-tool arm is extra Go permissiveness; the strict schema rejects it, but Phase 2 does not require a redesign solely for that reason.
- Nested provider-option nulls, numeric zero settings backed by pointers, function-tool `strict: false`, approval `approved: false`, and tool-result JSON null remain representable.
- TypeScript header properties with `undefined` values disappear during JSON serialization and therefore do not create a request-wire distinction requiring a Go model change.

## Coherence gate

After removing codec-only differences, the remaining losses still form one coherent Phase 2 provider-contract change: make transport-neutral request values preserve the registered numeric, optional scalar, and file-data variant distinctions that an explicit V4 mapper cannot otherwise recover. Phase 2 does not need to redesign types for required fields or non-nil empty collections. The future strict V4 codec remains responsible for their wire presence, while the existing tolerant legacy decoder stays a separately tested dialect.
