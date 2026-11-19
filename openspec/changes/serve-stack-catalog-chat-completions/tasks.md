## 1. Catalog route

- [ ] 1.1 Add the local route-table entry, declared authz operation and model
  invoker grant; regenerate and verify the committed manifest artifacts.
- [ ] 1.2 Add route/manifest tests for the nested mount, distinct telemetry
  label and exact check/grant. Cover shared parent, denied caller, invalid
  catalog (424), disallowed endpoint, unknown model and unmatched requests;
  assert fixed documents or platform boundaries and no provider I/O.

## 2. Defaults and extensions

- [ ] 2.1 Verify precedence on both Chat routes with the shipped tenant
  defaults: omitted/null versus explicit zero, stop and token limits, and
  missing versus caller system instructions. At the native boundary, check
  preserved settings and per-candidate mapping across heterogeneous fallback.
- [ ] 2.2 Exercise operator and applicable tenant test extensions through the
  real Gateway composition. Check unary and setup 403/503 with no native I/O;
  check committed stream denial/unavailability, withholding, and no replay
  or successful terminator. Verify fixed envelopes exclude private data.

## 3. Client and deployed-host evidence

- [ ] 3.1 Extend the official OpenAI Go SDK plugin harness with a saved
  catalog, unary and streaming requests in both access modes, and assertions
  for catalog alias, model and decrypted key at the synthetic native provider.
- [ ] 3.2 Extend the existing pinned-Grafana Playwright catalog-route suite
  for model-invoker RBAC, missing parent and successful unary/SSE calls with
  the stored key. Use CI for deployed-host evidence and report the Cloud limit.

## 4. Documentation and verification

- [ ] 4.1 Update the Chat guide, stack catalog docs and affected architecture
  and glossary entries. Separate Chat and ProviderWire base URLs and note
  access, defaults, error boundaries, no Chat overview and Cloud prerequisites.
- [ ] 4.2 Run focused Go tests, manifest validation and the official SDK
  harness. Verify the pinned CI and parity gates on the candidate head; do not
  change baseline or conformance fixtures without an identified wire change.
