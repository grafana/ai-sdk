## Decisions

### Validate All Upstream Consumers

Treat `test/conformance/tools/package.json`, `test/integration/package.json`,
and `test/cli/package.json` as baseline consumers. Conformance tools must list
every package declared in `upstream.yaml`; integration and CLI packages only
need to keep any `ai` or `@ai-sdk/*` packages they use at the registered
baseline version.

### Add Coverage Inventory as a Separate Signal

Add `mise run parity-coverage` instead of folding every check into
`validate-parity-baseline`. The inventory should fail on local consistency
problems, such as missing expected snapshots or missing `INDEX.yaml` coverage.
When a local upstream clone is available, it should also report upstream
streaming fixtures that are not listed in the local index.

### Expand Harness Expressiveness Before Fixture Churn

Support the missing conformance config fields first so future bug fixes and
features can be fixture-driven. Config support should be mirrored in Go replay,
TypeScript generation, and TypeScript recording.

### Add Hook-Level Interop Tests

The current integration tests validate parser/schema compatibility. Add a small
React hook-level suite so `useChat`, `useObject`, and `useCompletion` behavior
is covered by the same upstream package baseline.

### Add API-Shape Drift Reporting

Start with a report rather than a strict generated binding. The report should
compare upstream LanguageModelV4 discriminator values with Go constants and
surface missing values as actionable parity risk.
