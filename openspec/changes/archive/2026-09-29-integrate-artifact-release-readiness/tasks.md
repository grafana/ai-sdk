# Tasks

- [x] Integrate current main source/provenance checks without restoring global standalone source gates.
- [x] Correct root middleware ownership and pinned preview implementation.
- [x] Distinguish Gateway application from library component inventory.
- [x] Add release identity/current candidate checks and selected-library validation.
- [x] Add ownership/freshness/selection policy tests.
- [x] Keep all release automation inactive, including release PR preparation, pending #245 activation.

## Deferred activation work

Issue #245 owns the remaining work. It must integrate #263's versioned-image
validation, implement Gateway intent from linked workspace changes, and prove
independent release histories against release-please 17.6.0. It also owns exact
bootstrap and dependency-only scenarios, App/broker/Vault verification, tested
first-party Renovate progression, required-check/current-base protection, and
publication/retry review. The current main intentionally keeps internal Renovate
version updates disabled. None of these activation tasks is claimed complete here.
