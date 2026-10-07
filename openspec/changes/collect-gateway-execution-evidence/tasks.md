## 1. Isolate the internal foundation

- [x] 1.1 Extract the unchanged request-local collector, wrappers, projection, retention and tests; leave production service/handler wiring absent.
- [x] 1.2 Extract the namespace schema and strict schema tests and retain the approved Gateway-only Go 1.27 baseline.

## 2. Validate the foundation

- [x] 2.1 Run full Gateway tests, focused collector/handler/service races, Gateway vet/lint and schema TypeScript checks.
- [x] 2.2 Run ProviderWire/parity, authenticated command, workspace/boundary and documentation checks, proving unchanged production output.
- [x] 2.3 Record package/schema evidence limits, validate this OpenSpec change and commit only the foundation slice.
