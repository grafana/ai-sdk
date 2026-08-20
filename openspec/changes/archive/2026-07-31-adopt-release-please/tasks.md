## 1. Release Configuration

- [x] 1.1 Register every published module with its component and tag shape
- [x] 1.2 Bootstrap the version manifest from the released root tag
- [x] 1.3 Configure the alpha prerelease channel and changelog sections
- [x] 1.4 Exclude nested module paths from the root module

## 2. Publication

- [x] 2.1 Add the release workflow that grooms the release pull request and publishes tags
- [x] 2.2 Delegate the post-release core requirement bump to Renovate
- [x] 2.3 Document the GitHub App credentials the release workflow requires

## 3. Validation

- [x] 3.1 Add `internal/releasecheck` for registry, tag shape, and publishability invariants
- [x] 3.2 Wire `mise run release-check` into the existing CI build job
- [x] 3.3 Add the required Conventional Commit check for pull requests
- [x] 3.4 Add a read-only `mise run release-preview` dry run

## 4. Documentation and Agents

- [x] 4.1 Add the maintainer release runbook
- [x] 4.2 Document release intent in CONTRIBUTING and the pull request template
- [x] 4.3 Create the repository-local `release-ai-sdk` skill and agent metadata

## 5. Verification

- [x] 5.1 Run the release configuration tests
- [x] 5.2 Verify tag and version calculations against the pinned release-please implementation
- [x] 5.3 Run repository formatting, vet, lint, docs lint, and module tests
