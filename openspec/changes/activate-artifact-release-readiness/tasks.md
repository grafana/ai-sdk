# Tasks

This delivery implements section 1 only. Sections 2–6 remain follow-up work,
except the previously completed App authentication evidence in 5.1.

## 1. Candidate identity and library readiness

- [x] 1.1 Strengthen release identity and generated-file validation, including immutable App identity and manifest/body/version consistency; test spoofed metadata, ordinary PRs, multiple components and current-base/checkout mismatches. Gateway generated-file support remains in section 3.
- [x] 1.2 Complete owned-source snapshot selection, including embedded assets and nested module ownership; test stale behavior, docs-only changes, root middleware, deletion and rename cases.
- [x] 1.3 Retain selected-module public-proxy validation and add failures for replacement, unverifiable origin, unmerged commit, pseudo-version and proxy outage; verify root readiness never invokes unrelated downstream standalone builds.
- [x] 1.4 Add selected consumer regression harnesses using the candidate module and pinned prerequisites without workspace replacements; verify root frontend, provider and middleware behavior with dependency-origin assertions and credential-free tests.
- [x] 1.5 Update the library readiness runbook with ownership/exclusion rules and harness commands; run the underlying `release-check` commands and the new selected harness tests (avoid the tool-installing wrapper).

## 2. Guarded release-please publication

- [ ] 2.1 Add locked release-please 17.6.0 tooling and a shared preparation/preview driver; verify dry runs produce expected independent release candidates without writes.
- [ ] 2.2 Freeze pending candidates and authorize each merged revision before publication; test delayed runs, edited PR metadata, candidate injection during validation and independent ready/unready candidates.
- [ ] 2.3 Validate merged-tree equivalence, ancestry, expected source-check provenance and artifact readiness in isolated checkouts; test stale, skipped, cancelled, failed and missing evidence.
- [ ] 2.4 Pass only approved candidates into release-please publication with serialized execution and immutable-tag recovery; mock GitHub to test proxy failures, duplicate runs, tag-created/release-failed retries and conflicting tags without real publication.
- [ ] 2.5 Document candidate selection, bootstrap inventory and recovery; verify workflow lint and retain inactive guards until activation approval.

## 3. Gateway plugin integration and release intent

- [ ] 3.1 Integrate the merged #428 revision and reconcile its package version configuration, CI identities and affected specs; verify no image-era or standalone-Gateway publication assumptions remain in current guidance.
- [ ] 3.2 Add the pinned release-please attribution plugin using Gateway's own release boundary and tested linked-module ownership; verify SDK-only fixes, root middleware, unrelated libraries, deduplication, breaking commits, non-releasing commits, bootstrap and dependency-only updates.
- [ ] 3.3 Wire Gateway readiness to same-revision frontend, application, two-architecture plugin, licensing/validator, Grafana E2E and test host results; verify any missing or failed result blocks authorization.
- [ ] 3.4 Preserve tag-selected workspace builds and package source/version evidence for #459; test older merged pins and failing standalone Gateway compilation do not block a valid plugin package.
- [ ] 3.5 Update release inventory, contributor guidance, agent instructions and current specs to describe plugin distribution; run docs lint, strict OpenSpec validation and plugin workflow regression tests.

## 4. Dependency progression and bootstrap

- [ ] 4.1 Prepare narrow Renovate rules for independent first-party tagged library updates, alpha versions, prompt scheduling and `fix(deps)` intent while preserving signed App commits; validate effective config against representative root, provider, Bedrock and Gateway dependencies.
- [ ] 4.2 Exercise root release → OpenAI repin/release → Bedrock repin/readiness with deterministic local Git/proxy fixtures; verify no global standalone deadlock and no dependency on future tags in a release PR.
- [ ] 4.3 Test the retained adoption boundary, first tags and dependency-only release proposals; document when temporary `last-release-sha` can be removed and verify root middleware attribution remains covered.

## 5. Reviewed activation

- [x] 5.1 Verify broker App configuration through non-secret evidence and record the remaining maintainer setup steps; demonstrate token issuance only in the approved workflow/branch/event context without printing credentials.
- [ ] 5.2 Deliver the exact required-check/current-base/squash-policy migration and historical-candidate inventory for approval; verify existing source gates are preserved and no broad standalone requirement is added.
- [ ] 5.3 Obtain explicit maintainer approval for protection changes and activation; record approved settings and verify them read-only after the maintainer applies them.
- [ ] 5.4 Enable the existing workflow and Renovate progression only after prerequisite approval; verify App-generated signed-compatible release PRs trigger required checks and publication uses the approved merged revision.

## 6. Integration verification

- [ ] 6.1 Run release configuration/readiness/driver tests, workflow lint, docs lint and `openspec validate activate-artifact-release-readiness --strict`; retain results for review.
- [ ] 6.2 Run candidate-source CI and Gateway plugin checks at the final branch revision; verify all #245 acceptance scenarios have evidence or an explicitly approved external prerequisite, with no claim of package publication under #459.

## Workflow follow-up

- Archive after implementation, verification and activation disposition are complete.
- Coordinate package publication with #459 and deployment with #104.
