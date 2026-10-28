# Proposal

## Why

Library releases do not converge. A library may only require published tags of
other first-party modules, so a pull request that changes root and a provider
together, such as #531, needs three steps: release root, merge a requirement
update, then release the provider. Each requirement update waits for the
self-hosted Renovate's forced three-day hold and a fresh review, and further
prerequisite changes in the meantime restart the chain. On current `main`, four
of the nine nested libraries cannot build against any published root version.

The release process should meet four goals:

1. Each Go module keeps an independent version and releases only when it changed.
2. Pseudo-versions are not supported.
3. A release does not wait for a minimum release age.
4. Releasing needs no paused merges, manual steps or dependency dashboard actions.

## What Changes

- Every Go module's `go.mod` replaces each first-party module in its build graph
  with that module's repository directory. Go applies `replace` directives only
  in the main module, so consumers still resolve published tags, while every
  in-repository build uses same-revision source and needs no first-party
  `go.sum` entries. AWS SDK Go v2 and OpenTelemetry Go use the same model.
- **BREAKING (repository):** `go.work`, `go.gateway.work` and their sum files are
  removed. Tasks and CI run per module with `GOWORK=off`; the Gateway plugin
  backend builds from `ai-gateway/go.mod`.
- First-party requirements always equal the version manifest, or the registered
  initial version of a never-released module. Pseudo-versions are rejected. This
  replaces the merged-pin ancestry check.
- release-please proposes all pending libraries in one release pull request;
  Gateway keeps its own. Release preparation sets every first-party requirement
  to the new manifest versions in the same pull request, and merging it tags
  every released library at the merge commit.
- Release readiness checks release metadata and an exact generated footprint:
  the manifest, released changelogs and first-party requirement lines. Library
  behavior is proven by the per-module CI of that pull request. Resolving
  prerequisites through the public proxy and prerequisite freshness are removed.
- After tagging, publication verifies each new version as an external consumer
  through the public Go proxy. A dependent is not tagged when a co-released
  prerequisite is blocked, and lifecycle labels move once all libraries of the
  pull request are published.
- `refactor` commits release, so refactors that change library source reach the
  next release.
- Renovate no longer updates first-party modules.

## Capabilities

### New Capabilities

### Modified Capabilities

- `multi-module-release-management`: combined library release pull request,
  requirement updates in release preparation, footprint-based readiness,
  post-publication proxy verification, dependency-closed publication, no
  Renovate progression, local replacements allowed.
- `module-validation-modes`: per-module validation with local replacements
  instead of workspaces; Gateway builds from its module; no prerequisite
  freshness or public-proxy standalone readiness.
- `merged-internal-pins`: replaced by the rule that first-party requirements
  equal the version manifest.
- `candidate-source-ci`: plugin validation builds the Gateway module without a
  workspace.
- `providerwire-v4-http-contract`: the Gateway boundary no longer refers to
  workspaces; the separate integration workspace requirement is removed.
- `gateway-sources`: evidence wording refers to same-revision source instead of
  a candidate workspace.
- `sdk-provider-tools`, `gateway-provider-tools`, `bedrock-mantle-responses-provider`:
  pin, workspace and replacement wording follows the manifest-aligned model.
- `upstream-parity-governance`: coordinated producer and consumer changes rely on
  local replacements; release evidence is per-module CI plus post-publication
  verification.

## Impact

- All `go.mod` and `go.sum` files, `go.work*`, `go.gateway.work*`, `mise.toml`,
  CI workflows, `scripts/module-policy.sh`, release readiness and authorization,
  `release/tooling` preparation and publication, `release-please-config.json`,
  `renovate.json` and release documentation.
- The first release after merge proposes all ten pending libraries in one pull
  request. Open per-module release pull requests and the grouped first-party
  Renovate pull request are closed.
- Not in scope: replacing the custom release tooling with the stock
  release-please action, and Gateway plugin publication (#459).
