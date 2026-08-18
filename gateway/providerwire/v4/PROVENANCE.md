# ProviderWire V4 contract provenance

The executable authority is the exact package set registered in
[`test/conformance/upstream.yaml`](../../../test/conformance/upstream.yaml):

| Package | Version |
| --- | --- |
| `@ai-sdk/provider` | `4.0.7` |
| `@ai-sdk/gateway` | `4.0.52` |
| `@ai-sdk/provider-utils` | `5.0.27` |
| `ai` | `7.0.65` |

Request captures and response-consumption tests execute these npm versions. They
prove stock-client emission or consumption, not acceptance by Vercel's private
server.

## Source authority

The registered source commit is
`d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. Its workspace manifests match the
registered package set, so no release-equivalence substitution is required.
The relied-on protocol paths are pinned by their Git objects:

| Package | Source path | Git object |
| --- | --- | --- |
| `@ai-sdk/provider@4.0.7` | `packages/provider/src/language-model/v4` | `b1750eeeda29c46461e6d758390eb4c86b5661e4` |
| `@ai-sdk/provider@4.0.7` | `packages/provider/src/shared/v4` | `6aeaf1740ec862f09e3336b49a52be4251a14e2a` |
| `@ai-sdk/gateway@4.0.52` | `packages/gateway/src/gateway-language-model.ts` | `19d3182e3070ba806cd4acade0c2da0788fb1a6a` |
| `@ai-sdk/gateway@4.0.52` | `packages/gateway/src/gateway-provider.ts` | `1b30f58ffd729b7991ffec9d408970f101560c61` |
| `@ai-sdk/gateway@4.0.52` | `packages/gateway/src/gateway-config.ts` | `cb375d24c4c2d8966787948ec323d07fc6ce8e9c` |
| `@ai-sdk/gateway@4.0.52` | `packages/gateway/src/errors/create-gateway-error.ts` | `f7a5766353d0a8f76189e195ecd440e88d2b1906` |
| `@ai-sdk/gateway@4.0.52` | `packages/gateway/src/errors/as-gateway-error.ts` | `8d7bd4d32acc074a8ea03a0866f6aaf8f205d499` |
| `@ai-sdk/gateway@4.0.52` | `packages/gateway/src/errors/parse-auth-method.ts` | `6f04551ae0cad772ea6896386be451205f3e5edd` |
| `@ai-sdk/provider-utils@5.0.27` | `packages/provider-utils/src/post-to-api.ts` | `b7747a1b0a95bb03b321d0ffdcdd7847b16e4cc3` |
| `@ai-sdk/provider-utils@5.0.27` | `packages/provider-utils/src/response-handler.ts` | `69836ee1fac46cae1fd0e03fa4e1c7a3d33d3710` |
| `@ai-sdk/provider-utils@5.0.27` | `packages/provider-utils/src/parse-json-event-stream.ts` | `4769ebcf28c4b20ee46223137599ead2c6822db7` |
| `@ai-sdk/provider-utils@5.0.27` | `packages/provider-utils/src/combine-headers.ts` | `5f842268d4044cea1269fbc637cb818c14da5d33` |

Source comparison from the previous registered baseline found no change to the
existing LanguageModelV4 serialized type shapes, request preprocessing, routing
headers, response selection, SSE parsing, or header combination used by this
contract. Provider 4.0.7 adds a batch-language-model surface outside this
language-model HTTP capability. Provider-utils widens POST body input to `Blob`
without changing JSON requests. Gateway 4.0.52 changes failed-response message
extraction to `getErrorMessage`; response-consumption projections execute that
behavior directly.

## Divergence classification

| Observation | Classification | Treatment |
| --- | --- | --- |
| Exact stock-client request emission and permissive response consumption | pinned-client behavior | Captured and tested with the exact npm package set. |
| Closed response schemas beyond Gateway's `z.any()` validation | local serialized projection | Proven independently by the curated corpus; no private-server claim. |
| Safe errors add `statusCode` and `isRetryable`, omit `code`, and restrict `param` | local serialized projection | The pinned client accepts the shape but derives its own retryability from HTTP status. |
| Provider references reject a member named `type` | intentional host-policy restriction | Prevents collision with the selected file-data discriminator; not claimed as upstream runtime validation. |
| Existing Go structs lose optional presence, permit flat inactive siblings, widen metadata values, and add response provider identity | coverage gap for the future strict adapter | H1 does not adapt or marshal those structs; the boundary ledger requires a later presence-aware adaptation. |
| Existing `gateway/providerwire` remains tolerant and deployed | parity-preserving coexistence | Legacy bytes, API, Grafana defaults, and frontend behavior remain unchanged. |
| No V4 handler, decoder, Go client, or provider invocation exists | documented H1 coverage gap and non-goal | Later phases must implement and test runtime behavior against this contract. |

No implementation defect was accepted or hidden by opening a standard schema
object.

## Upgrade rule

A baseline change updates the manifest, exact TypeScript pins, this source
record, captures, schemas, fixture index, boundary ledger, parity map, and
lockfiles together. If a future registered commit's workspace versions differ
from the package manifest, path-level release-equivalence evidence is required;
it must not be replaced silently with source from `main`.
