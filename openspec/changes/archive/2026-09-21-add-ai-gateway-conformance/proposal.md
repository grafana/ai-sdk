## Why

AI Gateway is developing faster than its current contract tests can describe end-to-end SDK compatibility. Running the entire existing provider conformance corpus through both gateway clients makes missing providers, unsupported features, and behavioral differences an executable backlog that blocks merging until it is resolved.

## What Changes

- Preserve the existing required direct conformance suite and its upstream-generated expectations.
- Add full provider-fixture execution through `@ai-sdk/gateway` and `providers/grafana`, each calling the AI Gateway plugin through one Grafana instance backed by deterministic provider replay servers.
- Discover every provider scenario for both clients, including streaming, multi-step, and existing unary cases. Do not filter by gateway capabilities, skip unsupported cases, or convert failures into expected passes.
- Compare UI output, backend provider requests, provider stream parts, and applicable object, usage, and unary expectations against the existing direct upstream reference. Keep inputs and goldens unchanged.
- Report every fixture/client pair, with isolated execution, bounded timeouts, actionable diffs, and explicit distinctions between compatibility failures, provider setup failures, and harness failures.
- Add an independent gateway conformance CI job and require it, together with direct conformance, through the existing `conformance-test` status.
- Keep provider-independent `ui/` mock-model fixtures in the existing direct suite; they are not provider replay scenarios.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `conformance-testing`: Extend the existing provider fixture corpus into a two-client gateway execution matrix, including Grafana stack lifecycle, reference comparisons, complete failure reporting, and required CI policy.

## Impact

- Shared Go and TypeScript conformance execution, fixture discovery, replay, and comparison helpers under `test/conformance/`.
- Gateway test orchestration, the plugin package build, `providers/grafana` test integration, and exact-baseline dependency validation.
- `mise.toml`, `.github/workflows/ci.yml`, conformance documentation, and `test/conformance/PARITY.md`.
- The gateway remains a separately built AGPL plugin. The reusable SDK and harness must not import its implementation.
- No gateway feature implementation, provider expansion, upstream baseline upgrade, fixture recording, or snapshot regeneration is included. The matrix blocks merging while it is red.
