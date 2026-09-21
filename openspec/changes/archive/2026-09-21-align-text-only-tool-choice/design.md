## Context

On current main, `streamtext.go` resolves configured and per-step tool choice before provider invocation, but only supplies the automatic default when effective tools are nonempty. `GenerateText` and both Agent entry points use this engine. The Grafana client preserves explicit choice, and the Gateway's shared function-tool mapper already accepts it in both wire modes. The remaining service incompatibility is `fallbackTextRequest`, which rejects every nonnil choice.

The registered baseline is `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`: `ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, provider `4.0.7`, and Anthropic `4.0.38`. Pinned source and tests establish:

- `packages/ai/src/prompt/prepare-tool-choice.ts` defaults omitted choice independently of tools and preserves explicit choices.
- `packages/ai/src/generate-text/stream-text.ts` uses non-nullish step choice before configured choice. The generate path uses the same preparation helper.
- `packages/gateway/src/gateway-language-model.ts` preserves call options in both request modes.
- `packages/anthropic/src/anthropic-prepare-tools.ts` omits choice without tools, so direct Anthropic request snapshots cannot alone expose the core omission.

Per `test/conformance/PARITY.md`, this work spans core orchestration, provider call options, and Gateway host compatibility. Core tools-dependent defaulting is an implementation bug. Rejecting harmless auto on fallback routes is a Gateway compatibility bug against the public client's emission; no parity claim is made about Vercel's private service. Missing high-level request assertions are a coverage gap. Go's shared streaming engine for GenerateText is a parity-preserving adaptation.

## Goals / Non-Goals

**Goals:** align automatic choice preparation across shared callers; admit and preserve text-only auto on fallback routes; prove actual high-level client requests and retain existing safety boundaries.

**Non-goals:** change existing direct-route function-tool mapping, enable effectful fallback, modify serializers or schemas, upgrade dependencies, change authentication or UI/SSE behavior, or resolve high-level TypeScript `generateText` body headers and default unary token limits.

## Decisions

### 1. Default only after effective choice resolution

Remove the tool-count condition from the existing nil-choice default branch in `streamtext.go`. Keep precedence as nonnil `PrepareStepResult.ToolChoice`, configured choice, then auto. Tool filtering remains independent. Do not mutate configuration or carry a step override into later steps; explicit choices continue unchanged even with empty tools.

Defaulting in the Grafana client would leave other providers inconsistent. Defaulting in provider adapters would conflate core intent with provider-specific serialization. No new preparation API is needed.

### 2. Narrow the fallback guard, not the mapper

In `fallbackTextRequest`, accept nil or pure auto (automatic type with no tool name) while retaining the empty-tools requirement and every existing prompt, history, provider-option, header, raw-output, and response-format check. Pass original options through unchanged. Non-auto choices and effectful requests still fail before any physical candidate executes.

The shared HTTP mapper and direct function-tool behavior are existing dependencies, not changes. Regression tests cover their choice preservation and unsupported-family boundaries because the core now sends auto on every otherwise unconfigured text call. No request stripping or normalization workaround is introduced.

### 3. Prove the high-level request boundary

Use the existing authenticated edge/real-command test composition with actual Go `StreamText` and pinned TypeScript `streamText`, equivalent prompts/options, and no tools or explicit choice. Passive inbound capture must show auto, backend execution, and expected text for each client without rewriting requests. Existing privacy assertions remain in force.

Build the narrow Go probe under `providers/grafana/internal/capture/testdata/streamtext/` with the repository `go.work`, and assert local core/client module selection. Ordinary client and Gateway builds remain isolated against their published dependencies. Reuse the existing exact TypeScript dependency and lockfile.

The cloud-authentication delta updates the stale high-level-streaming claim and adds this new evidence. Full unchanged context in retained `MODIFIED Requirements` is required for safe OpenSpec replacement; it does not claim implementation of those existing capabilities.

## Validation

| Boundary | Regression evidence added by this PR |
| --- | --- |
| Core | `streamtext_test.go`: absent/empty/filtered tools, explicit choices, per-step precedence/reset. `generatetext_test.go` and `agent_test.go`: shared defaults and overrides. |
| Gateway HTTP | `runtime_test.go`: both modes, absent versus explicit choices, absent/empty tools, exact forwarded values, correct invocation counts, and unchanged schema/unsupported-family failures. |
| Fallback | `fallback_route_test.go`: pure-auto acceptance, pre-commit failover preserving options, restart at primary, and zero physical calls for effectful variants. |
| Actual clients | `gateway-command.test.ts`: both high-level streaming defaults through the authenticated edge, plus omitted/auto choices across the existing low-level fallback matrix. |
| Existing contracts | Direct function-tool command round trips, pinned ProviderWire contracts, and provider/UI/object conformance snapshots remain passing. |

Run root and isolated Gateway tests, focused core/service race tests, command integration, `mise run parity-check`, vet/lint, strict OpenSpec validation, and whitespace checks. Existing provider inputs and expectations remain unchanged; focused test doubles are not provider recordings. No frontend wire behavior changes, so no new UI/SSE integration scenario is needed.

## Risks / Rollout

- Custom providers and middleware observe nonnil auto instead of nil on previously unconfigured text calls; explicit choices and public APIs do not change.
- The fallback exception must not admit effectful requests. Both direct guard tests and the command matrix retain their zero-candidate rejection evidence.
- For separately deployed SDK/Gateway releases using fallback routes, deploy the fallback guard fix first. Rolling back this PR's Gateway change restores no-tools-auto rejection only on fallback routes; current-main direct routes already accept it. Do not strip defaults in clients to compensate.
- The pinned Anthropic required/named-without-tools deviation and unrelated high-level unary gaps remain outside scope. Deferred matrix validation is recorded in `tasks.md`; focused evidence is not a claim about that unexecuted matrix.

## Open Questions

None for this PR's behavior or API.
