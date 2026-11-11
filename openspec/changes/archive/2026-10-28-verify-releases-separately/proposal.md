# Proposal

## Why

The first combined library release published all ten libraries, but two
post-publication checks failed. The public Go proxy kept answering "unknown
revision" for `providers/openai@v0.1.0-alpha.2` for about 26 minutes after the tag
existed: a release pull request mentions not-yet-tagged versions while it is open,
and any lookup in that window caches a miss for about 30 minutes. The in-job retry
covered five minutes and did not recognize Go's fallback error ("does not contain
package"). Separately, every release run ends red when `main` advances during its
CI wait, although it correctly writes nothing and the next run prepares the release
pull request.

## What Changes

- A separate release verification workflow runs for each published library GitHub
  Release and on manual dispatch for a tag. It waits up to 40 minutes until the
  version and its first-party requirements resolve through the public proxy, then
  runs the consumer check once. The release workflow no longer verifies published
  versions, so its concurrency group is not held by proxy waits.
- When `main` advanced during a release run, preparation is skipped with a notice
  instead of failing the run. Blocked publication still fails it.

## Capabilities

### New Capabilities

### Modified Capabilities

- `multi-module-release-management`: post-publication verification runs in its
  own workflow with a bounded availability wait; a release run whose base advanced
  skips preparation successfully.

## Impact

`.github/workflows/release-verification.yml`, `scripts/release_consumer.py`,
`release/tooling/publish.mjs`, `publication.mjs`, `authorize.mjs`, release docs.
