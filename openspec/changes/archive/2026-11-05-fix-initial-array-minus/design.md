# Design

## Context

See proposal.md for the user-visible failure. The root UI reader and the output package each contain a private partial JSON repair state machine. The registered reference is `ai@7.0.159`, commit `028a025398782f4938303f48fbeee45c79af544b`; its `packages/ai/src/util/fix-json.ts` and tests omit the premature array-start index update.

## Goals / Non-Goals

Restore registered upstream parsing at the initial-minus boundary in both package copies. Keep existing parsing, final validation and stream deduplication intact. Parser consolidation is outside this fix.

## Decisions

Remove only the unconditional last-valid-index assignment before value-start processing in each array-start branch. The shared value-start logic already retains opening containers and waits for digits after a minus. A special-case minus branch would duplicate this logic; an internal shared package would enlarge the change without improving this regression.

Use focused helper tests and public reader/output regressions, plus synthetic provider-independent UI conformance inputs replayed through the exact registered upstream. These are core fixtures, not provider recordings. A first chunk ending in minus is necessary to prove the output stream emits the previously missing partial; a separate minus delta alone can be hidden by normal duplicate suppression.

## Risks / Trade-offs

The two private copies can drift; matching regression cases protect both without restructuring package boundaries. Whitespace-only unfinished arrays now repair to `[]`, matching upstream. No API migration is needed. Reverting the two removed statements restores the prior behavior.
