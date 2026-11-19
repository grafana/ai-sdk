## 1. Report Grafana-produced errors

- [x] 1.1 Add a regression test with Grafana's recorded 404, 403 and 429 bodies that fails against the current client.
- [x] 1.2 Return a Grafana-named non-retryable APICallError with status, message and ResponseBody when a JSON error body has no `error` member.
- [x] 1.3 Run the Grafana module tests, vet, golangci-lint and strict OpenSpec validation.
