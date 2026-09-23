## 1. Preserve the SDK prerequisite implementation

- [x] 1.1 Retain the published Apache commits and registered upstream reference; assign only SDK requirements to this change.
- [x] 1.2 Preserve direct tool validation, nil-args adaptation, native conversion and normalized marker consumers with their focused tests.
- [x] 1.3 Preserve distinct text/UI input-start inference and cross-language regression coverage.
- [x] 1.4 Preserve standalone Grafana unary/SSE readers and metadata projection tests.
- [x] 1.5 Preserve Anthropic MCP option presence and unary deadline/explicit-timeout tests.

## 2. Make the intermediate tree mergeable

- [x] 2.1 Migrate Gateway boolean consumers and immutable pins without enabling provider tools or MCP; retain rejection coverage.
- [x] 2.2 Scope SDK documentation and parity coverage to implemented behavior, leaving service activation to successors.
- [x] 2.3 Run tests, build, vet, lint, parity, integration, module-resolution and Gateway-boundary checks on this intermediate tree.
- [x] 2.4 Validate this OpenSpec change, commit and publish the first draft PR; keep the change unarchived.

## 3. Integrate the current main baseline

- [x] 3.1 Merge main's registered ai 7.0.107/provider 4.0.17/Gateway 4.0.87/Anthropic 4.0.58 reference and review the affected source/tests; preserve unary warnings, CAP authentication and Mantle continuation.
- [x] 3.2 Publish combined source at immutable ref 25ba9521879f and repin affected consumers, including standalone Bedrock's OpenAI dependency.
- [x] 3.3 Re-run all intermediate tests, build, vet, lint, parity, integration, module-resolution and Gateway-boundary gates against the merged reference; keep this change unarchived.
