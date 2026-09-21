# Validation and provenance summary

All nine approved request corrections are implemented, with red-first regression
tests. Target provider versions are Anthropic 4.0.57, OpenAI 4.0.70 and Bedrock
5.0.87 at `b3033f77f0459bcc68df182c8052f52305467bcc`; the complete target set is
in `design.md`. Candidate installation retained the 4320-minute release-age gate.
Canonical pins, lockfiles, expectations and provider inputs are unchanged.

## Validation

- Passed: root and Anthropic/OpenAI/Bedrock `go test ./...` suites, including Mantle.
- Passed: focused race tests, `mise run build`, `mise run vet`, `mise run lint`.
- Passed: registered-baseline `mise run parity-check` before and after changes.
- Passed: `mise run test-integration` (6 files, 28 tests).
- Passed: strict OpenSpec validation and whitespace checks.
- Passed: 442 exact-target request probes and candidate `mise run generate-conformance`.
- **Failed:** candidate `mise run test-conformance`, in 14 unique scenarios below.
  Passing generation is not passing target replay or full target certification.

## Residual owners

Scenario names are relative to their provider's conformance directory.

| Owner | Scenarios and differing fields |
| --- | --- |
| PR 2H, Anthropic caller metadata | `recorded/multi-step-web-search`, `recorded/web-search-citations`, `upstream/programmatic-tool-calling`, `upstream/tool-search-bm25`, `upstream/tool-search-deferred-bm25`, `upstream/tool-search-deferred-regex`, `upstream/tool-search-regex`, `upstream/web-fetch-tool-20260209` |
| PR 2H, OpenAI assistant history | `upstream/mcp-tool-approval-3`, `upstream/shell-local-multiturn`: string content replaces output-text arrays; shell history also drops a stored item ID |
| PR 6, Anthropic cross-call IDs | `recorded/multi-tool-chain`, `recorded/tool-approval-request`, `upstream/tool-search-bm25`, `upstream/tool-search-deferred-bm25`, `upstream/tool-search-deferred-regex` |
| PR 2S, OpenAI finish reason | `upstream/apply-patch-continuation`, `upstream/apply-patch-delete`: target `tool-calls`, Go `stop` |

Three caller cases overlap ID cases: 10 + 5 + 2 owner assignments represent
**14 unique scenarios**. Approval-ID `id-2` → `id-3` follows shared generator
consumption and belongs to PR 6, not an inferred approval-policy defect.
PR 7 owns canonical expectations and cumulative certification, not these fixes.
The residuals do not expand or independently block scoped PR 1 review.

## Coverage and handoff

No authentic inputs cover the new request edges. Focused tests and synthetic
request probes are bounded evidence, not recordings, live Mantle/Vertex-auth
verification, complete provider-tool schemas, or full response-stream parity.
Raw probes, logs, candidate metadata and diffs are preserved externally and are
not part of the PR. In-session spec/source verification found no remaining
blocking issue within the nine; archival remains before merge.

Private raw budget presence preserves explicit zero while typed zero remains
omitted; caller schemas stay unchanged. PR 2H/2S retain history/stream ownership,
PR 3 retains returned-call enforcement, and unrelated public APIs remain
planner-owned pending decisions rather than PR 1 dependencies or PR 7 leftovers.
