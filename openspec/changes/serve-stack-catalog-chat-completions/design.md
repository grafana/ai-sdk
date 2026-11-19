## Context

The namespace Chat and catalog ProviderWire handlers share the runtime
selector. The latter gets the `ModelCatalog` and decrypted keys from the
platform's route request. Selection builds only that catalog's candidates and
marks the source `stack-catalog` with customer-owned keys. The extension host
wraps the logical selection; each physical candidate has its own translator
wrapper. The catalog route needs neither a second Chat handler nor a new
selection path.

## Decisions

### One route, one declared check

Append one route-table entry: POST at the catalog prefix plus
`chatcompletions.Path`, label `catalog_chat_completions`, using
`chatCompletionsPath(chat)`, the Chat host-error writer and `catalog: true`.
Reuse `parentCatalog` and `DecideCatalog`; both precede body reads. The
shared selector receives the parent spec and keys for this request. Aliases
and configured fallback resolve only in that catalog, even if the operator
catalog has the same alias. The namespace path and access remain unchanged.

Declare the OpenAPI operation on the full path in `kinds/manifest.cue`.
The route path below the name is `v1/chat/completions`, but the **declared
authorization subresource** is the single segment `chat-completions`:
`get` on `modelcatalogs/chat-completions` of the named catalog. Grant
`ModelCatalog/chat-completions:get` in `aigateway:modelinvoker` only. This
matches the language-model route's verb and roles without granting invocation
to catalog readers or making one protocol's permission imply the other's.
App SDK routes separate the kind route tail from the single-segment declared
authorization subresource. Verify both in the generated manifest and mounted
Grafana route.

| Condition | Outcome |
| --- | --- |
| Platform denies declared authorization or cannot find the catalog | Platform 403 or 404; its document need not be a Chat envelope. |
| Plugin receives missing or inconsistent route-parent identity | Fixed Chat internal error (500), before reading the body. |
| Matched parent, but Gateway refuses account access | Fixed Chat permission error (403), before reading the body. |
| Invalid stored spec, missing decrypted key or endpoint disallowed by catalog policy | Shared catalog-configuration category (424) in a fixed Chat envelope; no provider I/O or catalog details. |
| Unknown model in an otherwise valid catalog | Fixed Chat model-not-found error (404); no provider I/O. |
| Unlisted route or undeclared method | Not dispatched as Chat; no access decision or native work in the plugin. |

Prove endpoint refusal on main with a forbidden catalog endpoint override.
Do not add an allowlist lookup, depend on one, or test a permitted custom
endpoint. Reusing the selector ensures Chat follows the same policy and
classification if that policy later changes. Keep edits in shared route,
manifest and role files to the new entry, operation and grant; regenerate
the existing artifacts. No dependency on the catalog UI, health,
available-models or custom-endpoints changes.

### Defaults live at different layers

1. Validate and map the existing Chat request first. Preserve the caller's
   system instructions and explicit accepted values (including zero).
   Omitted or accepted null settings are unset. Both supported token-limit
   spellings map to the same setting; supplying both non-null is still
   invalid. Omitted/null `stop` can be defaulted; a valid explicit stop wins,
   and `stop: []` remains invalid at the Chat boundary.
2. The runtime's extension host runs the namespace's enabled tenant request
   transforms once per logical call. The shipped defaults fill only absent
   generic fields and prepend instructions only without a caller system
   message. They do not set native provider options or tool/schema
   strictness. This is precedence for **defaults**, not a change to operator
   transform or policy ordering.
3. On every attempted candidate, translate a copy of the resulting options
   to the native protocol. Keep caller-supplied strictness and explicit
   parallel-tool behavior; omitted/null function strictness is false.
   Keep Chat's Responses/compatible `store: false` policy (true is not an
   accepted caller setting). The translator must preserve resolved prompt
   and generic settings. Host policy and native restrictions still apply;
   filling a value does not make it supported by every model.

The current composition already puts these layers in order. Pin it with
tests, not new defaulting code. Exercise omitted and caller-set settings and
instructions on both routes and verify a heterogeneous fallback case: two
native requests with the same resolved generic values and independently
translated storage/strictness/parallel behavior. ProviderWire receives no
Chat translator.

### Make extension evidence distinguish both boundaries

