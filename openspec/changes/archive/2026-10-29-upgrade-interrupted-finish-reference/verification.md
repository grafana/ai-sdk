# Verification

The exact package manifests match the frozen source reference in the design. Provider recordings, imported inputs, fixture configs and INDEX files are unchanged.

## Contract evidence

- Native UI, request and provider-part replay passes for the registered corpus. Two expectations changed: the imported OpenAI `shell` fixture now carries `maxOutputLength: 8912` in the tool input (3cb1dae), and the synthetic Anthropic `end-of-stream-after-delta` provider-part golden now ends with an `interrupted` finish with empty usage (43ad966, aac6026).
- Repeated provider generation and every committed UI generator reproduce identical expectations.
- Frontend integration passes 163 tests, including schema-parsed interrupted finish chunks and a `useChat` scenario that does not continue automatically after a late tool output. ProviderWire passes 140 tests including the upstream-type witness; Gateway command integration passes 94 tests.
- Focused tests cover Anthropic interrupted finish cases mirroring upstream, core tool-choice/continuation/execution gating, shell controls, and finish-reason acceptance in ProviderWire, the Grafana client, Gateway logging and middleware. The new core and Anthropic regressions failed before their corrections.
- Aggregate parity, all module tests, build, vet, lint, docs lint, examples, module requirements and affected race suites pass.

## Publication boundary

Candidate-source validation uses the repository workspaces; no Go module pins or published-module replacements change. Synthetic and imported replay evidence does not establish live-provider acceptance. Remaining assessed parity packages belong to the upgrade PR and issues, not this specification.
