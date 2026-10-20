# Upstream Parity Coverage

This map lists the parity surfaces, their layer, how they are verified and where
the evidence lives. It tells a contributor which proof a change needs.

Update it only when a surface is added or removed, its status changes, or its
evidence moves. Adding tests to an existing surface does not require an edit.

Other records have their own homes:

- [upstream.yaml](upstream.yaml): the registered reference and every accepted
  deviation or support boundary (`knownGaps`).
- [`upstream-sync` issues](https://github.com/grafana/ai-sdk/issues?q=is%3Aissue+label%3Aupstream-sync):
  actionable differences and missing evidence.
- [README.md](README.md#evidence-limits): what each kind of evidence does and
  does not prove.
- `openspec/specs/`: per-capability behavior and support boundaries.
- The upgrade PR: run-specific assessment, validation and issue list.

## Status

- `automated`: the stated scope is checked by tests. It does not mean exhaustive parity.
- `mixed`: automated evidence covers only part of the surface.
- `manual`, `documented-deviation`, `gap`: as named.

## Coverage map

| Surface | Layer | Status | Evidence |
| --- | --- | --- | --- |
| Core orchestration and tools | Core | mixed | Root tests, `ui/` fixtures, `test/integration` |
| Structured output | Core | mixed | `expected-object.json` snapshots, `output/` tests |
| UI messages and SSE | Core | mixed | `ui/` chunk snapshots, framing tests, schema-parsed `test/integration` tests |
| Agent, middleware and registry | Core | mixed | Root tests, `test/integration` |
| React hooks | Frontend interop | mixed | `useChat`, `useCompletion` and `useObject` tests in `test/integration` |
| LanguageModelV4 contract | Provider contract | mixed | `mise run parity-provider-shape`, request snapshots |
| SDK provider-tool contract | Provider contract | mixed | Provider tests, Grafana client differential tests |
| SDK provider transport context | Provider contract | mixed | Anthropic/OpenAI HTTP tests, `mantle_transport_test.go` |
| ProviderWire request projection | Provider contract | automated | `ai-gateway/test/providerwire-v4` (`mise run test-providerwire-v4`) |
| Provider stream parts | Provider implementation | automated | `expected-parts.jsonl` for streaming `recorded/` and `upstream/` cases, `testdata/anthropic-*` |
| Provider adapters | Provider implementation | mixed | `<provider>/recorded/`, `<provider>/upstream/`, provider module tests |
| Bedrock Mantle Responses | Provider implementation | mixed | `providers/bedrock/mantle` request tests |
| Anthropic on Microsoft Foundry | Provider implementation | mixed | `providers/azure` HTTP tests |
| Buffered model fallback | Go extension | mixed | `fallback/` tests, `test/integration` |
| Fallback observation | Go extension | mixed | `fallback/` and root tests |
| Agent Observability integration | Go extension | mixed | `middleware/agentobservability` tests, Gateway export tests |
| AI Gateway | Go extension | mixed | `openspec/specs/gateway-*` and `grafana-gateway-client` specs, `ai-gateway/` tests |
| Baseline and fixture inventory | Conformance harness | automated | `mise run validate-parity-baseline`, `mise run parity-coverage` |
| Published module dependencies | Conformance harness | automated | `mise run verify-module-resolution` |
