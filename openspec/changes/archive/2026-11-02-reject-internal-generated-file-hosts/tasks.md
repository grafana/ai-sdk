## 1. Reproduce and implement

- [x] 1.1 Add failing initial-URL and redirect tests with public DNS answers, including uppercase and trailing dots.
- [x] 1.2 Add the shared `.internal` suffix check and verify allowed lookalikes.

## 2. Validate and complete

- [x] 2.1 Run focused tests, root tests/race/vet/lint and parity checks; record any limits.
- [x] 2.2 Validate and archive the OpenSpec change, updating the owning spec.

Validation passed: downloader tests (red before the fix, green after), root
`go test ./...`, focused downloader `-race`, root `go vet` and golangci-lint
2.15.0, documentation structure/style checks, strict OpenSpec validation, and
all `parity-check` constituent commands run directly with the pinned tools.
Coverage inventory and provider-shape used a temporary upstream checkout at
`612be2311464804d1e4897afacbf8f3fc2f6d9e3`. No provider recordings or UI
fixtures were added or changed; no live provider acceptance is claimed.
