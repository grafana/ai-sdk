## 1. Isolate the internal foundation

- [x] 1.1 Extract the unchanged request-local collector, wrappers, projection, retention and tests; leave production service/handler wiring absent.
- [x] 1.2 Extract the namespace schema and strict schema tests (the original jsontext implementation required Gateway Go 1.27; task 3 removes that requirement).

## 2. Validate the foundation

- [x] 2.1 Run full Gateway tests, focused collector/handler/service races, Gateway vet/lint and schema TypeScript checks.
- [x] 2.2 Run ProviderWire/parity, authenticated command, workspace/boundary and documentation checks, proving unchanged production output.
- [x] 2.3 Record package/schema evidence limits, validate this OpenSpec change and commit only the foundation slice.

## 3. Owner-approved simplification

- [x] 3.1 Replace token-level projection with standard Go JSON normalization and protected data transformation; update regressions for duplicate handling, normalized escapes and numeric precision.
- [x] 3.2 Separate synchronized execution facts from metadata assembly; centralize allocation enforcement and remove repeated state encoding and retention rescans.
- [x] 3.3 Remove jsontext and the Gateway-only toolchain increase; retain the schema as a contract-test artifact and document the superseding normalization decision.
- [x] 3.4 Run package/race/lint, schema/parity, command, standalone Go 1.26 and boundary checks; document downstream API migration and push the reviewed PR update.
