# Tasks

## 1. Regression and resolution

- [x] 1.1 Add paired request/warning tests for disabled, explicit budget/effort, zero/default controls and namespaces; demonstrate the existing implementation fails.
- [x] 1.2 Resolve explicit controls before derivation and copy caller config; verify focused request tests and repeated/concurrent reuse including provider-default between-tools behavior.
- [x] 1.3 Exercise both Converse endpoints with synthetic transport, asserting serialized requests and surfaced warnings; document precedence in the existing Bedrock guide.

## 2. Integration validation

- [x] 2.1 Run Bedrock/Mantle tests, race tests, vet, existing conformance and available parity checks; record exact results and any environment limits without fabricating fixture provenance.
- [x] 2.2 Compare the final implementation to pinned upstream and validate the OpenSpec delta, preserving the existing Nova token omission and between-tools policies.

## Workflow follow-up

- Sync the completed delta and archive the change.
- Recheck issue eligibility and publish the own signed branch as a draft PR with evidence and limits.
