## 1. Execution boundary

- [x] 1.1 Extract the selector API, Gateway-control handling and shared deadline with unary/streaming regression tests.
- [x] 1.2 Keep every command call site on the catalog adapter; retain existing account/listener behavior.
- [x] 1.3 Extract bounded native construction, ordered default fallback and transport/isolation/preflight tests.
- [x] 1.4 Record internal-engine coverage separately from authenticated service or deployed acceptance.

## 2. Independent validation

- [x] 2.1 Run Go/race, parity, command/integration, build/vet/lint, docs, module-policy and image gates on this branch.
- [x] 2.2 Validate this OpenSpec change and verify the new engine is not wired into the command.
