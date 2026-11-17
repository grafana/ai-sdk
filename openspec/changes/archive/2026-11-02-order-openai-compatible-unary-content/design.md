## Context

Baseline: `@ai-sdk/openai-compatible@3.0.78`, source
`612be2311464804d1e4897afacbf8f3fc2f6d9e3`, registered in
`test/conformance/upstream.yaml`. Upstream
`packages/openai-compatible/src/chat/openai-compatible-chat-language-model.ts`
appends reasoning, converted content, then tools. Its matching test
"should prefer reasoning_content over reasoning field when both are provided"
asserts reasoning before text. The Go converter and an existing test instead
place text first. This is an implementation bug in the mixed provider-adapter
surface in `test/conformance/PARITY.md`.

## Decisions

Move the text append after the existing reasoning selection. Preserve the
nonempty `reasoning_content` preference and fallback to `reasoning`, empty-text
omission, tool IDs, arguments and metadata. `DoStream` is independent.

Use synthetic HTTP unit and consumer integration tests: no authentic recorded/imported fixture covers
mixed unary reasoning and text. Existing snapshots therefore need no regeneration.
Exercise response messages through `middleware.SimulateStreaming`, which invokes
`DoGenerate`, then `GenerateText`. Plain `GenerateText` collects `DoStream` and
would not prove this fix.

## Non-goals and existing boundaries

This change establishes ordering only. Upstream uses nullish reasoning-field
selection, whereas Go falls back for an empty primary string. Go currently
accepts string/null text content and rejects arrays; upstream normalizes arrays
including thinking parts. Preserve these existing boundaries; this change does
not claim full unary normalization parity or add invented provider recordings.

## Risks and validation

Consumers relying on text-first unary ordering observe the upstream order after
this fix. Run regressions before and after the change, provider tests with race,
vet/lint, OpenSpec validation and the repository parity-check components. Keep
any unavailable checks explicit in the PR evidence.
