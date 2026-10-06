## 1. Baseline and Inventory

- [x] 1.1 Extend baseline validation to conformance tools, integration tests, and CLI tooling.
- [x] 1.2 Update integration and CLI AI SDK package pins to the registered baseline.
- [x] 1.3 Add a parity coverage inventory script and `mise run parity-coverage`.
- [x] 1.4 Add missing local upstream fixture names to provider `INDEX.yaml` files as imported or explicit `null`.

## 2. Harness Expressiveness

- [x] 2.1 Add `toolChoice` config support to Go replay and TypeScript generation/recording.
- [x] 2.2 Add `activeTools` config support to Go replay and TypeScript generation/recording.
- [x] 2.3 Add `streamOptions` config support to Go replay and TypeScript generation/recording.
- [x] 2.4 Add tool `providerOptions` support to provider-defined tool configs.
- [x] 2.5 Add tool error simulation support for function tool configs.
- [x] 2.6 Add focused tests covering the expanded config behavior.

## 3. Frontend and API Shape

- [x] 3.1 Add hook-level frontend interop tests for `useChat`, `useObject`, and `useCompletion`.
- [x] 3.2 Add a provider V4 API-shape drift report script and mise task.
- [x] 3.3 Update parity docs and specs with the new automated coverage signals.

## 4. Verification

- [x] 4.1 Run baseline validation and coverage inventory.
- [x] 4.2 Run conformance tests.
- [x] 4.3 Run integration tests.
- [x] 4.4 Run OpenSpec status for the change.
