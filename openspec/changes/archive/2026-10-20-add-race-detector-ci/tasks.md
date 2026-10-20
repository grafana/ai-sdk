## 1. Race detector in CI

- [x] 1.1 Add the `test-race` mise task for the root, Gateway, provider and middleware modules on the Gateway stream path.
- [x] 1.2 Add a `race` job to `ci.yml` and require it for Gateway image publication.
- [x] 1.3 Assert the job and the publication dependency in the workflow structural test.
- [x] 1.4 Document `mise run test-race` in `CONTRIBUTING.md`.
- [x] 1.5 Run the task locally on a clean checkout and confirm no data races.
