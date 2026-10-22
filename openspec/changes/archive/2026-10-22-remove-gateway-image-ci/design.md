# Design

## Context

See proposal.md for motivation. The workflow contains seven source jobs plus image-validation and publish-ai-gateway-image. The publisher needs both image validation and all source jobs. No remaining job consumes its output after deployment removal in #486.

## Goals / Non-Goals

Remove obsolete CI image automation while keeping source jobs byte-for-byte unchanged. Preserve local image tooling, runtime, release-please configuration and read-only authorization behavior. Replacement application release work remains separate.

## Decisions

Delete both image jobs rather than leave an invalid needs edge or publish without validation. Remove the Gateway tag push trigger because it exists only for image automation. Keep tag-aware cache expressions in source jobs unchanged to avoid unrelated cleanup.

Replace the removed image assertions with checks for exactly seven source jobs, PR/main-only triggers, no image automation or registry-write permission, valid dependencies and blocking source checks. Keep local Dockerfile/workspace assertions as a separate test.

Update only the owning source-CI and source-gate specs. Existing conditional container recipe/evidence contracts do not create an eligible publication path; App Platform builds and packaging belong to #428, and future publication remains separate under #245.

## Risks / Trade-offs

Historical workflows continue using their original definition; this change affects future runs only. The current image publisher is removed with its prerequisite; this does not decide the future App Platform build or publication design. No replacement publication or authorization shortcut is introduced.

## Migration Plan

Merge the reviewed CI-only PR to affect subsequent runs. Reverting the commit restores the old workflow. No tags, releases, repository settings or historical runs are modified.
