# Tasks

## 1. Kind and admission

- [x] 1.1 Add the two optional provider fields to `kinds/modelcatalog.cue`. Run `mise run generate-ai-gateway-kinds` and verify the Go and TypeScript types and both manifests expose the fields without changing existing fields, kind membership, routes, or roles.
- [x] 1.2 Validate the stack-only policy in `stackcatalog.Parse`, including bounds, duplicates, exact instance/ID conflicts against all primaries and fallbacks, and fixed field errors. Add parser and CREATE/UPDATE validator tests for omitted fields, off with a saved blocklist, same-type account isolation, errors without secrets, and unchanged operator config validation.

## 2. Catalog routes

- [x] 2.1 Add the qualified lookup after listed IDs and aliases and reuse existing candidate construction and routing. Cover same-type instances, suffixes with slashes, disabled, malformed and blocked selectors, listed precedence, saved keys and attribution, and no construction or provider calls on refusals in Go tests.
- [x] 2.2 Verify in Go tests that unlisted candidates neither inherit fallbacks nor opt into Anthropic skills or server-side fallback. Exercise explicit `gateway.models` with an eligible and a blocked selector and preserve `order`, `only`, deduplication and attempt bounds.
- [x] 2.3 Add route tests for unary and streaming selection, unknown-model errors, listed-only `/config` without provider traffic and qualified discovery for opted-in providers, and invalid stored settings failing closed on all catalog routes. Keep on-demand health and inventory unchanged.
- [x] 2.4 Document the kind settings and an example qualified call in the README's **Store model catalogs in Grafana** section. Verify the example against the route contract and explain exact matching, listed precedence, best-effort discovery, fallback behavior, and that removing a listed entry need not block its backend ID. Update the Gateway architecture walkthrough and glossary where they describe catalog selection and discovery.

## 3. Catalog page

- [x] 3.1 Add the switch and blocklist to the saved provider card. Use one versioned update per toggle, add, or remove action, allow only removal of blocked IDs while off, keep keys untouched, and distinguish no listed models from no callable models. Ensure removal copy does not promise that a listed model's backend becomes inaccessible. Cover the update body, retained blocklist, errors, no automatic health or listing, and update permission checks with Jest.
- [x] 3.2 Extend Playwright against the pinned Grafana image and fake provider. Enable a provider with no listed models, check discovery and unlisted invocation, and add a blocked ID. Disable the setting and verify unlisted calls now fail and saved blocked IDs can be removed without enabling. Exercise admission's primary/fallback conflict and verify read-only presentation.

- [x] 3.3 Set and disclose the UI-only default in the shared provider form. Verify create and connect payloads, existing providers, and immediate invocation without listed models.

## 4. Discovery inventory

- [x] 4.1 Share the bounded provider listing transport and cover schema-valid IDs, partial results, collisions, and discovery budgets in Go tests.
- [x] 4.2 Separate discovery admission and add the 30-second revision-scoped cache. Verify writer isolation, expiry and revision misses, uncached failures, and eviction.

## 5. Integration checks

- [x] 5.1 Run focused Gateway Go tests and race checks, frontend typecheck/lint/Jest, docs lint, and strict OpenSpec validation. Run the Gateway boundary script and Go lint directly where required by the sandbox; record gaps and use CI for Grafana end-to-end evidence.
