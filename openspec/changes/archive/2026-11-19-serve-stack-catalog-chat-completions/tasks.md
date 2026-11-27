## 1. Catalog route

- [x] 1.1 Add the local route-table entry, declared authz operation and model
  invoker grant; regenerate and verify the committed manifest artifacts.
- [x] 1.2 Add route/manifest tests for the nested mount, distinct telemetry
  label and exact check/grant. Cover shared parent, denied caller, invalid
  catalog (424), disallowed endpoint, unknown model and unmatched requests;
  assert fixed documents or platform boundaries and no provider I/O.

## 2. Defaults and extensions

- [x] 2.1 Verify precedence on both Chat routes with the shipped tenant
  defaults: omitted/null versus explicit zero, stop and token limits, and
  missing versus caller system instructions. At the native boundary, check
  preserved settings and per-candidate mapping across heterogeneous fallback.
- [x] 2.2 Exercise operator and applicable tenant test extensions through the
  real Gateway composition. Check unary and setup 403/503 with no native I/O;
  check committed stream denial/unavailability, withholding, and no replay
  or successful terminator. Verify fixed envelopes exclude private data.

## 3. Client and deployed-host evidence

- [x] 3.1 Extend the official OpenAI Go SDK plugin harness with a saved
  catalog, unary and streaming requests in both access modes, and assertions
  for catalog alias, model and decrypted key at the synthetic native provider.
- [x] 3.2 Extend the existing pinned-Grafana Playwright catalog-route suite
  for model-invoker RBAC, missing parent and successful unary/SSE calls with
  the stored key. Use CI for deployed-host evidence and report the Cloud limit.

## 4. Documentation and verification

- [x] 4.1 Update the Chat guide, stack catalog docs and affected architecture
  and glossary entries. Separate Chat and ProviderWire base URLs and note
  access, defaults, error boundaries, no Chat overview and Cloud prerequisites.
- [x] 4.2 Run focused Go tests, manifest validation, the official SDK
  harness and parity baseline validation. No wire or conformance fixture
  change is needed.

The deployed-host acceptance gate is the pinned Grafana Playwright run on the
PR head. It requires CI after publication; the local SDK harness does not
substitute for it.
