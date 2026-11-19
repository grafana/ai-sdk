# Design

## Context

`stackcatalog.Parse` validates the stored kind's spec and keys, and projects it into the shared catalog shape. Admission and the four catalog routes use the same checks. Selection indexes listed IDs and aliases, and discovery projects only configured rows. The saved provider card already supports an explicit, permission-gated inventory lookup for **Add model**.

## Goals / Non-Goals

- Keep the policy local to a saved provider instance, its key and its existing destination. Apply changes on the next request.
- Do not expand namespace routes, the operator catalog, BYOK, or what the current provider adapters can do. Do not verify model existence or capability before inference.

## Decisions

### Selector: require the saved instance name and a model ID

Use `<provider-instance>/<model-id>` for unlisted models. Split at the first `/` like BYOK, but use the saved instance name (e.g. `team-openai`), not the provider type (`openai`). Keep the rest as the literal backend ID, including further slashes. Do not guess even if just one instance has opted in: that guess would become ambiguous when another account enables the setting. Try the exact configured ID or alias first, including ones that look like qualified selectors; do not reserve a new prefix in the existing ID space.

After shared preflight, a qualified lookup must name an enabled instance and a valid, unblocked ID. Otherwise return the existing unknown-model error without constructing any candidate. A match produces one candidate with the same attribution and credential path as a listed candidate. Use the full selector as the logical canonical ID and the suffix as the backend ID.

### Discovery: list configured models only

Keep `/config` local and deterministic, even if a provider serves unlisted IDs and the catalog advertises none. Calling `available-models` from discovery would run provider traffic on the user's stored key, add latency and partial results, and make account inventory visible to catalog invokers. Do not call it from inference, saves, or rendering the new settings either. Reuse the on-demand model ID picker for writers with listing permission. It adds listed models or edits blocked IDs, with only one picker shown at a time. Suggestions reflect account inventory, not what the Gateway serves. Writers may still enter an exact ID by hand.

### Blocklist: exact IDs scoped to the instance

Use optional `serveAllModels` (boolean) and `modelBlocklist` (list of strings) in the existing `ModelCatalog` provider object. Store a blocklist even when the switch is off so a writer can prepare it and re-enable serving without losing it. Limit it to 128 distinct IDs per instance. IDs are literal, case-sensitive backend IDs, not selectors, aliases, globs or prefixes. Use the same ID check for each entry and each unlisted selector suffix: valid UTF-8, 1–2,048 bytes, no control characters and no leading or trailing whitespace. Do not rewrite any existing listed IDs or target rules. Provider families and vendor aliases are not a stable basis for pattern matching.

Extend `stackcatalog.Parse` to validate the stack-only policy and reject an exact `(instance, model ID)` match against any listed primary or fallback, even when the switch is off. Report the offending field without echoing the entry. Keep that policy alongside the shared shape, not in the operator config type or YAML schema. The checked policy used by runtime selection must be the one admission validates; an invalid stored spec still fails preflight on every catalog route.

### Fallbacks: no implicit chain for unlisted IDs

An unlisted selector supplies one candidate, with no inherited or inferred fallbacks, even if a differently named listed model targets the same backend. Use the same resolver for the requested ID and explicit `gateway.models` entries, then the existing `order`, `only`, deduplication and attempt bound. A blocked fallback entry fails as unknown instead of being skipped, before any provider work. Keep listed chains as configured.

### Per-candidate policies (#781): deny by default for unlisted IDs

Do not define provider-wide policy defaults or copy policy from a listed candidate for the same backend. An unlisted Anthropic candidate has no opt-in to container skills or server-side fallback. Writers who need per-candidate grants would list a model and configure it explicitly once those fields are supported. #781 remains separate.

### Provider settings: save each deliberate action

On a saved provider card, toggling the switch or adding or removing an ID writes once through the versioned catalog update. Writing a switch change preserves saved blocked IDs. A writer who needs exclusions before enabling can edit the blocklist while the switch is off. This keeps one-click enable for empty blocklists without requiring other writers to expose models they intend to block. Reflect only successful writes, omit secure values, and retain an ID if its add is refused. Readers see saved settings without editing controls.

## Risks / Trade-offs

- **Opt-in can expose new provider models and spend.** Explain that the provider still decides access, capability, and quota, and that unknown provider IDs can fail at the provider. Leave the setting off for a strict allowlist.
- **A blocked ID is not a whole model family.** An upstream alias or another provider account is a different ID or destination. An explicit catalog ID that looks like a selector still wins, but cannot route to a blocked target. Removing a listed row does not necessarily stop access to its backend ID; block it separately if needed.
- **Discovery won't guide callers to an unlisted model.** Show an example using the actual provider instance name, and document that callers must know and request the qualified ID.
- **Not parity-sensitive.** At the pinned Vercel `@ai-sdk/gateway` 4.0.146 baseline (`0e8e0543`), model creation uses the supplied ID on `/language-model` and metadata is a separate call. Grafana's saved catalog policy changes which selectors this server can route, not the clients, ProviderWire, or provider conformance.

## Migration Plan

Add optional fields to `kinds/modelcatalog.cue` and regenerate the Go and TypeScript types and both manifests with `mise run generate-ai-gateway-kinds`. Do not change served versions, routes, roles, or existing fields. No existing catalog is migrated or opted in. To stop serving unlisted IDs, switch the setting off; the saved blocklist can remain.
