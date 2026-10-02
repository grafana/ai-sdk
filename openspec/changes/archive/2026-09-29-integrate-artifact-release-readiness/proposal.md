# Integrate artifact-specific release readiness

## Why

Source CI now validates candidate workspaces and merged-only pins. The older release
adoption draft assumes global standalone checks, excludes root-owned middleware, and
classifies Gateway as a Go library. These assumptions cannot authorize publication.

## What changes

- Prepare a dedicated release-identity and selected-library readiness check.
- Preserve root-owned middleware release intent and match preview to action 17.6.0.
- Classify Gateway as an application and keep all release automation inactive.
- Record Gateway linked workspace attribution and #263 image readiness as activation
  requirements owned by #245, outside this inactive scaffold.

## Impact

Release configuration, workflows, validation scripts, agent guidance and runbook.
No SDK runtime or registered upstream behavior changes.
