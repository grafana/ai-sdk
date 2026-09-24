## Why

Coordinated SDK, provider, middleware, and Gateway changes currently require unmerged dependency pins to satisfy standalone checks, even though the modules are intended to remain independently publishable. Issue [#244](https://github.com/grafana/ai-sdk/issues/244) establishes safe source-integration and merged-pin validation primitives before [#245](https://github.com/grafana/ai-sdk/issues/245) changes any merge or publication gates.

## What Changes

- Add a fail-closed check that every real internal dependency selected for a published module resolves to a commit already merged into an explicitly fetched canonical `grafana/ai-sdk` `main`, including direct requirements and the selected graph. Migrate existing unmerged pins only after reviewing old/new provenance and standalone behavior.
- Add an explicitly selected Gateway source-integration workspace alongside the SDK-only root `go.work`; test that it uses candidate source, while production and release builds remain `GOWORK=off`.
- Separate structural license/import/module boundary checks from standalone build/test execution, and support selecting a published module for standalone validation while retaining the existing all-module command and required checks.
- Update developer guidance and the Gateway boundary contract to allow the explicit integration workspace without allowing SDK-to-Gateway dependencies or a Gateway entry in root `go.work`.
- Do **not** relax source PR gates, change Gateway publication/deployment authorization, or introduce release/version automation. Those belong to #245/#21.

## Capabilities

### New Capabilities

- `merged-internal-pins`: Verify real published-module internal requirements against canonical merged history and safely migrate existing branch-only pins.
- `module-validation-modes`: Offer explicit Gateway candidate-source integration and module-scoped standalone validation without changing current all-module enforcement.

### Modified Capabilities

- `providerwire-v4-http-contract`: Clarify that an explicitly selected Gateway integration workspace is permitted while the root workspace, reverse dependency boundary, license separation, and standalone SDK validation remain mandatory.

## Impact

Build tooling in `scripts/`, `mise.toml`, a separate integration workspace (without modifying root `go.work`), Go module requirements, Gateway/SDK integration test entry points, `.github/workflows/ci.yml`, `AGENTS.md`, `CONTRIBUTING.md`, and the listed OpenSpec contracts. No upstream AI SDK behavioral or wire-format change; the registered `test/conformance/upstream.yaml` baseline remains unchanged.
