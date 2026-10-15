## Context

The registered `@ai-sdk/anthropic@4.0.71` converter and its cache-iteration test retain top-level cache counters. Exact tag commit `45e1fbc3ea10d22f852e12b1c1851e289c85e835` was reviewed. This correction is an intentional bug-fix deviation, registered in `upstream.yaml` and `PARITY.md`.

## Decisions

Sum cache reads and writes over selected `message` and `compaction` iterations. Use the existing selector for all normalized counters: any fallback iteration or no executor iterations keeps top-level counters. Missing/null executor cache counters contribute zero. Advisor counters remain in raw usage for separate attribution.

Adding top-level counts would double-count server-tool loops. Preserving provisional streaming counts would miss on-demand compaction and cannot reliably aggregate multiple iterations. Raw usage and stream lifecycle remain unchanged.

## Evidence and limits

Twelve scenarios exercise generation and terminal stream usage through the actual SDK HTTP decoder. Existing imported compaction fixtures have no cache activity, so synthetic regressions establish conversion behavior, not live billing evidence. Streams exposing iterations only at message_start remain outside this change.
