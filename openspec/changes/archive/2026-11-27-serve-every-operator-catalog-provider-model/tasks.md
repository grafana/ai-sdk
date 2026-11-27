## 1. Startup configuration

- [x] 1.1 Reuse the stack catalog's exact blocklist checks for the optional operator YAML fields and validate empty-map and listed-target rules before resolving secrets.
- [x] 1.2 Add strict YAML and startup tests for defaults, types, ID limits, same-instance conflicts (including explicit server fallbacks), and the `allowDefault` refusal.

## 2. Selection and discovery

- [x] 2.1 Share qualified-selector resolution with stack catalogs. Keep listed routes prebuilt; resolve and construct unlisted candidates on the fast and routed paths with the existing configured-account model client.
- [x] 2.2 Test listed and alias precedence, exact matching, refused IDs with no provider calls, literal backend IDs with slashes, account and destination binding, explicit routing controls, and no inherited Anthropic grants or implicit fallbacks.
- [x] 2.3 Reuse stack listing/admission/cache code for operator `/config`. Test authorization, filtering, busy/failure behavior, current limits, and concurrent cache hits without admission on both routes.

## 3. Documentation and checks

- [x] 3.1 Document operator fields, precedence, startup checks and discovery behavior. Align affected architecture and glossary entries.
- [x] 3.2 Run format, lint, Gateway tests, race tests on touched packages, docs lint, module boundary checks, and review and fix the diff.
- [x] 3.3 Sync and archive this change and scan outgoing commits for secrets.
