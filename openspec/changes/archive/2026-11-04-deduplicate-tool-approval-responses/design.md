## Context

The registered baseline is `ai@7.0.153`, source `612be2311464804d1e4897afacbf8f3fc2f6d9e3`. Its `collect-tool-approvals.ts` tracks collected tool call IDs after approval/request/call validation and existing-result suppression. Its repeated-response and follow-up tests assert first-decision semantics and one execution/result.

## Goals / Non-Goals

Match that collector behavior for identical, metadata-differing, conflicting, and distinct-approval-ID responses. Preserve invalid-approval errors, existing execution-denied results, and the last-tool-message rule. Do not change execution ordering/concurrency (#444), provider fixtures, or exported APIs.

## Decisions

Use a function-local Go map keyed by tool call ID, populated immediately before appending a collected approval. This is a parity-preserving Go adaptation fixing an implementation bug, not a permanent deviation. Shared orchestration covers StreamText, GenerateText and both ToolLoopAgent entrypoints, including UI histories converted to model messages.

The coverage map already records core orchestration and agents as mixed with root tests. Focused synthetic tests are appropriate for this caller-history bug; no provider input will be fabricated or represented as recorded evidence. No baseline or coverage-map update is required.

## Risks / Trade-offs

Moving the guard before validation could hide malformed later responses. Marking a call before existing-result suppression could incorrectly drop an eligible denial. Tests cover both boundaries. The map is local to one collection, so it does not impose cross-request idempotency.

## Validation

Run the new collection and continuation regressions against unchanged code for red evidence, then the fix for green. Run root tests, race checks, lint, OpenSpec validation, and the direct pinned equivalents of parity-check where available. Inspect tool-approval guidance and maintained Gateway architecture/glossary for affected claims.
