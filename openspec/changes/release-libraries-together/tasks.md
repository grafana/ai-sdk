# Tasks

## 1. Readiness and authorization

- [x] 1.1 Validate combined library release metadata, footprint, changelogs and every released library in `scripts/release_readiness.py`
- [x] 1.2 Remove the prerequisite freshness check and source ownership snapshots
- [x] 1.3 Write and accept readiness record schema 2 with all released components in `scripts/release_authorization.py`

## 2. Release tooling

- [x] 2.1 Prepare one combined library release pull request in `release/tooling/activation.mjs`
- [x] 2.2 Move lifecycle labels once per pull request after all its libraries publish in `release/tooling/publication.mjs`
- [ ] 2.3 Keep the root bootstrap exercise on its single-root shape

## 3. Configuration and documentation

- [ ] 3.1 Release libraries from one pull request in `release-please-config.json`, with Gateway separate, and check it in `internal/releasecheck`
- [ ] 3.2 Update `release/README.md`, `release/tooling` docs and `CONTRIBUTING.md`, including the escape hatch

## 4. Validation

- [ ] 4.1 Run release tooling, readiness, authorization and release configuration tests
- [ ] 4.2 Preview the combined release pull request against current `main`
