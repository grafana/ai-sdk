## Why

AI Gateway is developing faster than its current text-only contract tests can describe end-to-end SDK compatibility. Running the entire existing provider conformance corpus through both gateway clients will make missing providers, unsupported features, and behavioral differences an executable backlog that can be rebased or merged while still failing.

## What Changes

- Preserve the existing required direct conformance suite and its upstream-generated expectations.
- Add full provider-fixture execution through `@ai-sdk/gateway` and `providers/grafana`, each targeting the actual AI Gateway container image backed by deterministic provider replay servers.
- Discover every provider scenario for both clients, including streaming, multi-step, and existing unary cases. Do not filter by gateway capabilities, skip unsupported cases, or convert failures into expected passes.
- Compare UI output, backend provider requests, and applicable object, usage, and unary expectations against the existing direct upstream reference. Keep inputs and goldens unchanged.
- Report every fixture/client pair, with isolated execution, bounded timeouts, actionable diffs, and explicit distinctions between compatibility failures, provider setup failures, and harness failures.
- Add an independent gateway conformance CI job that remains visibly unsuccessful on failures but is initially not required for merging. Keep it outside image publication/deployment prerequisites during this advisory period.
- Keep provider-independent `ui/` mock-model fixtures in the existing direct suite; they are not provider replay scenarios and will not gain a production gateway mock adapter.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `conformance-testing`: Extend the existing provider fixture corpus into a two-client gateway execution matrix, including container lifecycle, reference comparisons, complete failure reporting, and advisory CI policy.

## Impact

- Shared Go and TypeScript conformance execution, fixture discovery, replay, and comparison helpers under `test/conformance/`.
- Gateway test orchestration, existing registered-client test helpers, `providers/grafana` test integration, and exact-baseline dependency validation.
- `mise.toml`, `.github/workflows/ci.yml`, conformance documentation, and `test/conformance/PARITY.md`.
- Repository required-check settings must leave the new check optional; no unrelated required check is weakened.
- The gateway remains a separately built AGPL service with pinned dependencies. The reusable SDK and harness must not import its implementation.
- No gateway feature implementation, provider expansion, upstream baseline upgrade, fixture recording, or snapshot regeneration is included. A complete, trustworthy red matrix is a successful delivery of this change.
