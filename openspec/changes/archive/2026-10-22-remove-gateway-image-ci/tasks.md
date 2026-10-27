# Tasks

## 1. Retire image CI

- [x] 1.1 Remove both image jobs and the image-only tag trigger; update workflow regression tests and affected contributor/container guidance while preserving all seven source jobs and local Docker tooling.

## 2. Validate the scoped change

- [x] 2.1 Run workflow tests, TypeScript checks, workflow lint, docs checks and relevant release-tooling tests; compare all seven source jobs with current main and verify release authorization code is unchanged.

## Workflow follow-up

- Archive and sync the OpenSpec change after validation and review.
- Open a separate draft PR with a non-releasing ci title, Refs #245 and #468, and empty closingIssuesReferences.
