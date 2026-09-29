# Tasks

- [x] Integrate current main source/provenance checks without restoring global standalone source gates.
- [x] Correct root middleware ownership and pinned preview implementation.
- [x] Distinguish Gateway application from library component inventory.
- [x] Add release identity/current candidate checks and selected-library validation.
- [x] Add ownership/freshness/selection policy tests.
- [x] Disable automatic tag and GitHub Release creation pending reviewed activation.
- [ ] Integrate #263 same-revision workspace image readiness for Gateway versioned releases; source baseline now includes #263's build recipe.
- [ ] Implement and test release-please-based Gateway intent for relevant linked workspace changes using the pinned 17.6.0 engine and independent Gateway history.
- [ ] Prove that a provider-only behavior change produces a Gateway release candidate after that provider has released independently, while docs/examples-only changes do not and component versions remain independent.
- [ ] Exercise the release-please bootstrap and generation path at 17.6.0 with representative histories.
- [ ] Verify App installation, broker and Vault prerequisites.
- [ ] Add and test the first-party Renovate tagged-prerequisite rule after publication is ready; current main keeps internal version updates disabled.
- [ ] Configure required readiness/current-base protections with explicit authorization.
- [ ] Review exact-candidate publication, retries and pending release PRs before enabling tags.
