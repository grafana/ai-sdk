## Context

The registered reference in `test/conformance/upstream.yaml` is `@ai-sdk/anthropic` 4.0.59 and `@ai-sdk/amazon-bedrock` 5.0.90. Neither knows `claude-sonnet-5-5`. The behavior here comes from `@ai-sdk/anthropic` 4.0.67 and `@ai-sdk/amazon-bedrock` 5.0.99, both published from vercel/ai commit `ec683e7`. The upstream sources are `anthropic-language-model.ts` (`getModelCapabilities`, thinking normalization, JSON-tool selection), `anthropic-prepare-tools.ts` (the forced tool-choice fallback), `amazon-bedrock-chat-language-model.ts` and `amazon-bedrock-prepare-tools.ts`. The upstream tests are the `claude-sonnet-5-5 specific behavior` block in `anthropic-language-model.test.ts` and the `models that reject forced tool use` block in `amazon-bedrock-chat-language-model.test.ts`. Upstream has no `__fixtures__` recordings for this model.

The conformance recorder drives the registered packages, so any recording made today would come from 4.0.59 and would send `disabled` thinking, which this model rejects. Porting ahead of the baseline therefore loses the usual upstream-backed replay. See proposal.md for why the port still goes ahead.

## Goals / Non-Goals

**Goals:** requests that the Claude API, Vertex AI and Bedrock Converse accept for `claude-sonnet-5-5`, with the same rewrites and warnings as the named upstream versions; a record of each difference that the next pinned-version upgrade can check against its target.

**Non-Goals:**
- Advancing the registered baseline.
- The other 4.0.60–4.0.67 capability changes: `claude-opus-5-5` and `claude-fable-5-1` get the same flags upstream, and upstream maps reasoning `none` to effort `low` on models without `between_tools`.
- The Bedrock 5.0.93 sampling-parameter removal for newer Claude models.
- Recorded provider fixtures.

The Opus 5.5 and Fable 5.1 items are already tracked by [#275](https://github.com/grafana/ai-sdk/issues/275). The Bedrock sampling removal is tracked by [#279](https://github.com/grafana/ai-sdk/issues/279), and the missing upstream-backed evidence for this model by [#278](https://github.com/grafana/ai-sdk/issues/278).

## Decisions

1. **Port behavior, not the baseline.** Capability flags, rewrites, warning strings and ordering follow 4.0.67 and 5.0.99. The registered pins stay put, because advancing them is a comprehensive pinned-version upgrade with its own contract (`upstream-parity-governance`). Alternative: run that upgrade first. Rejected because the model is unusable for common calls until then, and the upgrade's comprehensive assessment is not tied to this model.
2. **Capability flags on the existing lookup.** `rejectsThinkingDisabled`, `rejectsForcedToolUse` and `supportsBetweenToolsThinking` are added to `getModelCapabilities`, with a `claude-sonnet-5-5` row before `claude-sonnet-5` because the lookup matches substrings. The Bedrock module keeps its own ID predicates, since it does not import the Anthropic module.
3. **Resolve the caller's forced tool choice after the response format.** In upstream, once the JSON response tool is present, it replaces the caller's tool choice with `required` before `prepareTools` runs. Go therefore applies the forced-choice fallback only when no JSON response tool is used. Otherwise, a named choice would drop the other tools and the warning would be duplicated.
4. **Named-tool matching uses the mapped provider name.** Upstream compares the raw tool name with the wire name, so a provider tool the caller renamed is dropped. Go maps the name first, as the existing `convertToolChoice` does, so a renamed provider tool is kept. This is a parity-preserving Go adaptation for function tools, where both behave the same.
5. **Bedrock reasoning `none` sends `between_tools`.** This is an intentional deviation from 5.0.99, which sends `disabled`. That omits `thinking`, and the model then runs adaptive thinking at its default `high` effort, which is the opposite of what `none` asks for. Go sends `between_tools`, as the Anthropic provider does, and also accepts `reasoningConfig.type: between_tools` with the Anthropic effort limit. Both are recorded in `upstream.yaml` (`bedrock-sonnet-5-5-reasoning-none`).
6. **Bedrock model IDs.** Upstream lists `anthropic.` and `us.anthropic.claude-sonnet-5-5`. Go lists `anthropic.claude-sonnet-5-5` only, because AWS currently offers only the `global.` inference profile for this model. The lists are advisory (`bedrock-provider`), and capability detection matches every prefix. This is a parity-preserving adaptation.

## Divergence classification

| Difference from 4.0.67 / 5.0.99 | Classification |
|---|---|
| Bedrock reasoning `none` and explicit `between_tools` type | Intentional deviation (`upstream.yaml`) |
| Named-tool match after tool-name mapping | Parity-preserving adaptation |
| No `us.anthropic.claude-sonnet-5-5` in the advisory list | Parity-preserving adaptation |
| Tool-choice warning appears before sampling warnings, not last | Parity-preserving adaptation (same warnings, different order) |
| `claude-opus-5-5`, `claude-fable-5-1` flags; reasoning `none` → effort `low` | Implementation gap, tracked by #275 |
| Bedrock 5.0.93 sampling removal for newer Claude models | Implementation gap, tracked by #279 |
| No upstream-backed replay for this model | Coverage gap, tracked by #278 |

## Risks / Trade-offs

- [No upstream-backed conformance replay until the baseline reaches 4.0.67 / 5.0.99] → Focused request tests cover every scenario in the delta specs. Live Claude API, Vertex and Bedrock checks were run manually, not committed. #278 has the next pinned-version upgrade record or import evidence and recheck each classification.
- [Upstream may change these rewrites before the baseline catches up] → The next upgrade assessment compares the specs, which name their source versions, against its target instead of assuming they still match.
- [`anthropic-sdk-go` v1.76.0 bump across consumers] → Covered by conformance replay, the Gateway build, agentobservability and the examples in CI.
