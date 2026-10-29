## 1. Separate compatibility fixes

- [x] 1.1 Retain flushing when socket deadlines are unsupported and preserve
  deadline handling for writers that support it.
- [x] 1.2 Discard the known Anthropic metadata part, preserving canonical
  identity and rejecting unknown custom kinds.
- [x] 1.3 Add focused regressions without changing provider recordings.
- [x] 1.4 Document the shared runtime/lifecycle blocker and evidence limits.

## 2. Verification and archive

- [x] 2.1 Run package tests with race detection on the fixes branch.
- [x] 2.2 Validate the change with OpenSpec 1.14.3, sync and archive it.

The restack uses #555/#579/#581; #483 lifecycle validation remains pending and
is not marked complete by these tasks.