Drive the actual route table, selector and extension host. A test operator
extension should run for matching keys on both routes (operator-owned
namespace, customer-owned catalog); a test customer-key tenant extension
should run on the catalog with stored namespace configuration and not on the
operator-key namespace call. Both shipped tenant defaults, which support both
ownerships, run on both. There is no successful customer-key namespace Chat
path because Chat has no BYOK fields; do not add one to make the test pass.

Use real extension phases, not preclassified stub errors. Request denial and
fail-closed unavailability yield 403/503 on unary and stream setup with zero
provider I/O. A response inspector that stops an established stream after a
visible text delta yields one SSE Chat error (`permission_denied` or
`unavailable`), not a new HTTP status. The triggering delta is withheld, no
fallback/replay occurs, and there is no successful finish, usage or
`[DONE]`. Use sentinels to prove error text and private names aren't
serialized. Keep existing classification, framing and cancellation ownership.

### Parity boundary

Checked [the registered baseline](../../../test/conformance/upstream.yaml)
and [PARITY.md](../../../test/conformance/PARITY.md): Vercel AI commit
`0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1` (ai 7.0.173,
`@ai-sdk/openai` 4.0.118, `@ai-sdk/openai-compatible` 3.0.92,
`@ai-sdk/gateway` 4.0.146). The pinned OpenAI and compatible Chat
implementations and tests use `/chat/completions`, standard request field
names, streamed choices/deltas/finish and optional usage, and handle error
frames. The pinned Gateway client calls `/language-model`; the catalog Chat
prefix is a Grafana extension, not an upstream route. The pinned Chat client
schemas do not define a Grafana execution overview. Upstream passes named
`store`/strict controls through as applicable; the existing Gateway subset's
storage refusal and strictness defaults remain intentional adapter policy,
not claims about upstream provider defaults.

Keep Chat's existing schema and wire representation on both mounts. Native
payload assertions and the official SDK harness are the evidence for adapter
behavior; no UI stream, provider conformance fixture, baseline, or parity map
change is needed for this route. Synthetic providers are not live recordings.

### Evidence and rollout

- Extend the compiled-plugin Go OpenAI SDK harness with a manifest-backed
  catalog created via the test platform, using the existing test-native TLS
  proxy approach to reach a synthetic native provider without an endpoint
  override. Use service-account credentials, both Gateway access modes,
  the catalog base URL ending in `/v1/`, and disabled SDK retries. Assert
  unary and streaming results, selected alias/model and the stack key at the
  native hop. Use a distinct operator key/model for the same alias and check
  that namespace selection stays unchanged, including cloud-mode HTTP 400.
  The test host verifies the protocol hop, not Grafana's declared RBAC.
- Extend the pinned-Grafana `ai-gateway/tests/catalog-routes.spec.ts` using
  the existing role accounts, saved catalog and proxy/digest fixture. Verify
  Editor/Admin invocation, Viewer 403 and missing-catalog 404 from Grafana,
  and unary and SSE success with the decrypted fixture key. This also
  verifies the multi-segment route on real Grafana. Use CI, not local Docker.
- Cloud mode in the protocol harness is not deployed Grafana Cloud. Cloud
  still lacks secure values for plugin kinds; do not promise a working Cloud
  deployment until the platform supports them. No platform code or rollout
  change is required. Rollback removes the new route and grant only.

## Open Questions

| Question | Recommendation |
| --- | --- |
| Add a caller-visible execution overview to Chat now? | No. Leave the existing Chat document and diagnostics unchanged. ProviderWire keeps its overview. A future per-call extension should have its own schema, projection/redaction, unary/SSE placement (including errors), and SDK-consumption review. Aggregated customer telemetry is separate. |
| Which declared permission should the nested route use? | `get` on the single-segment `chat-completions` subresource in the existing model invoker role, not parent-only `get`, the slash-separated route tail, or a new role. |
| Expand supported Chat fields or clear defaults via empty `stop`? | No. Keep the existing subset and presence semantics; make any new wire behavior a separate compatibility change. |
| How much deployed and SDK evidence should gate the change? | One official Go SDK matrix through the plugin for both modes, plus focused Grafana Playwright API coverage in the existing suite. Do not add a second SDK matrix or browser UI work. Mark deployed Cloud verification blocked on platform support. |
| Which catalog page should document the URL? | Use [Call a stored catalog](../../../ai-gateway/README.md#call-a-stored-catalog) now and link it from the Chat guide. Do not depend on the in-app catalog page; showing the URL there can follow once that page lands. |
