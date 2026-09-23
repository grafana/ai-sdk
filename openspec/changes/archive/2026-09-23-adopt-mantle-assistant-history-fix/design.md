## Context

Issue [#207](https://github.com/grafana/ai-sdk/issues/207) is a provider-implementation defect at the published dependency boundary, not a missing encoder in this workspace. `mantle.NewResponses` constructs the authenticated Bedrock client and delegates to `openaiprovider.NewResponsesWithClient`. Bedrock still requires OpenAI `v0.0.0-20260910193046-bcca929c67e5`; the workspace producer already has the correction from merged [#230](https://github.com/grafana/ai-sdk/pull/230).

The selected adoption version is `v0.0.0-20260923145714-9fb535a02817`, from merge commit `9fb535a028172fd48a73457c655d1389d6e6290c`. During planning, GitHub confirmed #230 was merged and `GOWORK=off GOPROXY=https://proxy.golang.org GONOPROXY=none go mod download -json github.com/grafana/ai-sdk/providers/openai@v0.0.0-20260923145714-9fb535a02817` succeeded. This establishes availability, not yet standalone regression success or fresh-cache verification.

### Registered upstream reference

`test/conformance/upstream.yaml` registers commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`, OpenAI 4.0.71 and Amazon Bedrock 5.0.88. The inspected sources match that reference; no alternate baseline is used:

- [Responses conversion](https://github.com/vercel/ai/blob/08ae5ad05bc12496dd1ffcf64e34419e0831300d/packages/openai/src/responses/convert-to-openai-responses-input.ts): assistant text emits an item reference when storage is enabled and an ID exists; otherwise it emits string content, preserves phase, and omits the stale ID.
- [Converter tests](https://github.com/vercel/ai/blob/08ae5ad05bc12496dd1ffcf64e34419e0831300d/packages/openai/src/responses/convert-to-openai-responses-input.test.ts): cover no-ID text and phase-bearing non-stored reconstruction.
- [Mantle construction](https://github.com/vercel/ai/blob/08ae5ad05bc12496dd1ffcf64e34419e0831300d/packages/amazon-bedrock/src/mantle/bedrock-mantle-provider.ts): delegates Responses behavior to the shared OpenAI model.

Local `providers/openai/assistant_history_test.go` already checks string content, empty text, both phases, stale-ID omission, and stored references. Mantle tests currently prove stored continuation, route/auth policy, and stream metadata, but not reconstructed history. `PARITY.md` classifies provider adapters as mixed coverage and explicitly distinguishes workspace substitutions from published adoption.

## Goals / Non-Goals

**Goals:**
- Deliver the merged encoder correction to standalone Bedrock consumers.
- Prove unary and streaming Mantle requests preserve the existing Responses assistant-history contract.
- Keep existing routing, authentication, provider identity, and OpenAI metadata behavior intact.
- Record bounded deterministic evidence and supplemental live results without overstating parity.

**Non-Goals:**
- A Mantle-specific encoder or transport request rewriting.
- New APIs, baseline changes, root module dependency upgrades absent a demonstrated requirement, or unrelated module refreshes.
- Mantle Chat, reasoning-summary controls, web-search includes, or changes to frontend SSE behavior.
- Synthetic provider recordings or credential-dependent CI.

## Decisions

### Adopt the published producer, not another implementation

Update Bedrock's OpenAI requirement and the necessary checksums using the exact selected version with `GOWORK=off`. The producer retains the same root SDK and OpenAI Go SDK requirements. The first parity-check exposed one additional direct consumer: `test/conformance/go.mod` requires the older OpenAI version while replacing Bedrock and OpenAI with local modules; Go's readonly graph rejects the mismatch. With owner approval, align that test module's OpenAI requirement and necessary sums to the same published revision, keeping its existing test-only replacements. Inspect resulting module diffs and stop for approval if other dependencies change.

A local `replace`, workspace-only verification, an unmerged pseudo-version, or a Mantle request-rewriting workaround would fail the consumer-delivery requirement and duplicate the shared adapter's responsibility. No change to `mantle/provider.go` is expected.

### Test at the Mantle HTTP boundary

Extend the existing fake `http.RoundTripper` pattern in `providers/bedrock/mantle/provider_test.go`. Construct the actual model through `NewResponses`, exercise both `DoGenerate` and `DoStream`, decode captured request JSON, and compare the complete assistant item. Keep synthetic unary/SSE responses within focused tests; drain streaming results and assert no error parts and a terminal finish.

Use user → assistant → user history with the code-word prompt from #207. The table covers both call modes with:

| Storage / assistant metadata | Expected assistant input |
| --- | --- |
| `store: false`, no ID | String content, no output-message fields |
| `store: true`, no ID | String content, not an item reference |
| `store: false`, stale ID | String content, ID omitted |
| `store: false`, stale ID and `commentary` or `final_answer` phase | String content and matching phase, ID omitted |
| `store: false`, empty text | Explicit empty string content |
| `store: true`, stored ID | `item_reference`, no resent assistant text |

Assert the surrounding user items and ordering, explicit store setting, and streaming flag where applicable. Exact assistant-item equality rejects the old `output_text` array shape and accidental `id`/`status` fields. Retain the existing response-derived item-reference test and metadata attribution checks rather than replacing them with hand-authored metadata alone.

First run the new regression against the existing requirement with `GOWORK=off` and capture the expected wire-shape failure. Workspace success at this point is expected because its producer is already corrected. Then bump the requirement and rerun both contexts. Existing producer fixtures already cover conversion; a fabricated Mantle recording adds no valid provider evidence.

### Treat module resolution as an acceptance gate

After adoption, run Bedrock tests in workspace mode and `GOWORK=off go test -mod=readonly ./...` from `providers/bedrock`. Run `mise run verify-module-resolution`, which uses a fresh cache, public proxy, disabled workspace, no production replacements, and standalone tests for published modules. Also run Bedrock vet/lint and `mise run parity-check` for the parity-sensitive change; the conformance module's test-only local replacements remain unchanged.

No frontend chunk or framing change is intended, so a new cross-language scenario is not warranted. Reconsider that boundary if implementation introduces output-stream changes. Do not regenerate conformance expectations unless an actual request or output change in existing fixtures requires it; inspect and explain any resulting differences.

### Keep coverage claims scoped

Update `test/conformance/PARITY.md` only after passing adoption checks, with a Mantle-specific evidence entry linking #207 and identifying unary/streaming request capture plus standalone module testing. Keep it a stable coverage statement, not an issue status log. Synthetic tests do not establish live service acceptance, and the earlier producer PR alone did not deliver the fix to Bedrock consumers. Existing Mantle Chat gaps remain unchanged.

When credentials are available, use the real Mantle adapter with `openai.gpt-oss-20b` in `us-east-2`, user `Remember a code word.`, assistant `The code word is cobalt.`, user `Repeat the code word only.`, `store: false`, and `max_output_tokens: 256`. Require a completed Responses result and text recalling `cobalt`, not merely HTTP 200. Observe status through existing raw response data or a pass-through capture without rewriting requests. Prefer temporary `workloads-dev` SSO credentials, keep secrets out of artifacts, and record unavailable credentials as a supplemental validation gap rather than fabricating a result.

## Risks / Trade-offs

- Workspace substitution masks the defect → demonstrate the red/green transition with standalone readonly tests.
- A producer revision contains more than this fix → review the dependency diff and run the full Bedrock suite, existing routing/authentication/metadata tests, and parity gate.
- HTTP capture proves conversion, not live compatibility → retain the issue's prior live evidence and repeat the bounded recall check when available.
- Global module verification fails for an unrelated module → report the exact blocker and ask before expanding scope; do not silently weaken the gate.

## Migration Plan

1. Add deterministic regressions and establish the old standalone failure.
2. Adopt the public merged producer and update required sums without replacements.
3. Pass standalone/workspace, parity, and fresh-cache module gates; record supplemental live evidence or its limitation.
4. Update the coverage record and publish the Bedrock adoption change. Consumers must select a Bedrock revision containing this bump; publication of the OpenAI producer alone is insufficient.

There is no API or data migration. Reverting the dependency bump would restore the known GPT-OSS defect; do not describe such a rollback as preserving corrected behavior.

## Open Questions

None blocking planning. Credential availability and live model access will be checked during implementation; successful live validation is supplemental to deterministic CI.
