# Design

## Context

- **Registered baseline:** `ai@7.0.159`, commit `028a025398782f4938303f48fbeee45c79af544b` (`test/conformance/upstream.yaml`). The local upstream checkout isn't readable here, so the source was fetched with `gh api repos/vercel/ai/contents/<path>?ref=ai@7.0.159`. The tag resolves to the registered commit. Files read:
  - `packages/ai/src/middleware/default-settings-middleware.ts` and its test;
  - `packages/ai/src/middleware/default-instructions-middleware.ts` and its test;
  - `packages/ai/src/util/merge-objects.ts`;
  - `packages/ai/src/prompt/prepare-language-model-call-options.ts`;
  - `packages/ai/src/prompt/prompt.ts`.
- **Go middleware.**
  - `middleware.DefaultInstructions(messages...)` is the existing port: it prepends only when no prompt message has the system role, and it copies the prompt.
  - `middleware.DefaultSettings(options)` fills nil pointers and nil slices. It merges `Headers` and `ProviderOptions` one level deep, and it also supports `ResponseFormat`, `Reasoning`, `Tools` and `ToolChoice`.
  - Neither middleware reads `TransformParamsInput.Type` or `Model`.
- **Tenant extension path:**
  - `tenanttype` is the public registry. It ships empty, and `Type.Bind` returns the tenant definition.
  - `kinds/middleware_members.cue` is hand-written, one structural member per type. A `manifestdata` test checks that each member mirrors its type's schema and accepts its example.
  - Tenant schemas use closed objects and arrays only, and bound integers to ±(2^53−1). Admission validates through float64, as upstream's JS numbers do.
  - #765's cache reads each namespace as the plugin, with a 1-minute TTL. `TenantCacheConfig` is reachable only from Go tests, not from the built plugin.
- **Transforms.** Tenant transforms run before operator transforms. The host checks each result against request policy (no protected headers, no `gateway` provider-option namespace). A transform must return copies, and the host clones only the top-level header, provider-option and prompt containers.
- **ProviderWire decoding:**
  - JSON `null` and an absent field both become nil.
  - `stopSequences: []` stays a non-nil empty slice.
  - `topK`, `seed` and `maxOutputTokens` decode into `*int`, so a fraction refuses the request.
  - An empty `tools` list becomes nil.
- **Plugin targets:** Linux amd64 and arm64 only, so Go's `int` is 64-bit.
- **End-to-end suite:**
  - Grafana runs with a fake Anthropic that answers every call with fixed text.
  - `accounts.ts` mints service-account tokens with Playwright request contexts. Those contexts send the token as a header, and the minting response contains it.
  - Traces are recorded on the first retry, and CI uploads the report of a failed run.
  - `scripts/e2e.sh` checks the API-access record but doesn't print it.
  - Tests run fully parallel, in the `default` namespace.

## Goals / Non-Goals

**Goals:**
- Two shipped, off-by-default tenant extensions with upstream semantics and closed, public schemas.
- The first end-to-end proof of tenant configuration against Grafana: writes, roles, refusals and runtime effect.
- No production knob, and no test code in the shipped backend.

