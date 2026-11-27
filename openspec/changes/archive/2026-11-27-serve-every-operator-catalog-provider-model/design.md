## Context

`config/file.go` decodes a single bounded YAML document with known fields and validates it before endpoint and secret resolution. The configured branch of `service/selection.go` uses the prebuilt catalog for calls without routing controls; `service/routed.go` resolves the request and its `gateway.models` through the listed routes. Stack catalogs already implement exact serving rules, first-slash resolution, lazy candidate construction after planning, and bounded discovery with a separate admission budget and 30-second inventory cache.

The pinned `@ai-sdk/gateway@4.0.152` [language model](https://github.com/vercel/ai/blob/%40ai-sdk%2Fgateway%404.0.152/packages/gateway/src/gateway-language-model.ts) passes the model ID as a string header, and [discovery](https://github.com/vercel/ai/blob/%40ai-sdk%2Fgateway%404.0.152/packages/gateway/src/gateway-fetch-metadata.ts) decodes string IDs on the existing `/config` route. This is Gateway catalog policy, not a change to either contract.

## Design

- Use the same public-ID validator for listed IDs, aliases, blocklist entries, and both the full selector and backend ID in unlisted lookup and discovery. Reuse exact blocklist matching. Keep normal YAML validation and listed-only behavior when no provider opts in. Apply the conflict check to each primary and Gateway fallback and its explicit Anthropic server fallback IDs, scoped to the candidate's own provider instance and regardless of the switch. Fail startup with field paths, not blocklist values or credentials.
- Keep listed routes prebuilt. After a listed lookup misses, the fast path constructs one candidate from the qualified selector; the routed path resolves the same descriptor before planning and constructs it only if the existing planner retains it. Use the existing configured identity, model client and startup-resolved provider settings. A request cannot supply credentials or change the destination. The unlisted descriptor has no policy or fallback fields, even for a backend with a listed route. Explicit `gateway.models` entries use the same lookup and routing rules; invocation never checks inventory. Use the fixed Prometheus model label `unlisted` for unlisted operator calls; keep the qualified canonical ID for execution, Agent Observability and structured logs, subject to the existing log limits.

## Decisions

### A. Anthropic `allowDefault` with a nonempty blocklist

- **Refuse at startup (approved).** Reject a primary or Gateway fallback on that provider with `serverFallbacks.allowDefault: true`, even if `serveAllModels` is off. Anthropic chooses the backend for `"default"`; the Gateway cannot check it before dispatch. Explicit, nonblocked fallback IDs remain available.
- **Allow with a documented exception.** Keeps vendor-chosen fallback available but permits a blocked ID to serve a call. That weakens the meaning of a blocklist.

### B. Operator discovery

- **Reuse stack discovery (approved).** After configured-account authorization, append eligible inventory IDs alongside the listed rows. Reuse the bounded listing worker, projection rules and 30-second inventory cache; scope the operator cache to the immutable startup catalog and key it by provider instance. Use the existing bounded probe transport and the startup-validated destination, not stack/BYOK endpoint policy. On both paths, read cached inventories before admission and only try to acquire a slot for cache misses. Admit one operator discovery run at a time, separately from stack discovery, health and inventory actions. If admission is busy, return listed and eligible cached rows; if a probe fails or exhausts a budget, keep eligible results collected so far. This makes an empty listed catalog useful to existing discovery clients without making listing a prerequisite to invoking a new model.
- **Invocation only.** Keep `/config` listed-only and make no new upstream requests. This is smaller, but applications must learn unlisted IDs elsewhere, and an empty listed catalog always looks empty to discovery clients.
