# Tasks

## 1. Uploaded user-file conversion

- [x] 1.1 Add synthetic unary and streaming request tests for exact namespaces, empty IDs, media independence, missing/invalid references, metadata, ordering and input immutability; reproduce failure on the unchanged converter.
- [x] 1.2 Thread the configured reference namespace and emit presence-aware file IDs; verify the focused tests and existing inline/URL tests pass.
- [x] 1.3 Document uploaded user-file usage and synthetic-only evidence in the provider guide and upstream.yaml; verify documentation and baseline validators.

## 2. Integration validation

- [x] 2.1 Run provider tests, race tests and vet plus applicable parity-check components; record exact outcomes and any environment limits without claiming unrun checks.
- [x] 2.2 Compare implementation to the pinned upstream and delta scenarios, validate OpenSpec artifacts and inspect the final diff for scope and provenance.

## Workflow follow-up

- Sync the delta and archive the completed change; validate the archived result.
- Recheck eligibility and publish the signed branch as its own draft PR only.