**Non-Goals:**
- Changing the root `middleware.DefaultSettings` merge (see Parity).
- Stack-catalog sources (#757). Both definitions apply to every source, so stack catalogs are covered once that source exists.
- A settings UI.
- Provider-specific bounds.
- Per-message provider options.

## Decisions

### Type: `default-instructions`

- Name `default-instructions`, member `defaultInstructions`, `EnabledByDefault: false`.
- Description: "Default system messages for calls that have none."

```json
{
  "type": "object",
  "properties": {
    "messages": {
      "type": "array",
      "description": "System messages prepended, in order, to calls whose prompt has no system message.",
      "minItems": 1,
      "items": {
        "type": "object",
        "properties": {
          "content": { "type": "string", "minLength": 1, "description": "The system message text." }
        },
        "required": ["content"],
        "additionalProperties": false
      }
    }
  },
  "required": ["messages"],
  "additionalProperties": false
}
```

- Example: `{"messages":[{"content":"You are an assistant for this Grafana stack. Answer concisely."}]}`.
- Shape: upstream accepts a string, a system message or a list of them (`Instructions`). The projection can't express a union, so the member uses the list form with `content` only. Multiple messages keep upstream's ordering. Fields can be added later, optionally, as the additive evolution rule allows.
- No `providerOptions` per message. Upstream keeps them, but tenants mustn't set provider options on calls with operator-owned keys.

### Type: `default-settings`

- Name `default-settings`, member `defaultSettings`, `EnabledByDefault: false`.
- Description: "Default sampling and limit settings for calls that leave them unset."
- Fields, each with a description:

| Field | Type | Constraints (admission only) |
|---|---|---|
| `temperature`, `topP`, `presencePenalty`, `frequencyPenalty` | `number` | none |
| `topK`, `seed` | `integer` | ±(2^53−1) |
| `maxOutputTokens` | `integer` | 1 to 2^53−1 |
| `stopSequences` | array of `string` | none |

- The root is an object with `additionalProperties: false` and `minProperties: 1`. Nothing is required.
- Example: `{"temperature":0.2,"maxOutputTokens":2048,"stopSequences":["END"]}`.
- The integer bounds are the safe-integer range, which upstream's JS numbers can represent exactly. The host canonicalizes the member through float64, so `1.0` and `4e1`, which JSON Schema treats as integers, reach `Decode` as `1` and `40`. A number beyond the float64 range, such as `1e400`, fails admission's decoding. The `maxOutputTokens` bound mirrors upstream's `maxOutputTokens >= 1` call-setting rule. `minProperties` refuses a configuration that would do nothing.

### Member mirror

| Rule | `defaultInstructions` | `defaultSettings` |
|---|---|---|
| Every node has one JSON type | yes | yes |
| Objects declare properties with `additionalProperties: false` | root, message item | root |
| No empty object, open object or combinator | none | none |
| Arrays declare `items` | `messages` | `stopSequences` |
| Dropped into admission | `minItems`, `minLength` | `minimum`, `maximum`, `minProperties` |
| Example accepted by the member | only declared fields | only declared fields |

Hand-written members:

```cue
middlewareMembers: {
	// Default system messages for calls that have none.
	defaultInstructions?: {
		// System messages prepended, in order, ...
		messages: [...{
			// The system message text.
			content: string
		}]
	}
	// Default sampling and limit settings for calls that leave them unset.
	defaultSettings?: {
		frequencyPenalty?: number
		maxOutputTokens?:  int
		presencePenalty?:  number
		seed?:             int
		stopSequences?: [...string]
		temperature?: number
		topK?:        int
		topP?:        number
	}
}
```

Undeclared keys, including `headers`, `providerOptions`, `tools`, `toolChoice`, `responseFormat` and `reasoning`, are refused twice: by Grafana's validation of the closed member, and by admission's full schema.

### Registry entries

- Each type has its own file in `tenanttype`: `default_instructions.go` and `default_settings.go`. Each holds the schema and example as JSON literals, and an exported accessor, `DefaultInstructions() Type` or `DefaultSettings() Type`, that returns a copy. The implementations bind through these accessors.
- `registry` lists both, and `All()` returns them in name order.
- Schemas are compiled when the package initializes, without panicking. If one fails to compile, the type keeps its `Example` and gets a zero schema. `ValidateDefinitions` then fails startup ("an example without a schema"), and the registry tests fail too.
- The package doc keeps its contract text. The godoc of `DefaultExtensions` and `All` drops "none in this version".

### Implementations and binding

- A new internal package, `ai-gateway/pkg/internal/calldefaults`.
  - It exposes `Definitions() []extension.Definition` and returns `[instructions, settings]`.
  - `gateway.DefaultExtensions()` returns that list.
  - Internal Gateway tests use the package directly, because `pkg/gateway` imports `pkg/internal/gateway`.
- Each definition is `tenanttype.X().Bind(tenanttype.Implementation{FailurePolicy: extension.FailSkip, New: ..., Decode: ...})`:
  - `Keys` stays nil, so both key ownerships apply, whatever the call's source. Transforms are ordered tenant scope first, then by name.
  - No secrets and no collectors.
- Each extension has one instance per process. `New` returns a stateless transformer that implements only `RequestTransformer`.
- `Decode` turns a namespace's stored member, which the host passes in canonical JSON, into typed data once per change. The host puts it in `Call.Config` for each call.
- **`default-instructions`:** `Decode` reads `{messages: [{content}]}` and builds `provider.NewSystemMessage(content)` for each item. The transform applies `middleware.DefaultInstructions` with those messages.
- **`default-settings`:**
  - `Decode` reads a struct with `*float64`, `*int` and `[]string` fields into `middleware.DefaultSettingsOptions`, with only those eight fields set;
  - it clips `StopSequences` with `slices.Clip`, so a later transform's `append` never writes into the backing array every call of the namespace shares;
  - the transform applies `middleware.DefaultSettings`. Headers, provider options, tools, tool choice, response format and reasoning are never set. When the defaults have none, `mergeMaps` returns a copy of the call's map, which is equivalent.
- A call without the expected configuration is a transform error, so the `skip` policy applies.
- `TransformParamsInput.Type` and `Model` stay zero, and neither middleware reads them.

### Parity: `default-settings` against `defaultSettingsMiddleware`

Upstream is `mergeObjects(settings, params)`. A defined call value wins. Nested plain objects merge recursively. Arrays and primitives replace. `undefined` call values are skipped.

| # | Behavior | Upstream `ai@7.0.159` | Gateway `default-settings` | Classification |
|---|---|---|---|---|
| 1 | Unset call field | default applied | default applied (nil pointer or nil slice) | parity |
| 2 | Call value set, including `0` | call wins | call wins | parity |
| 3 | Call value `null` | `null` kept; upstream test passes it `as any` | ProviderWire decodes `null` as unset, so the default applies | parity-preserving Go adaptation: `CallOptions` has no null, and `null` isn't a valid V4 value |
| 4 | `stopSequences: []` from the call | call's empty array wins | the non-nil empty slice wins | parity |
| 5 | Arrays (`stopSequences`) | replaced, never merged | replaced | parity |
| 6 | `topK` type | `number` | integer (`*int` in `CallOptions` and ProviderWire) | parity-preserving Go adaptation, already in the provider contract |
| 7 | Validation of defaults | none in the middleware; upstream checks a call's `maxOutputTokens` (integer ≥ 1) and `seed` (integer) only in `prepareLanguageModelCallOptions`, which defaults bypass | admission: `maxOutputTokens` ≥ 1, integers within ±(2^53−1), at least one field | intentional deviation, Gateway-owned. Recorded in `gateway-tenant-call-defaults` |
| 8 | Allowed fields | also `responseFormat`, `tools`, `toolChoice`, `headers`, `providerOptions`, and at runtime any key | only the eight sampling and limit fields | intentional deviation, Gateway-owned (issue #766). Recorded in `gateway-tenant-call-defaults` |
| 9 | Headers merge | one level, per key; call wins | `mergeMaps`, the same | parity in the library; tenants can't set headers |
| 10 | Provider options merge | recursive deep merge across provider namespaces and nested keys | root `DefaultSettings` merges per provider key only, so the call's whole namespace replaces the default's. The root spec says "no deep merge", and no known gap is recorded | root-library divergence, unreachable from the tenant schema. Unrecorded; tracked in [#791](https://github.com/grafana/ai-sdk/issues/791) |
| 11 | `responseFormat` | objects deep-merged, so a call's `{type: text}` keeps a default `schema` | replaced | same as 10 |
| 12 | Call `tools: []` | call's empty array wins | ProviderWire drops an empty list, so a default would apply | same as 10, at the wire |
| 13 | `reasoning` | not in the pinned settings type | supported by root `DefaultSettings` | Go-only addition in the library, not exposed to tenants |
| 14 | Sharing default values | the same array reference is reused across calls | the same slice is reused, with capacity clipped | parity-preserving Go adaptation |

`default-instructions`:
- Detecting a system message anywhere in the prompt, the ordering, the no-op when nothing is configured, and not mutating the prompt all match upstream: this is the existing port. Per-message provider options and the string form aren't offered, and empty lists and empty content are refused. These are Gateway-owned restrictions, recorded in the new spec.
- Upstream's test "should respect system messages added by an earlier middleware" holds for earlier tenant transforms. Operator transforms run after tenant transforms, so an operator system message added later doesn't suppress the defaults. That's the existing Gateway transform order.

### End-to-end plan

A new `test.describe` in `tests/middleware.spec.ts` with `mode: 'serial'` and an explicit timeout of 240 s. It owns the `default-instructions` and `default-settings` resources in `default` and deletes them, ignoring `404`, before and after the block. The existing tests use `e2e-unregistered` and don't conflict.

| Deferred check | How |
|---|---|
| Save without a folder; folder refused | Admin `POST` of a valid `default-settings` with `metadata.annotations["grafana.app/folder"]`: a non-2xx response and a `404` on `GET`. The same object without the annotation: `201`. The refused status is recorded from CI. |
| Member round-trip through Grafana | `GET` returns `defaultInstructions.messages` with two items (an array of objects), deep-equal to what was sent. The `default-settings` member, with `seed` at `Number.MAX_SAFE_INTEGER`, reads back deep-equal. |
| Grafana's messages don't echo values | Creates refused before admission (the message lacks "admission hook for Middleware"), each with the sentinel `e2e-middleware-sentinel-value` as a value: `defaultSettings.temperature: SENTINEL` (wrong type); `defaultInstructions.messages: [{content: SENTINEL}, {}]` (missing required); `defaultInstructions.messages[0].note: SENTINEL` and `defaultSettings.headers: {x: SENTINEL}` (undeclared keys, which also shows headers refused end to end). The status is `400` or `422`, with no sentinel in the body and nothing stored. |
| Schema-failing write with a location | An admission refusal with `400`: `messages: [{content: SENTINEL}, {content: ""}]` names `spec.defaultInstructions` and `/messages/1/content`. An update with `maxOutputTokens: 0` and `stopSequences: [SENTINEL]` names `/maxOutputTokens`. No sentinel in either body. |
| Valid update and disable | A `PUT` with the read `resourceVersion` changes `temperature` (`200`). A disable-only `PUT` of `default-instructions` with its member kept (`200`). |
| Delete a stored resource | `DELETE default-settings` returns `200`, then `GET` returns `404`. |
| Viewer and Editor on a stored resource | `GET` returns `200`. `PUT` and `DELETE` return `403`, and the resource is unchanged. |
| Runtime effect | Through the `e2e/fake` catalog model (operator-owned). The fake answers a unary call whose last user text is the marker `e2e-echo-request` with the JSON of the request's `system`, `temperature`, `max_tokens` and `stop_sequences`. Other calls keep the fixed text, so concurrent route tests are unaffected. Steps: after both creates, `expect.poll` (2 s interval, 70 s timeout) until the system blocks equal the two messages and the settings equal the configured ones. Then disable instructions and delete settings, and poll again until there's no system block and the settings are absent or back to the provider defaults. |
| Plugin log | `e2e.sh` still fails on a sentinel in plugin lines, now with real reads and admissions behind them. |

The poll bound is one TTL plus a refresh plus margin. Both transitions share one poll, so the block's worst case is about 2.5 minutes. No TTL knob is added.

### End-to-end hygiene

- `scripts/e2e.sh` prints the matched API-access record (`echo "plugin API access: $api_access"`). The record holds only `available=<bool>`.
- `tests/accounts.ts` turns tracing off for every test that uses the `accounts` fixture, by overriding the worker-scoped `trace` option inside the fixture definition. A new spec can't forget it, and other specs keep their traces. This covers both the token-minting responses and the `Authorization` headers. Whether Playwright accepts the override is checked locally with `playwright test --list` and a scratch spec that reads the option, without Grafana.

### Documentation

- **New `docs/guides/gateway-stack-defaults.md`, for org admins:**
  - a complete YAML resource and a `kubectl`/HTTP example for each type;
  - the roles: Admins write, Viewers and Editors read;
  - both types are off until a resource enables them;
  - a client system message suppresses the default instructions, and a call's own settings win;
  - settings combine field by field, so a default `topP` plus a call's `temperature` reaches the model with both, which some models refuse;
  - a default `maxOutputTokens` above a model's limit fails those calls;
  - propagation: about a minute per replica, with read failures falling back to off;
  - disabling and deleting;
  - self-hosted Grafana without API access refuses writes;
  - defaults add tokens to every call.
- `docs/README.md` links the page, and the guide footers chain gateway-extensions → stack defaults → architecture.
- The extension guide's tenant section links the new guide and names the two shipped types.
- The glossary adds "default instructions; default settings; stack defaults". The architecture walkthrough notes that the shipped registry holds two types and that the shipped build reads `Middleware`.

## Test plan

| Acceptance | Evidence |
|---|---|
| End to end: create, call and see it applied, then disable and see it removed | **CI e2e:** the runtime-effect test. **Go:** a gateway test through the test host with the shipped definitions. Namespace `a` enables both, `b` disables instructions and deletes settings after creating them, and `c` stores nothing. Each catalog call's fake-Anthropic body shows the system blocks and `temperature`, `max_tokens` and `stop_sequences` for `a` only. Transitions within one namespace are covered by #765's `synctest` pipeline tests. |
| A client system message suppresses `default-instructions` | **Unit (`calldefaults`):** a system message first, a system message later, none (prepends both, in order), unary and stream; the input prompt isn't mutated. **Gateway:** a call with a system message reaches the fake with only its own system block. |
| The `default-settings` schema refuses headers, provider options, tools, tool choice and response format | **`tenantconfig` admission table with the shipped definitions:** each of those, plus `reasoning`, an unknown key, a wrong type, `maxOutputTokens: 0`, `1.5` for `topK`, `seed: 9007199254740992`, `{}`, and for instructions an empty list, an empty content and an extra key. Each refusal names the location and holds no sentinel. Boundary values are accepted: `seed` at ±(2^53−1), `maxOutputTokens: 1`. **Manifest test:** the projected properties are exactly the eight fields. **CI e2e:** `headers` refused by Grafana. |
| Both apply to operator-owned and customer-owned calls | **Pipeline test (`extensionhost` or `service`)** with the shipped definitions and a fake reader that enables both: an operator-catalog call and a request-BYOK call, unary and stream, each reaching the model with the defaults. **Definition test:** nil `Keys`, a `Decode`, failure policy `skip`, tenant scope. Stack catalogs need #757. |
| Settings precedence (parity rows 1–5, 14) | **Unit tests** ported from upstream's `temperature` and "other settings" cases: a call's `0`, `stopSequences: []`, unset fields, and headers, provider options, tools, tool choice and response format unchanged. A later `append` to the stop sequences doesn't change another call's defaults. |
| Registry and binding | **`tenanttype`:** `All()` returns both types in name order; each example passes its schema, and the members are `defaultInstructions` and `defaultSettings`. **The `manifestdata` member-mirror test** covers both members. **`pkg/gateway`:** `DefaultExtensions()` passes `ValidateDefinitions` against `tenanttype.All()`. |
| Shipped behavior changes | Update `TestMiddlewareAdmission_ShippedTypesRefuseEveryCreate` to the new shipped scenario. `TestTenantConfiguration_ShippedDefinitionsReadNothing` becomes "reads once per namespace, calls unchanged without resources", plus a test with no tenant definitions that makes no reads. The self-hosted test logs the no-access warning with the shipped build. |
| Read-time failure applies `skip` | **Snapshot test:** a stored `default-settings` member that fails the schema (injected through the fake reader) drops the extension, the call runs, and the configure failure is counted. |
| Decode | Safe integers stay exact, and every member admission accepts decodes. A malformed member at read time is a `Decode` error under `skip`. No collectors are registered. |

## Risks / Trade-offs

- **[The shipped build now reads `Middleware` for every active namespace]** This means API load of about replicas × active namespaces per minute, and a first call per namespace and replica that waits for one read (bounded at 5 s). → That's #765's accepted design. Both extensions are `skip`, so failed reads leave calls running without defaults. The manual development check confirms reads succeed in Cloud.
- **[Cloud permission gap]** If the plugin identity lacks `list` on `middlewares`, every read is `refused`, and defaults silently don't apply. → It's counted and logged. The manual check covers it.
- **[Grafana's own messages may echo values]** The e2e test would fail. → That's the point of the check. If it happens, the change records it as platform behavior, documents it in the guide, and adjusts the test to assert the observed behavior after review.
- **[The folder annotation may be accepted by Grafana]** → The e2e test fails, and the spec's "saved without a folder" stays true. The folder refusal expectation would be revisited with the observed status.
- **[Public schemas are permanent]** Types can only grow. → The shape and bounds were reviewed before implementation (Resolved questions).
- **[Provider-level conflicts]** Defaults merge field by field. For example, a default `topP` plus a call's `temperature` reaches the model with both, which some Anthropic models refuse. → Same semantics as upstream. Documented in the guide. Provider-specific validation is a non-goal.
- **[End-to-end duration and flakiness]** The block adds up to about 2.5 minutes, and a retry repeats the whole block. → A bounded poll, serial mode, and cleanup before and after.
- **[Generator tooling]** `generate:kinds` downloads the pinned `grafana-app-sdk` binary unless it's cached. → If the sandbox blocks the download, the exact command is reported for approval.
- **[Self-hosted without API access]** The shipped build now logs the "tenant configuration is not applied" warning and refuses writes. → Already documented by #765. The new guide repeats it.

## Migration Plan

- No data migration. Both types are off by default, so behavior only changes in namespaces that store a resource.
- **Rollback to #765's build:** stored resources become unimplemented types. Enabled ones are skipped and counted. They can be disabled, with the member kept or removed, or deleted. Grafana's manifest then lacks the members, so a disable must remove the member, as the guide already says.

## Recorded decisions

- Member names `defaultInstructions` and `defaultSettings`.
- Field names are upstream's camelCase `CallOptions` names.
- No schema panics. A compile failure surfaces at startup and in tests.
- Implementations live in `pkg/internal/calldefaults`, reuse the root middleware through one adapter, and bind through exported type accessors.
- `default-settings` exposes only the eight issue fields, not `reasoning`.
- The end-to-end runtime check uses an echo marker in the fake provider and a 70 s bounded poll, with no TTL override.

## Resolved questions

1. **`defaultInstructions` shape:** `messages: [{content}]`. It matches `middleware.DefaultInstructions(...provider.Message)` and upstream's array form. The guide shows the one-message case first.
2. **Bounds:** only `maxOutputTokens` ≥ 1, the safe-integer ranges, non-empty messages and content, and at least one setting. No `temperature` or `topP` ranges.
3. **Root-library divergences:** no root-library change in this change. The `upstream-sync` issue is [#791](https://github.com/grafana/ai-sdk/issues/791).
4. **Trace hygiene:** `trace: 'off'` as a worker-scoped override in the `accounts` fixture. Verified locally: with `trace: 'on'` forced in a scratch configuration, a test using the `accounts` test object wrote no trace, and a test using the base fixtures wrote one whose network log contained the scratch bearer token.

## Shipped reads

The shipped tenant extensions apply to every call, so the shipped Gateway reads the `Middleware` resources of every namespace it serves calls for, even one that stores none. On each replica, the first call per namespace waits for that read, bounded at 5 s. API load is about the active namespaces per replica per minute. The cache constants (`ttl` 1 minute, `maxStaleness` 10 minutes, `readTimeout` 5 s, `failureBackoff` 30 s, `maxNamespaces` 1024) are the values to revisit with development metrics. The extension guide and the org admin guide state this, and so does the pull request body.

## Follow-ups

- **Manual check for nara** on a development deployment with this build:
  1. Apply `Middleware` resources in two stacks with different configurations, such as different instructions and temperatures.
  2. Call each stack's language-model route, and confirm each call uses its own stack's configuration.
  3. Confirm the plugin logs show no `refused` read failures, and `grafana_ai_gateway_tenant_config_reads_total{result="success"}` grows.
  4. Confirm list responses carry `kind: MiddlewareList`. Otherwise reads fail with class `invalid-response`.

  The same steps go into the PR body.
- **List kind label.** Against the pinned Grafana nightly, listing `middlewares` returns `kind: ModelCatalogList`, the list kind of the manifest's first kind, with media type `application/json`. The reader therefore no longer requires `kind: MiddlewareList`: the request path identifies the resource, a JSON object with an `items` array is a list unless its `kind` names a non-list such as `Status`, and each item needs a name and, when present, the Middleware group and version. A platform bug report is drafted for the maintainers.
- Stack-catalog coverage when #757 lands. No code change is expected, because `Keys` is nil.
- Root `DefaultSettings` alignment: [#791](https://github.com/grafana/ai-sdk/issues/791).

### `upstream-sync` issue, filed as [#791](https://github.com/grafana/ai-sdk/issues/791)

**Title:** `fix(middleware): align DefaultSettings merging with upstream defaultSettingsMiddleware`

**Body:**

> Registered baseline: `ai@7.0.159` (commit `028a025398782f4938303f48fbeee45c79af544b`). Upstream `defaultSettingsMiddleware` returns `mergeObjects(settings, params)` (`packages/ai/src/util/merge-objects.ts`). In that merge, a defined call value wins, nested plain objects merge recursively, and arrays and primitives are replaced. The Go `middleware.DefaultSettings` differs as follows:
>
> 1. **Provider options.** Go merges per provider key, so a call's provider namespace replaces the default's whole namespace. Upstream merges nested keys too. For example, a default `{anthropic: {a: 1}}` plus a call's `{anthropic: {b: 2}}` gives `{anthropic: {a: 1, b: 2}}` upstream and `{anthropic: {b: 2}}` in Go. See the upstream tests "should merge complex provider metadata objects" and "should handle nested provider metadata objects correctly".
> 2. **Response format.** Upstream deep-merges the default and call objects, so a call's `{type: "text"}` keeps a default `schema`. Go replaces the object. Decide whether to match this or record a deviation.
> 3. **Empty tools.** The Gateway's ProviderWire decoding maps a call's `tools: []` to nil. Go then applies default tools, where upstream keeps the call's empty list.
> 4. **Reasoning.** Go's `DefaultSettingsOptions` supports `Reasoning`. The pinned upstream settings type doesn't list it, although `mergeObjects` would merge it at runtime. Record it as a Go addition, or remove it.
> 5. **Spec drift.** `openspec/specs/builtin-middleware-default-settings` says zero-length slices are filled, but the code fills only nil slices. The code matches upstream for `stopSequences: []`. The spec's provider-options scenario says "no deep merge".
>
> Acceptance: each item is fixed or recorded in `knownGaps`; upstream's provider-options and response-format cases are ported as tests; and the spec is updated. The Gateway's tenant `default-settings` type exposes none of these fields, so it isn't affected.
