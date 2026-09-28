## Context

`prepareTools` converts declarations but `applyToolChoice` currently maps every selected allowed name to `{type:"function",name:...}`. The registered reference is `@ai-sdk/openai@4.0.72`, commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`, `packages/openai/src/responses/openai-responses-prepare-tools.ts` and its allowedTools tests. It records direct declarations and provider-name aliases as tools are prepared, then resolves the restriction before ordinary choice. Go's request converter already propagates preparation warnings/errors to unary and streaming calls.

## Goals / Non-Goals

**Goals:** Match the registered Responses allowed-tools declaration resolution and warning/error semantics; preserve input order, duplicates, ordinary choice precedence, and caller-owned options. Assert serialized requests and failures without inventing provider recordings.

**Non-Goals:** Change ordinary forced shell/local-shell/tool-search choices (#32); validate mode values at runtime in tool preparation; change frontend chunks, live API behavior, or other providers.

## Decisions

1. Build direct-name and canonical-alias resolution indexes while preparing actual emitted declarations, not by mapping arbitrary strings against `providerToolNames`. Record supported entry or unsupported reason per declaration. Function: `{type:"function",name}`; custom: `{type:"custom",name}`; MCP: `{type:"mcp",server_label}`; hosted types: type only (`file_search`, `web_search`, `web_search_preview`, `image_generation`, `code_interpreter`, `computer`, `apply_patch`, `shell`, `local_shell`, `programmatic_tool_calling`). Namespace-contained and deferred functions, plus `tool_search`, cannot be selected; unknown/omitted provider tools must not be misclassified as supported. Alternative, inferring type from the selected name or mapping table, fails on aliases, unsupported declarations and custom/MCP payloads.
2. Maintain direct-name priority even if that name is an alias for another declaration (warn about shadowing), and track ambiguous canonical aliases when declarations with different resolutions share one alias (warn and drop); direct MCP names select their own server labels. Follow the reference's equality semantics when comparing resolutions so identical aliases do not become falsely ambiguous. Do not mutate `toolNameMapping`, tool lists, or option slices to resolve choices. Alternative, deduplicating names or failing on collisions, changes upstream ordering and recoverable behavior.
3. If declarations are absent, return without a choice. Otherwise, if `AllowedTools` is non-nil, iterate every requested name in order even if its list is empty: warn/drop ambiguous and unsupported known selections; warn but retain unknown names as mapped function entries. Fail before transport only when the resulting list is empty, listing dropped names in source order; else set `allowed_tools` and default mode `auto` when absent. The supported mode domain is the upstream typed `auto`/`required`; do not introduce blanket invalid-mode rejection in `prepareTools` or enlarge this change to options parsing. Alternative, rejecting all unknown or empty cases contradicts pinned 4.0.72; letting empty lists fall through to ordinary choice contradicts it too.
4. Keep declaration conversion/warning accumulation and normal `toolChoice` branch independent. Resolve the allowed-tools override only after successful tool conversion and return its warnings and errors through the existing `prepareTools`/request-building path. Test request JSON, warnings, no-transport errors, and input immutability for unary and streaming request construction as applicable. Provider `recorded/` inputs require real API capture; `upstream/` requires exact registered fixtures plus INDEX provenance. Do not synthesize either; synthetic local-HTTP tests establish encoding, not live API acceptance.

## Risks / Trade-offs

- [Unknown allowed name still reaches OpenAI] → This is required by upstream, with an explicit unsupported warning; tests assert warning and function entry, not acceptance by the API.
- [Go `AllowedToolsOption.Mode` is currently a string while upstream's source is a TS union] → State the allowed domain and default without making mode validation a new requirement; no new Go API change in this fix.
- [No provenance-valid allowed-tools recording identified] → Record a provider-boundary coverage gap in review results; update `PARITY.md` or baseline metadata only if a stable coverage status or support boundary changes.

## Migration Plan

No migration: keep existing option shape and request building API; deploy by replacing only allowed-tools resolution and adding focused regression tests. Revert the provider change if request compatibility regresses.

## Open Questions

None for the approved 4.0.72 alignment. Live OpenAI API acceptance remains outside synthetic request-test evidence.
