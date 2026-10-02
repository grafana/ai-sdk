## 1. Anthropic and Vertex

- [x] 1.1 Bump `anthropic-sdk-go` to v1.76.0 in every consumer module and verify `mise run build` succeeds
- [x] 1.2 Add `claude-sonnet-5-5` to the direct and undated Vertex model IDs and its capability row before `claude-sonnet-5`; verify `TestSonnet55ModelIDs` and `TestSonnet55Capabilities`
- [x] 1.3 Add `ThinkingBetweenTools`, map reasoning `none` to it, and normalize explicit disabled and budget thinking with the 4.0.67 warnings; verify `TestSonnet55ReasoningNoneUsesBetweenTools` and `TestSonnet55ExplicitThinkingIsNormalized`
- [x] 1.4 Lower `between_tools` effort above `high` and treat `between_tools` as active thinking for sampling removal; verify the effort cases in `TestSonnet55ExplicitThinkingIsNormalized` and `TestSonnet55DropsSamplingParameters`
- [x] 1.5 Send forced tool choices as `auto`, only after the JSON response tool decision; verify `TestSonnet55ForcedToolChoiceFallsBackToAuto` and the `required`/`tool` subtests of `TestSonnet55JSONToolWithoutNativeOutputUsesAuto`
- [x] 1.6 Fall back from `jsonTool` to native output with the model ID in the warning, and use `auto` for the JSON response tool; verify `TestSonnet55JSONToolModeUsesOutputFormat`
- [x] 1.7 Document the Sonnet 5.5 rewrites in `docs/providers/anthropic.md` and verify `mise run lint-docs`

## 2. Bedrock

- [x] 2.1 Add `anthropic.claude-sonnet-5-5` to the known model IDs; verify `TestSonnet55ModelID`
- [x] 2.2 Send reasoning `none` as `between_tools`, accept an explicit `between_tools` type with the effort limit, and record the deviation in `test/conformance/upstream.yaml`; verify `TestSonnet55ReasoningNoneUsesBetweenTools`, `TestSonnet55BetweenToolsEffortIsCapped` and `mise run validate-parity-baseline`
- [x] 2.3 Send forced tool choices as `auto` and route JSON responses to the instruction; verify `TestSonnet55ForcedToolChoiceFallsBackToAuto` and `TestSonnet55JSONResponseUsesInstructionWithoutTools`
- [x] 2.4 Document the Sonnet 5.5 behavior in `docs/providers/bedrock.md` and verify `mise run lint-docs`

## 3. Integration and evidence

- [x] 3.1 Run the provider module tests, `mise run lint` and `mise run parity-check`, and confirm no recorded or upstream fixture input changed
- [x] 3.2 Link #275 for the Opus 5.5 and Fable 5.1 items, and register the Bedrock sampling removal (#279) and the missing upstream-backed evidence for this model (#278) as `upstream-sync` issues linked from the PR
