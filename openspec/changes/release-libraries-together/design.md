# Design

## Context

Release-please supports one pull request for several packages
(`separate-pull-requests: false`); a package with `separate-pull-requests: true`
keeps its own pull request. On merge it still produces one tag and GitHub Release
per package. The repository's own readiness, authorization and publication
tooling currently assumes one component per release pull request.

## Goals / Non-Goals

**Goals:** release all pending libraries with one review and one merge, without
new release tooling.

**Non-Goals:** updating first-party requirements inside the release pull request,
Gateway publication (#459), and changing Renovate's hold.

## Decisions

- **Drop prerequisite freshness instead of co-releasing pins.** Bumping pins in
  the release pull request requires `go.sum` entries for untagged versions, which
  means rebuilding proxy-identical module archives and resolving prerequisites
  from them in readiness and authorization. Dropping freshness keeps all
  prerequisites as published tags and needs no new tooling. Standalone and
  consumer checks still prove each library works with its declared minimums, and
  workspace CI proves it with the source released alongside it.
- **One readiness record per pull request.** The evidence record lists every
  released component and version (schema 2). Authorization still decides and
  reports each component separately.
- **No publication ordering.** Released libraries require only existing tags, so
  their tags can be created in any order.
- **Labels after the whole pull request.** Release-please discovers merged
  release pull requests by the `autorelease: pending` label. Removing it after the
  first component would hide the remaining ones from a retry.
- **Escape hatch by configuration.** A library that cannot pass readiness is
  marked `separate-pull-requests: true` rather than handled by new tooling. The
  existing single-component readiness path validates its own pull request.

## Risks / Trade-offs

- **Lagging pins.** Released `go.mod` files declare older minimums; consumers
  that need newer behavior require the newer prerequisite themselves. This is
  normal Go minimum version selection.
- **One blocked library blocks the release.** It needs the configuration escape
  hatch and a second release round.

## Migration Plan

Merge, then close open per-component library release pull requests; the next
release run opens the combined pull request. Libraries with pseudo-version
prerequisites (currently Azure) need a tagged pin first or the escape hatch.
Rollback restores `separate-pull-requests: true`.
