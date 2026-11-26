## Context

At vercel/ai `0e8e0543` (`ai@7.0.173`), `toolSearch` takes a result limit and
a ranked-name callback. `createToolSearchState` snapshots eligible tools and
commits discovery only after resolving the ranked names against that snapshot.
Go already has the same lifecycle but a fixed matcher and a five-match cap.

## Decisions

- Use functional options on `ToolSearch` with an immutable per-tool
  configuration. Validate the explicit Go integer bound while validating the
  original registry, keeping the ordinary constructor return type. The default
  is five; JavaScript's safe-integer ceiling does not constrain Go integers.
  Calls without options keep their behavior. Code that uses `ToolSearch` as a
  `func() Tool` value needs a zero-argument wrapper around `ToolSearch()`.
- Use a context-aware callback that returns ranked names and an error, and
  supply a detached name/description slice in deterministic name order.
  Descending keyword score and ascending-name ties remain the Go default.
- Resolve output against the captured candidates, not callback-mutated data.
  Check cancellation before committing matches. Propagate callback errors
  through the existing local tool error path without activating any names.
- Extend provider-independent direct and nested core fixtures only. No common
  fixture runner, provider history, or generation loop changes.

## Risks

Rankers may run concurrently, and should honor the supplied execution context.
The SDK does not stop non-cooperative callbacks or roll back already completed
searches. No ranker receives schemas or an execution registry.
