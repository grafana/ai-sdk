# Proposal

## Why

Library releases never converge. Each module gets its own release pull request,
readiness requires every first-party prerequisite to be tagged at current main
source, and Renovate carries new tags into dependent `go.mod` files in separate,
review-gated pull requests held by the organization's forced three-day release age.
One logical release becomes a serial root -> provider -> Bedrock/Azure chain of
release, wait, bump and re-approve steps, and any merge that touches prerequisite
source during the chain makes the dependents stale again. The first rollout under
#245 never reached Bedrock or Azure, and the same chain repeats on every root change.

## What Changes

- **BREAKING (release process)**: release-please proposes one combined release pull
  request for all pending library modules, each keeping its own version, changelog and
  `<module>/vX.Y.Z` tag. Gateway keeps its own release pull request.
- Release preparation adds the first-party requirement updates to that same pull
  request: released modules and Gateway require the versions released alongside them,
  with `go.sum` entries computed from the exact candidate tree.
- All library tags of one release are created at the same merge commit, so every
  co-released prerequisite is fresh by construction.
- Readiness, authorization and the merged-pin check verify co-released prerequisites
  against a candidate module proxy built from the candidate tree, byte-identical to
  what the public proxy serves after tagging. Already-tagged prerequisites keep the
  existing public-proxy and canonical-tag checks.
- Publication is dependency-closed: a library is published only if every co-released
  prerequisite it requires is published in the same batch. Each tag is then checked
  on the public proxy against the hash recorded in `go.sum`.
- A non-published workspace module resolves unpublished first-party versions to
  in-repository source for root and Gateway workspaces, without workspace `replace`
  directives.
- **BREAKING (dependency automation)**: Renovate stops updating first-party modules;
  the tagged-prerequisite rule is removed.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `multi-module-release-management`: release pull request shape, readiness and
  prerequisite identity for co-released modules, Gateway pin updates, merged-candidate
  inventory, identity and authorization decisions, and dependency-closed publication.
- `module-validation-modes`: workspace resolution of unpublished co-released versions,
  merged-pin and consumer validation of co-released prerequisites on release pull
  requests.

## Impact

- Release tooling: `release/tooling` preparation, merged-candidate discovery,
  authorization and publication; `scripts/release_readiness.py`,
  `scripts/release_consumer.py` and `scripts/module-policy.sh`.
- Configuration: `release-please-config.json`, `renovate.json`, `go.work`,
  `go.gateway.work` and a new non-published `test/workspace` module.
- Workflows: `release-please.yml` needs no new permissions. Release preparation
  runs Go, which the workflow already installs.
- Docs: `release/README.md`, `release/tooling/ACTIVATION.md`, `CONTRIBUTING.md`.
- Open per-module release pull requests are superseded by the combined pull request.
  Existing tags, releases and manifest versions are unchanged.
- Gateway plugin publication (#459) is unaffected: it consumes the separate Gateway
  release and builds from the workspace.
