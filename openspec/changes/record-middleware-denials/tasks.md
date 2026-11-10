# Tasks

## 1. Prometheus middleware

- [x] 1.1 Add `Options.IsDenied`, classify denied call errors and stream error parts as status `denied` with error type `none` before other rules, and contain classifier panics; verify with `TestDeniedClassification` in `middleware/prometheus`.
- [x] 1.2 Document the `denied` status in the package godoc and `docs/middleware/prometheus.md`; verify `mise run lint-docs`.

## 2. Logger middleware

- [x] 2.1 Add `Options.IsDenied` and log denied generate, stream setup and stream error-part outcomes with outcome and error type `denied` at `Level`; verify with `TestMiddleware_DeniedCalls` in `middleware/logger`.
- [x] 2.2 Document the option in the package godoc and `docs/middleware/structured-logging.md`; verify `mise run lint-docs`.

## 3. Integration checks

- [ ] 3.1 Run `mise run fmt`, `vet`, `lint`, `test`, `build`, the release checks and `openspec validate record-middleware-denials --strict`; verify all pass.

## Workflow follow-up

- Archive the change after review, and verify the archived specs.
