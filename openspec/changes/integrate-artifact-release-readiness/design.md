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
A release-please-based linked-commit attribution mechanism still needs implementation and
history fixtures. #263 owns the workspace image build; this change does not duplicate it.
