## Context

Registered upstream commit 08ae5ad05bc12496dd1ffcf64e34419e0831300d defines URL/document sources; Gateway 4.0.87 uses permissive consumption. Current published root already exposes Title, while native producers retain legacy Text. Both must work.

## Decisions

Use nonempty Title then legacy Text, without a breaking provider migration. Closed citation metadata preserves only numeric position fields; omit cited text because it adds an unnecessary content disclosure. Bounds, native origins and omission behavior are normative in gateway-sources. Recognized OpenAI file_path gets a neutral title and omitted filename. IDs are response-local sequential values keyed by a struct, never hashes of native IDs. Source mapping occurs inside existing bounded runtime lifecycle; no new protocol framework.

## Risks / Trade-offs

Gateway metadata projection and source-ID restrictions deliberately differ from native provider parity. Unknown provider namespaces lose source metadata. Display content other than recognized file_path is not sanitized. Sources produced by hosted tools may accompany other currently unsupported families. Source timing in updated Apache middleware needs immutable published pins before the standalone Gateway adopts it; existing pins still establish safe pass-through/privacy. Review and publish prerequisites separately without committing replacements.

## Validation

Focused failing-before/fixed-after Gateway tests, standalone readonly modules, native synthetic command scenarios, independent-client differential tests, schema-parsed frontend assembly and provider-independent UI conformance. No synthetic provider recordings.
