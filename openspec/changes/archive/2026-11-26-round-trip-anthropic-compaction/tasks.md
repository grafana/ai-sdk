# Tasks

## 1. Anthropic compaction lifecycle

- [x] 1.1 Add on-demand compaction request validation, projection and beta selection.
- [x] 1.2 Preserve unary and streaming compaction content and metadata; replay signed blocks unchanged.

## 2. Evidence

- [x] 2.1 Test request/response and real SDK continuation using synthetic native HTTP responses; verify the unchanged unsigned upstream replay.
- [x] 2.2 Verify frontend wire metadata and convert the assembled UI message to a continuation.
- [x] 2.3 Run standalone provider, conformance and integration validation and review the diff against the pin.
