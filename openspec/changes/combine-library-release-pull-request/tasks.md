# Tasks

## 1. Workspace resolution module

- [x] 1.1 Add `test/workspace/go.mod` replacing every published first-party library with its directory, include it in `go.work` and `go.gateway.work`, and verify `mise run build`, `mise run verify-gateway-workspace` and `mise run verify-sdk-gateway-isolation` pass
- [x] 1.2 Extend `module-policy.sh` `boundary`/`workspace` to require the module to list exactly the published libraries with in-repository targets and to keep rejecting workspace `replace` directives; add fixtures to `test/module-policy.sh` for a missing library, an out-of-repository target and a workspace `replace`, and verify `mise run test-module-policy` passes
- [x] 1.3 Add a module-policy fixture where libraries and Gateway require an untagged first-party version and verify root workspace builds and the Gateway plugin backend build succeed with `GOPROXY=off`
- [x] 1.4 Update the module-validation section of `CONTRIBUTING.md` for the workspace module and verify `mise run lint-docs` passes

## 2. Candidate requirement generator

- [x] 2.1 Create the local-only `release/candidate` Go module, classify it as local-only in module inventory and publishable-module validation, and verify `mise run release-check` and `mise run test-module-policy` pass
- [x] 2.2 Implement archive building with Go command semantics (nested-module exclusion, root `LICENSE` for nested modules lacking one) and verify a test reproduces the published `h1:` and `go.mod` hashes for root `v0.1.0-alpha.3` and `providers/openai/v0.1.0-alpha.1` from their tags
- [x] 2.3 Implement requirement rewriting per design D2 (released modules and Gateway only; co-released version, otherwise latest tag) and verify table tests cover root+Bedrock without OpenAI, Gateway pins and unchanged non-released manifests
- [x] 2.4 Implement level-ordered `go.sum` generation with a file proxy, `GONOSUMDB` for the repository and an isolated module cache, and verify a fixture release produces checksums equal to archives rebuilt from the output tree, including a test that a pre-populated cache is not used
- [ ] 2.5 Add a check mode that regenerates on a given tree and fails on any byte difference, and verify tests cover a hand-edited pin and an unexpected third-party change

## 3. Release preparation

- [ ] 3.1 Set `separate-pull-requests: false` with a per-package `true` for `ai-gateway` only in test fixtures first, and update `prepareLibraries` to build one combined candidate plus the Gateway candidate; verify `activation.test.mjs` covers both shapes and Gateway stays excluded from library writes
- [ ] 3.2 Materialize the candidate tree (main plus release-please updates), run the generator, append its files to the combined candidate before `createOrUpdatePullRequest`, and verify a test asserts one App-created commit containing manifest, changelogs and generated requirement files, with the preparation transport still refusing other writes
- [ ] 3.3 Update `preview.mjs` to show the generated requirement files and verify `preview.test.mjs` covers a combined preview

## 4. Readiness and source checks

- [ ] 4.1 Update `release_readiness.py` to accept several released components, derive the allowed footprint (manifest, released changelogs, released modules' and Gateway's requirement files) and run the generator check mode; verify `scripts/test_release_readiness.py` covers multi-component success, a requirement change outside the release and a missing manifest update
- [ ] 4.2 Resolve co-released prerequisites from candidate archives of the PR-head-plus-base tree, keep public-proxy and tag checks for tagged prerequisites, and treat co-released prerequisites as fresh; verify tests cover co-released success, archive mismatch and a stale non-released prerequisite
- [ ] 4.3 Add a candidate mode to `module-policy.sh standalone` used only by readiness and consumer checks in `release_consumer.py`, leaving the on-demand public-proxy commands unchanged; verify module-policy fixtures cover both modes
- [ ] 4.4 Update `merged_pins` to accept an untagged first-party version only when it equals the candidate manifest version and its candidate archive matches the recorded checksum; verify fixtures cover a release PR pin, an ordinary PR with an untagged pin and a manifest-matching pin with a wrong checksum

## 5. Authorization and publication

- [ ] 5.1 Update `merged-candidates.mjs` to yield one record per released component of a combined merge and key ambiguity by component and tag; verify `authorization.test.mjs` covers a combined merge and a duplicate component
- [ ] 5.2 Update authorization to validate each library against published prerequisites and co-released archives built from the merge revision, and to block candidates whose co-released prerequisites are blocked; verify tests cover the OpenAI-blocked/Bedrock-blocked case and an unrelated library staying approved
- [ ] 5.3 Update `publication.mjs` to tag prerequisites before dependents at the merge commit and to skip dependents of a failed co-released tag; verify `publication.test.mjs` covers ordering, a failed prerequisite tag and retry from partial tag state without moving tags
- [ ] 5.4 Add post-publication public-proxy hash verification with bounded retries that never queries a version before its tag exists; verify tests cover a matching hash, a mismatch and an unavailable version

## 6. Dependency automation and documentation

- [ ] 6.1 Remove the first-party lookup and tagged-prerequisite rules from `renovate.json` and disable all first-party gomod update types, update the activation test, and verify `renovate-config-validator --strict` and `npm test --prefix release/tooling` pass
- [ ] 6.2 Rewrite the release runbook sections in `release/README.md`, `release/tooling/ACTIVATION.md` and `CONTRIBUTING.md` for the combined pull request, generated requirement files, dependency-closed publication and rollback, and verify `mise run lint-docs` passes
- [ ] 6.3 Switch `release-please-config.json` to the combined shape (`separate-pull-requests: false`, `ai-gateway` separate) and verify `mise run release-check` and an in-memory preview against current `main` produce one library pull request and one Gateway pull request

## 7. Integration

- [ ] 7.1 Run an end-to-end local rehearsal on a scratch branch: prepare a combined candidate for all pending libraries, run readiness, authorization (read-only) and a dry publication against a file proxy, and verify every generated checksum matches rebuilt archives and an external consumer builds
- [ ] 7.2 Run `mise run check`, `mise run test-module-policy`, `mise run test-ci-workflow` and `npm test --prefix release/tooling`, and verify all pass

## Workflow follow-up

- After merge, let the next release run supersede the open per-module release pull requests, review the first combined pull request and record tag, proxy-hash and consumer evidence on #245 and #468.
- Archive the change after the first live combined release is verified.
