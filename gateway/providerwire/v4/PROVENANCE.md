# ProviderWire V4 contract provenance

The executable authority is the exact package set registered in
[`test/conformance/upstream.yaml`](../../../test/conformance/upstream.yaml):

| Package | Version |
| --- | --- |
| `@ai-sdk/provider` | `4.0.4` |
| `@ai-sdk/gateway` | `4.0.33` |
| `@ai-sdk/provider-utils` | `5.0.16` |
| `ai` | `7.0.44` |

Request captures and response-consumption tests execute these npm versions. They
prove stock-client emission or consumption, not acceptance by Vercel's private
server.

## Source authority

The registered source commit is
`c527d7b3b26287598d2c80e7bce8f16b21653363`. Its workspace manifests contain
provider `4.0.4`, Gateway `4.0.30`, provider-utils `5.0.14`, and ai `7.0.40`.
Consequently, the commit alone is not executable evidence for the registered
package set.

The provider V4 source trees at the commit are byte-identical to the
`@ai-sdk/provider@4.0.4` tag:

| Source tree | Git tree object |
| --- | --- |
| `packages/provider/src/language-model/v4` | `13363d184fc4cbbb5fc8908923e72d5d1d937b6e` |
| `packages/provider/src/shared/v4` | `6aeaf1740ec862f09e3336b49a52be4251a14e2a` |

The HTTP/error paths relied on from Gateway and provider-utils are
byte-identical between the registered commit and the corresponding release tag:

| Release tag | Source path | Git blob object |
| --- | --- | --- |
| `@ai-sdk/gateway@4.0.33` | `packages/gateway/src/gateway-language-model.ts` | `0848a5fa9b16c1cd750d5299f1440722e1405d3e` |
| `@ai-sdk/gateway@4.0.33` | `packages/gateway/src/gateway-provider.ts` | `2630ab9954e8a52fdf700422d6420aeafe90ce6a` |
| `@ai-sdk/gateway@4.0.33` | `packages/gateway/src/gateway-config.ts` | `cb375d24c4c2d8966787948ec323d07fc6ce8e9c` |
| `@ai-sdk/gateway@4.0.33` | `packages/gateway/src/errors/create-gateway-error.ts` | `d90f86688f62df6f2889c9c690ec08cdfe310202` |
| `@ai-sdk/gateway@4.0.33` | `packages/gateway/src/errors/as-gateway-error.ts` | `8d7bd4d32acc074a8ea03a0866f6aaf8f205d499` |
| `@ai-sdk/gateway@4.0.33` | `packages/gateway/src/errors/parse-auth-method.ts` | `39ed93f4625570e756a8a2ea4241f7365c1fa5cc` |
| `@ai-sdk/provider-utils@5.0.16` | `packages/provider-utils/src/post-to-api.ts` | `520a7b0589dc9de0e940e91ef3f53ff01cc882d6` |
| `@ai-sdk/provider-utils@5.0.16` | `packages/provider-utils/src/response-handler.ts` | `69836ee1fac46cae1fd0e03fa4e1c7a3d33d3710` |
| `@ai-sdk/provider-utils@5.0.16` | `packages/provider-utils/src/parse-json-event-stream.ts` | `4769ebcf28c4b20ee46223137599ead2c6822db7` |
| `@ai-sdk/provider-utils@5.0.16` | `packages/provider-utils/src/combine-headers.ts` | `5f842268d4044cea1269fbc637cb818c14da5d33` |

The registered ai package differs from the commit and is evaluated only through
`ai@7.0.44` execution.

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

A baseline change updates the manifest, exact TypeScript pins, this equivalence
record, captures, schemas, fixture index, boundary ledger, parity map, and
lockfiles together. A missing or changed equivalence proof is a stop condition;
it must not be replaced silently with source from `main`.
