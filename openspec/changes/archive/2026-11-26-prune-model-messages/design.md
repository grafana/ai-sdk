# Design

## Context

The reference is [`prune-messages.ts`](https://github.com/vercel/ai/blob/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/ai/src/generate-text/prune-messages.ts) and its [tests](https://github.com/vercel/ai/blob/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/ai/src/generate-text/prune-messages.test.ts) at `ai@7.0.173`. Go uses the same `provider.Message` type for pre-provider model history and provider prompts. The caller can prune after UI conversion and before approval collection and provider sanitization, where application approval request data is still available.

## Goals / Non-Goals

- Use scoped Go functional options with pinned pruning semantics and stable message/part order.
- Do not validate or repair general message history, change the normal continuation flow, or add provider-specific rules.

## Decisions

- `PruneMessages` takes provider messages and scoped options. Reasoning uses a typed mode; tool scopes use a suffix count instead of encoding counts in strings. Zero prunes across the whole history; negative counts disable that rule. An omitted tool list selects all tools; a supplied empty list selects no known names. Repeatable tool rules execute in order.
- Determine which tool-call and approval IDs the suffix retains. Use a global approval-ID-to-call-ID map for both retained approvals and tool-name selection: a response may be in a separate message with no tool name of its own. Like upstream, a retained approval protects its call, but a retained call/result alone does not protect approval parts outside the suffix. An unresolved approval outside the suffix is removed by an applicable tool rule.
- Remove consecutive reasoning parts together when a tool rule removes the entire following run of other content. If any following part remains in that run, or there is no following part, keep the reasoning group.
- No explicit pruning flags means no parts removed. After all rules, remove zero-part messages by default unless the caller requests keep. Emptiness is based on Go content slices; a text part with empty text is still a part.

## Risks / Trade-offs

- In-place filtering would expose writes to callers that reuse history. Copy the message and content slices before filtering; retain opaque payloads without deep-copying them.
- Tests exercise the actual root continuation boundary as well as isolated filtering. They use test doubles, not new provider recordings or claims about all native adapters.
