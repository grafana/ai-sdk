## Context

This is the SDK prerequisite slice of #237 / WP13 (#107). The registered baseline is unchanged: `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, provider 4.0.7, ai 7.0.65, Anthropic 4.0.38 and Gateway 4.0.52. Reference paths are the V4 provider tool/result/stream types, `stream-language-model-call.ts`, `to-ui-message-chunk.ts`, Anthropic language-model options/prompt conversion, and Gateway client serialization.

## Goals / Non-Goals

Goals: make direct SDK validation, native conversion and Go client decoding coherent; preserve marker presence where operationally meaningful; publish usable independent modules.

Non-goals: enabling Gateway provider tools or MCP, changing its fallback/egress policy, introducing server correlation state, upgrading the baseline or modifying authentic provider fixture inputs.

## Decisions

- Keep the existing flat Go tool type and add shared validation instead of importing an HTTP schema. Nil Go args adapt to upstream factories' default `{}`; HTTP missing/null args remain invalid.
- Use booleans for preliminary and unary dynamic. Retain streaming dynamic pointers: pinned text input-start uses an explicit value before definition inference, while UI projection independently classifies known application tools. Test both layers through the pinned frontend parser.
- Extend the standalone Go client's closed codecs, not server types. Decode required continuation metadata with an explicit allowlist, retain bounded I/O and client-owned response/warning replacement, and do not duplicate server lifecycle checks.
- Preserve native MCP token/enabled omission using pointers. A caller context deadline supplies the Anthropic SDK timeout default before explicit request options, avoiding the large-token preflight refusal without reducing the model token budget. No-deadline guard and streaming remain unchanged.
- Preserve the existing published Apache commit chain through `15a57823`; do not rewrite immutable module source refs. Update Gateway pins and its boolean consumers in a separate AGPL compatibility commit, retaining rejection tests. A pure Apache-only split would leave workspace consumers uncompilable.

## Risks / Trade-offs

- Source-breaking marker/MCP fields → migrate all compiled consumers and test released modules with `GOWORK=off`.
- Client readers become broader than current service output → document this as readiness, not enabled service support.
- A prerequisite accidentally enables effects → retain Gateway rejection tests; runtime activation and MCP routing each get separate unarchived changes.
- Synthetic tests mistaken for provider evidence → keep provider fixtures unchanged; use transport tests only for their stated boundary.

## Migration Plan

Merge this PR first, then `gateway-provider-tool-runtime`, then `gateway-anthropic-mcp`. Each successor is tested against its committed published dependencies. Distribute docs and parity claims with their implementation. No change is archived during review. Rollback requires restoring the corresponding module set; no persisted state exists.

## Open Questions

None for SDK readiness. Live MCP remote-egress and deployment approval belong to the final service slice.
