## 1. Execution boundary

- [x] 1.1 Extract the selector API, Gateway-control handling and shared deadline with unary/streaming regression tests.
- [x] 1.2 Keep every command call site on the catalog adapter; retain existing account/listener behavior.
- [x] 1.3 Extract bounded native construction, ordered default fallback and transport/isolation/preflight tests.
- [x] 1.4 Record internal-engine coverage separately from authenticated service or deployed acceptance.

## 2. Independent validation

- [x] 2.1 Run Go/race, parity, command/integration, build/vet/lint, docs, module-policy and image gates on this branch.
- [x] 2.2 Validate this OpenSpec change and verify the new engine is not wired into the command.

## 3. Review-driven construction cleanup

- [x] 3.1 Separate wire decoding from typed request-only construction, retaining strict credential validation.
- [x] 3.2 Share explicit native constructors with configured execution without sharing configuration or account lookup.
- [x] 3.3 Preserve regression coverage and rerun independent validation before pushing PR #368.

## 4. Account configuration review

- [x] 4.1 Replace opaque request/custom credential decoding with plain account configs.
- [x] 4.2 Support service-approved base URLs and OpenAI organization/project overrides while retaining service-owned transport and retries.
- [x] 4.3 Add native/client boundary regressions, document the extension and run independent gates.

## 5. Decoder simplification review

- [x] 5.1 Replace account schema/normalization with struct decoding, account-count validation and exact endpoint approval.
- [x] 5.2 Prove ordinary Go decoding, shared request limits and native HTTP header rejection; update the existing contract.
- [x] 5.3 Rerun independent gates and push PR #368 without changing the other stack branches.

## 6. Client-guide usability review

- [x] 6.1 Rewrite the Gateway guide around application tasks, retain availability/security guidance and validate its links and Go/pinned TypeScript examples.

## 7. Review/fix loop

- [x] 7.1 Align fixed BYOK diagnostics with the plain-account contract and prove secret-safe unary/streaming errors.
- [x] 7.2 Re-review the correction and confirm focused, parity and source-integration validation without changing the other stack branches.
