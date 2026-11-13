# Design

## Context

Upstream reference: `ai@7.0.159`, commit `028a0253`, fetched with `gh api repos/vercel/ai/contents/<path>?ref=ai@7.0.159` (the tag resolves to the registered commit).

- Upstream `defaultSettingsMiddleware` is `mergeObjects(settings, params)`. `mergeObjects` skips `undefined` overrides, merges plain objects recursively, replaces arrays, primitives and `null`, and ignores `__proto__`, `constructor` and `prototype` keys. Its settings type lists the same fields as Go except `reasoning`; `LanguageModelV4CallOptions` has `reasoning`, so `mergeObjects` would merge it at runtime.
- Upstream `generateText`/`streamText` always set `toolChoice` (`prepareToolChoice` defaults to `{type: 'auto'}`) and send `tools: undefined` for an empty tool set (`prepareTools`).
- Go `DefaultSettings` fills nil pointers and nil slices, merges `Headers` and `ProviderOptions` one level deep, and replaces `ResponseFormat` and `ToolChoice` whole.
- The root module already ports `mergeObjects` for step provider options (`mergeStepProviderOptions` in `provider_options.go`): it marshals both maps to JSON, merges recursively, ignores the three prototype keys, and returns every namespace as `RawProviderOption`. When one side is nil it returns the other unchanged.
- `provider.ResponseFormat` and `provider.ToolChoice` are flat structs representing unions. The Grafana client rejects a text format with schema, name or description, and a non-named tool choice with a tool name. Native adapters act only on members of the active type.
- ProviderWire maps `tools: []` to nil and `responseFormat: {type: "text"}` to nil. Its unary-runtime spec requires empty tools and text format to normalize to ordinary text behavior. Upstream `@ai-sdk/gateway@4.0.132` sends call options as given, so the wire can carry `tools: []`. No Gateway path on `main` runs `DefaultSettings` after ProviderWire decoding.

## Goals / Non-Goals

**Goals:**
- Match upstream merge results wherever Go types can represent them.
- One Go implementation of the `mergeObjects` rules for provider options.

**Non-Goals:**
- Changing ProviderWire decoding or its wire contract.
- Changing the empty-tool-set shape produced by `StreamText`. It sends an empty non-nil `Tools` slice where upstream `prepareTools` sends `undefined`, so `DefaultSettings` tools do not apply. This is a core orchestration difference to track separately.
- A generic deep merge over `CallOptions` or new public API.

## Decisions

### Provider options: share the existing `mergeObjects` port

Move the merge from `provider_options.go` into a root-module internal package. `StreamText` step preparation keeps calling the existing whole-map merge (`Merge`) unchanged. `DefaultSettings` calls a second entry point (`MergeOverlapping`) that shares the same recursive object merge but only merges namespaces present on both sides. Errors are returned from `TransformParams`.

- Alternative: a second copy in `middleware`. Rejected as duplication of a security-relevant rule set.
- Alternative: an exported `provider` helper. Rejected: new public API with no other consumer.
- Alternative: reuse `Merge` as is. Rejected: it turns every namespace into `RawProviderOption`, breaking custom models that type-assert a typed option set on only one side. Changing `Merge` instead would change `prepareStep` behavior.
- Consequence: only namespaces that are JSON objects on both sides reach the model as `RawProviderOption`; providers resolve them with `provider.ResolveOption`. Other namespaces keep their original values, and only the shared namespaces are marshaled.

### Response format: merge active members only

Merge field by field: the caller's `Type`, or the default's when empty; then, for a JSON result only, fill an unset `Schema`, `Name` and `Description` from the default. A non-JSON result takes no members from the default and keeps the caller's value otherwise unchanged.

- Alternative: literal deep merge, giving `{type: text, schema: ...}`. Rejected: no Go adapter acts on those members and the Grafana client rejects them, so it would only add failures.
- Alternative: keep whole replacement. Rejected: a JSON caller without a schema would lose the default schema, which upstream keeps.
- Classification: fixes an implementation bug, with dropping inactive members as a recorded deviation.

### Tool choice: keep whole replacement

Upstream deep merge of `{type: 'tool', toolName}` with `{type: 'auto'}` yields `{type: 'auto', toolName}`. Since `StreamText` always sets a choice, as upstream does, the only difference is the inactive tool name. Keep replacement and record it with the response-format deviation.

### Empty slices and ProviderWire

Keep nil-only filling for slices; it matches upstream for `stopSequences: []` and `tools: []`. The spec text claiming zero-length slices are filled is corrected.

ProviderWire's empty-to-nil normalization is specified behavior, and no adapter distinguishes empty from nil. It becomes observable only if a Gateway extension applies `DefaultSettings` tools or response format after decoding. That would be a ProviderWire contract change and is out of scope here.

### Reasoning: keep as a Go addition

Removing `Reasoning` would break callers for no parity gain, since upstream would merge the value at runtime. Record it in `knownGaps`. This includes the limitation that an explicit `provider-default` caller value equals Go's zero value and receives the default.

### Parity records

Add one `knownGaps` entry, `default-settings-go-representation` (surface `core-middleware`, status `intentional-deviation`), covering the `Reasoning` addition and the union-member adaptation. `PARITY.md` is unchanged because the surface and its evidence location stay the same.

## Risks / Trade-offs

- [A caller relied on a namespace replacing the default's keys] → Upstream semantics; called out in the PR and changelog. Removing a default key requires a caller value such as `null`.
- [A custom model type-asserts a typed provider option] → Only a namespace set on both sides becomes `RawProviderOption`. Document `ResolveOption` in the `DefaultSettings` godoc.
- [Merged namespace JSON is re-encoded] → Keys reorder, but numbers are preserved through `json.Number`. No semantic change.
- [`TransformParams` can now fail] → Only when a namespace set on both sides cannot be marshaled, which would also fail at the provider.

## Migration Plan

None. This is a library patch release; rollback is reverting the commit.

## Open Questions

None.
