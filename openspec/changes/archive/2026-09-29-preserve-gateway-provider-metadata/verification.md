# Implementation verification and #201 handoff

This change is independently validated without importing the unmerged two-client matrix. At inspection, PR #201 (`nrbrd/ai-gateway-conformance`) was open at `cf1f92fd90a7b148459892b172749a5244dd1fee`. Read-only Git objects show `test/conformance/tools/gateway.mts` discovers two clients, compares captured UI/usage and backend requests against the direct fixture expectations, and writes a per-row report. Its `mise run test-conformance-gateway` accepts `SCENARIO` and `CLIENT` filters. The exact PR head can change; use the rebased ref when executing.

After this change merges, rebase #201 onto it and run both clients against **unchanged** `openai/recorded/simple-text` and `anthropic/recorded/tool-call` inputs and `expected.jsonl`/`expected-requests.jsonl`. For focused reproduction on #201, run `SCENARIO=openai/recorded/simple-text mise run test-conformance-gateway` and `SCENARIO=anthropic/recorded/tool-call mise run test-conformance-gateway`, then run its full matrix before merging #201. Inspect each client's UI chunks **and** backend requests. OpenAI's text-start/end `openai.itemId` is a metadata-only claim; Anthropic's `anthropic.caller` on tool call/output and continued tool use is a narrower claim, with other tool-result request differences still possible. Do not normalize metadata away, alter recorded provider inputs/direct goldens, or mark a row with any mismatch passing. The focused real-handler/Go-client/frontend tests on this branch do not count as #201 matrix replay or parity with Vercel's private service.

Rollout: the strict Go client and ProviderWire server/schema must be published and deployed compatibly. Older Go clients reject the new per-part wire member; a deployment that still serves them must retain the old metadata omission until those clients are updated. No publication or deployment was performed as part of this change.

Local checks for this change:

- `mise deps`: passed.
- `GOWORK="$PWD/go.gateway.work" go test ./ai-gateway/providerwire/v4 ./ai-gateway/cmd/grafana-ai-gateway/internal/service -count=1`: passed.
- `(cd providers/grafana && go test ./...)`: passed.
- `mise run test-ai-gateway`: passed. `TestAnthropicOptionPolicy_ForwardsDirectCallerOnContinuedToolUse` verifies the production policy forwards a continued assistant tool-use with exact direct caller to a local fake Anthropic backend, excluding hostile metadata; `TestProviderOptionPolicy_DirectCallerOnlyOnBasicAssistantCall` checks all other request levels, code-execution caller variants, extra members, aliases and provider-executed calls remain filtered.
- `mise run test-providerwire-v4`: passed (pinned `ai@7.0.109`, `@ai-sdk/gateway@4.0.88`); both clients complete a direct-caller tool-use continuation and safely reject invalid recognized metadata through the real handler.
- `mise run test-integration`: passed; `test/integration/gateway-provider-metadata.test.ts` parses UI chunks with the pinned schema and checks assembled text/tool metadata.
- `mise run parity-check`: passed; direct fixture inputs/goldens unchanged. Recognized namespace invalid surrogate escapes now fail before projection, while a bounded valid large number in an unknown sibling is dropped; the Go client rejects duplicate outer `providerMetadata` keys, including escaped spellings.
- `mise run vet`, `mise run lint`, `mise run build`, `mise run test-short`, `mise run verify-sdk-gateway-isolation`: passed.
- `mise run test-ai-gateway-command` (via `mise run test-integration`): passed, including fake OpenAI Responses end-to-end tests with public `msg_test` itemId and private backend model omitted.
- Metadata-only AO/log/metric coverage: `TestFunctionTools_MetadataOnlyExports` includes allowed item ID/caller and hostile unknown metadata; none reaches those sinks. The Gateway command test asserts `response-metadata.modelId` is `grafana/openai` (not `backend-private`) and response content keeps only the approved `msg_test` item ID.
- `openspec validate preserve-gateway-provider-metadata --strict` and `git diff --check`: passed.

Fixture provenance: no `test/conformance/*/recorded/input*.chunks.txt`, upstream fixtures, direct `expected*.jsonl`, or request goldens were edited. The new Go unit and frontend fake-Gateway scenarios are synthetic *focused boundary tests*, not provider recordings or a replacement for #201's direct-golden matrix.
