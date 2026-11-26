# Design

## Context

The reference is Vercel AI SDK `0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1` (ai 7.0.173, @ai-sdk/anthropic 4.0.105). Its model accepts only `summarize`, sends the September compact beta, projects compaction as text with Anthropic metadata, and replays string signatures verbatim. Older threshold compaction arrives without a signature and uses deltas.

## Goals / Non-Goals

Preserve nonempty summary bytes, optional signature presence and the legacy wire shape across request, unary, streaming and continuation. Do not add context-editing controls, message threads, encrypted compaction fields, Gateway or Bedrock APIs, model eligibility tables, or changes to usage and stop-reason normalization.

## Decisions

- Use a typed optional configuration and the existing SDK request and content block fields. Preserve an explicit empty instructions string; reject an invalid option before I/O.
- Call-level context editing is not represented in the Go provider. Detect conflicting non-null raw `contextManagement` when the new on-demand option is used, but do not implement context editing. Low-level constructor transport overrides remain outside the provider-option contract.
- Emit the signature on text-start metadata in streaming. A signed start with nonempty content is authoritative; any redundant compaction delta for that index is ignored rather than changing authenticated text. A legacy unsigned start still uses its deltas. Keep empty block handling consistent with existing text lifecycles and prompt omission.
- Automatic beta selection is driven by the on-demand option or a retained signed replay block. Neither is triggered by an empty history block. Deduplicate against caller betas.

## Risks / Trade-offs

The imported threshold stream demonstrates unsigned behavior only. New provider and frontend tests use synthetic native responses and do not establish live provider acceptance. Existing Gateway native-option forwarding remains unchanged and can pass through behavior now consumed by the provider; no Gateway-specific certification is claimed.
