# Proposal

## Why

Remove the current standalone Gateway image jobs to speed up CI while #428 migrates Gateway to App Platform. The obsolete image validation job delays completion of source CI after all seven library source jobs finish, blocking the existing read-only release authorizer's completed-workflow requirement.

## What Changes

- Remove Gateway image validation, its dependent image publisher and the image-only tag trigger from CI.
- Preserve all seven source job definitions and release authorization rules.
- Update workflow structural tests, CI guidance and the source-gate contract.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `candidate-source-ci`: Retire automatic image validation/publication while preserving candidate-source checks and separate release authorization.
- `module-validation-modes`: Remove the requirement to couple source checks to the retired image path.

## Impact

Only repository CI, its structural tests and related documentation/specifications change. Refs #245 and #468. No runtime, Dockerfile, local build-task, release-tooling, publication implementation, protection, tag or release changes. Historical run 39846721747 is unaffected. Current GitHub main is c8e497f3ca45571ef57c3556ebde32a836e8b918; the session checkout matches it despite an older Git-advertised main.

The registered upstream reference is eb77f09e3c06c28e860d92e0de941b143c2eecec (ai 7.0.127). This Grafana-specific CI contract has no corresponding upstream SDK behavior; no parity surface or baseline changes are required.
