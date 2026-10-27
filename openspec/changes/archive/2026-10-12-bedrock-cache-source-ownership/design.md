## Context

Registered reference: @ai-sdk/amazon-bedrock 5.0.105 (tag commit eb77f09e3c06c28e860d92e0de941b143c2eecec); provider 4.0.21. Upstream filters inactive tool history before conversion and suppresses cache-only assistant messages.

## Decisions

Use the actual prepared tool configuration, including synthetic JSON tools. Filter immutable source copies before grouping. Compare content counts at each source boundary before appending its cache marker. This prevents a dropped source from borrowing a neighbor's content.

Per-source ownership for user/tool/system messages is an intentional robustness deviation from upstream unconditional marker insertion, requested by #352 and recorded in PARITY.md. Preserve unsigned-reasoning role boundaries and existing non-tool prefill semantics.

## Risks

Tool preparation now precedes prompt conversion: warning ordering and invalid-input error precedence can change. Removed tool sources no longer have options parsed. Retained source options still validate. Synthetic HTTP coverage verifies serialization, not live cache acceptance or hits.

## Validation

Regression tests cover inactive tools, effective tool choice, native versus synthetic JSON output, approvals, unsigned/signed reasoning, source ownership, TTL, immutability and validation boundaries. Converse and ConverseStream HTTP tests assert serialized requests. Required repository gates are recorded separately in the PR, including pre-existing harness limits.
