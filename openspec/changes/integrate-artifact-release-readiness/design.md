# Design

Release-please remains the version/changelog/tag engine. The pinned action and local
preview both use release-please 17.6.0. The action prepares release PRs while
`skip-github-release: true` prevents creating tags or releases during integration.

A manifest version change must identify exactly one configured component and originate
from the dedicated release App in the canonical repository. Readiness requires the
exact merge of current canonical main and the live release head, and permits only the
selected generated changelog and manifest. Labels do not authorize release validation.

Library readiness validates the selected dependency closure with a clean public-proxy
cache and requires tagged, canonical merged prerequisites. It compares prerequisite
owned-source blob snapshots to the base, using registered nested module boundaries and
explicit tooling/test/documentation exclusions. Existing selected-module standalone
validation then builds/tests only the candidate library. Root does not wait for unrelated
providers. Gateway intentionally fails this gate pending its application/image path.

The new check cannot alone make old green results stale after main advances: reviewed
repository protections must require a current base or equivalent merge-queue evidence.
These changes are preparation and do not claim publication authorization.

The pinned built-in linked-versions plugin forces shared versions, conflicting with
independent Gateway application versions. It must not be used as an attribution shortcut.
Release-please 17.6.0 splits commits by touched package paths before each component's
strategy computes its version. Neither a conventional scope nor `Release-As` can restore
a provider-only commit once that split has excluded it from `ai-gateway`. A second
manifest rooted at `.` would see linked changes, but its denylist cannot express the
exact source set built into the image and would create separate release state. We will
instead evaluate a small wrapper around the pinned release-please API that registers a
preconfigure plugin. It must add Gateway-relevant commits to `commitsByPath["ai-gateway"]`
before the normal Gateway strategy calculates its independent version and changelog.

The plugin must read history since the Gateway's own last release, not union other
components' already-truncated commit lists. It must use an explicit allowlist of
workspace source/runtime paths included in the image, preserve commit metadata needed
for release notes, and deduplicate by SHA. It must ignore docs/examples-only commits.
The stock action cannot load externally registered plugins, so replacing its invocation
requires an exact-engine wrapper, bot-token handling, serialization and release-PR/tag
parity tests. This is a design direction, not an implemented release mechanism.

#263's workspace Docker build is present in the merged source baseline. Gateway
versioned-image readiness must still prove that an `ai-gateway/vX.Y.Z` tag selects the
same repository revision and image recipe, and that image validation/smoke and license
evidence gate publication. A new Gateway release intent without that proof is unsafe.
