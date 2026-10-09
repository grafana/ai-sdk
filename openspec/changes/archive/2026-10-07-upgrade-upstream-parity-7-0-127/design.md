## Context

The selected coherent reference is ai 7.0.127. Exact Anthropic 4.0.71 source at `45e1fbc3ea10d22f852e12b1c1851e289c85e835` maps fallback blocks to `anthropic.fallback` custom content in `anthropic-language-model.ts`, validates continuation in `convert-to-anthropic-prompt.ts`, and treats fallback as a reasoning-sort boundary. Matching tests cover unary/stream output, mid-output hops and invalid metadata. The existing imported fallback fixture fails after expectation regeneration.

## Goals / Non-Goals

**Goals:** Preserve fallback model identities, order and continuation; validate the fixed reference and account for supported-surface differences.

**Non-Goals:** Implement deferred parity packages, change Go public APIs, introduce client-directed fallback policy or claim live provider acceptance.

## Decisions

- Reuse existing provider/core/UI custom parts and Anthropic SDK fallback unions rather than add another event family.
- Validate required type/from/to/model fields in continuation, warning and omitting invalid metadata as upstream does. Do not attach cache control to fallback markers.
- Preserve fallback markers and model-specific reasoning signatures in their original order. Go does not perform upstream's segmented tool-use reordering; that older normalization gap remains deferred rather than being claimed as implemented.
- Preserve existing recorded/imported inputs; import the target mid-output fallback fixture byte-identically and regenerate expectations. Synthetic focused/frontend tests supplement rather than replace provenance.
- Match ai 7.0.127 process-ui-message-stream.ts and convert-to-model-messages.ts: persist accumulated raw input strings, restore only last-step streaming tools, use Input for rejected calls, replace output tool metadata and remove superseded pending approvals.
- Reuse reader initialization for finish-callback assembly; report malformed initial streaming input through the existing helper error/closure contracts. No public signature changes.
- Refresh Gateway attestation only after exact-source mapping and pinned-client evidence pass. Leave verification unset until candidate checks pass.

## Risks / Trade-offs

- Published root types could differ from workspace types → verify standalone provider dependencies before claiming delivery.
- Synthetic tests do not establish live fallback acceptance → retain the evidence boundary and authentic replay.
- An unresolved supported-integration incompatibility blocks publication → preserve partial work and report it rather than weaken checks.
