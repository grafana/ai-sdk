## 1. Establish Mantle regression evidence

- [x] 1.1 Reconfirm the registered OpenAI 4.0.71 / Bedrock 5.0.88 reference and merged producer `9fb535a028172fd48a73457c655d1389d6e6290c`; verify its selected OpenAI module is still downloadable from the public proxy before editing requirements.
- [x] 1.2 Add table-driven request-capture tests through `mantle.NewResponses` in `providers/bedrock/mantle/provider_test.go` for both `DoGenerate` and `DoStream`, using user → reconstructed assistant → user history and deterministic fake transport.
- [x] 1.3 Cover storage disabled with and without stale IDs, storage enabled without IDs, both supplied phases, explicit empty text, and storage-enabled item references. Compare complete assistant items, user ordering, store settings, and stream flags; drain streams and check successful termination without error parts. Retain response-derived reference and OpenAI metadata attribution coverage.
- [x] 1.4 Run the focused new tests from `providers/bedrock` with `GOWORK=off go test -mod=readonly` against the old dependency and capture the expected encoding failures before changing the requirement. Confirm workspace runs use the already-corrected producer rather than treating them as adoption evidence.

## 2. Adopt the published correction

- [x] 2.1 Update `providers/bedrock/go.mod` to require `github.com/grafana/ai-sdk/providers/openai v0.0.0-20260923145714-9fb535a02817` and update necessary `providers/bedrock/go.sum` entries with the workspace disabled. Inspect the dependency diff; add no replacements or unrelated upgrades.
- [x] 2.2 Rerun the focused regressions and the full Bedrock suite in workspace mode and with `GOWORK=off go test -mod=readonly ./...` from `providers/bedrock`; verify routing, authentication, stored references, and metadata attribution remain unchanged.
- [x] 2.3 Align the conformance test module's OpenAI requirement and necessary sums with Bedrock's published revision, retaining its test-only replacements; prove `GOWORK=off go test -mod=readonly -tags conformance ./...` works without modifying unrelated dependencies.

## 3. Validate parity and delivery

- [x] 3.1 Format changed Go tests and run Bedrock vet/lint using the repository toolchain.
- [x] 3.2 Run `mise run verify-module-resolution` and retain evidence of fresh public-cache standalone resolution. Stop and report any blocker rather than adding a replacement or expanding dependency scope without approval.
- [x] 3.3 Run `mise run parity-check`; inspect any fixture differences and preserve provider input provenance. Do not fabricate Mantle recordings or regenerate expectations merely to hide failures.
- [x] 3.4 Check temporary AWS access with `aws sts get-caller-identity --profile workloads-dev`. When credentials and model access are available, repeat the real-adapter `openai.gpt-oss-20b` recall check in `us-east-2` with the exact code-word history, `store: false`, and `max_output_tokens: 256`; require completed status and correct `cobalt` recall. Verified through the real adapter with temporary SSO credentials: HTTP 200, Responses status `completed`, text `cobalt`; no custom request rewriting or credential-dependent CI.

## 4. Record the evidence boundary

- [x] 4.1 Update `test/conformance/PARITY.md` with the Mantle unary/streaming assistant-history evidence and published-dependency adoption boundary, linking #207 without adding an issue-status catalog. Preserve existing unsupported surfaces and distinguish deterministic captures from live evidence.
- [x] 4.2 Review the final diff against the pinned upstream converter and this change's scenarios; confirm no Mantle encoder fork, routing/authentication change, frontend wire change, baseline drift, or unrelated API work was introduced. Record executed checks, remaining limitations, and that Bedrock consumers need the adoption revision rather than only the producer merge.
