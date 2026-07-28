## Why

The SDK depends on behavioral and wire-format parity with Vercel's TypeScript AI SDK, but the verified upstream baseline and parity workflow are currently implicit in conformance tool dependencies and agent guidance. Making the baseline, coverage, checks, and agent workflows explicit will reduce drift and make upstream upgrades reviewable.

## What Changes

- Add a first-class upstream parity baseline manifest that records the verified upstream repo/package versions, verification status, and known intentional deviations.
- Add a layered parity coverage map that explains which core, provider contract, provider implementation, frontend interop, and conformance harness surfaces are covered by automated conformance, docs, manual review, or known gaps.
- Add parity-focused commands for checking the current baseline and upgrading to a new upstream baseline.
- Update repository agent guidance so parity-sensitive changes require upstream comparison, conformance-first TDD when fixtures can express the behavior, and explicit divergence classification.
- Add Codex skills for scoped parity review and parity upgrade work.
- Wire the existing conformance suite into the parity baseline so generated TypeScript expectations, request snapshots, and CI signal are traceable to a declared upstream target.

## Capabilities

### New Capabilities
- `upstream-parity-governance`: Defines the repository standards for declaring, checking, upgrading, and reviewing upstream AI SDK parity.

### Modified Capabilities
- `conformance-testing`: Extends the existing conformance harness requirements so fixture generation and checks are tied to the registered upstream parity baseline.

## Impact

- Affected docs and guidance: `AGENTS.md`, conformance documentation, and new parity governance documentation.
- Affected tooling: `mise.toml`, `test/conformance/tools/package.json`, `test/pnpm-lock.yaml`, and lightweight validation scripts for baseline consistency.
- Affected Codex workflows: new repo-local skills under `.codex/skills/` for scoped parity review and parity upgrade.
- Affected CI: conformance remains available as a parity signal, with a path to promote a stable subset to a blocking gate.
