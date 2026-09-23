## 1. Establish the reference

- [x] 1.1 Check existing upgrade PRs, select once, obtain approval, and retain exact target evidence.
- [x] 1.2 Resolve exact upstream sources without switching the shared checkout and inspect contract changes.
- [x] 1.3 Apply all consumer pins, update the lockfile, and regenerate expectations.

## 2. Assess and correct blockers

- [x] 2.1 Compare supported core, tool, Agent, Output, UI and frontend surfaces; record implementation differences, evidence gaps and exclusions.
- [x] 2.2 Compare provider V4 and declared adapters across construction, requests, continuation, streams, errors and metadata.
- [x] 2.3 Compare Gateway client/runtime boundaries, middleware/registry and harness evidence limits.
- [x] 2.4 Explain generated changes, import required stream fixtures byte-for-byte, and correct replay/integration failures with regression evidence.
- [x] 2.5 Verify published-module ordering for the delivered scope and record the owner-approved separate Mantle adoption under #207.

## 3. Register and review

- [x] 3.1 Search labeled/unlabeled open/closed work, read candidate discussion/PRs, and register coherent remaining packages.
- [x] 3.2 Verify upstream-sync on 23 new and 22 reused packages and link them from the coverage assessment.
- [x] 3.3 Review upstream-to-Go breadth and Go-to-requirement necessity with the parity-review skill; record residual decisions and evidence limits.

## 4. Validate and prepare publication

- [x] 4.1 Run parity, standalone modules, boundary, frontend/Gateway, build, test, race, lint, docs and native-image checks; repeat generation for stability.
- [x] 4.2 Re-attest Gateway mapping, record reviewed verification date and rerun the full parity gate.
- [x] 4.3 Verify the eleven implemented requirements across six capability specs and prepare validated delta synchronization/archive.
- [x] 4.4 Prepare the conventional signed delivery and draft PR description with linked packages and limitations.

Synchronization/archive, the clean-tree formatting check, signed commit, push and
one draft PR follow this implementation checklist. Their actual outcomes belong
in the verification/delivery record; these checkboxes do not claim publication
has already occurred. Remote CI, especially multi-platform image validation,
remains a separate check on the published candidate.
