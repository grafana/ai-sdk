## Evidence

- Baseline: @ai-sdk/openai 4.0.96 at b7f6255acaae7724adc0fd37aac3dae3f3466578, matching test/conformance/upstream.yaml.
- Read upstream responses/convert-to-openai-responses-input.ts and its tests at that commit; both error variants use JSON.stringify({error: output.value}), including custom tools and grouped children.
- Confirmed the new synthetic request tests fail on the original helper because errors were emitted as bare values.
- `go test ./providers/openai/... -count=1`: PASS, including the full existing module suite (multipart, denial, cache precedence) and new exact unary/stream request tests.
- `go vet ./providers/openai/...`: PASS.
- `OPENSPEC_TELEMETRY=0 openspec validate fix-openai-tool-error-results --strict`: PASS.
- `git diff --check`: PASS.

## Scope and limitations

New HTTP transport tests cover function/custom errors, successful text/JSON, null and empty values, arrays/objects/booleans/large integers, function output schemas, active/inactive OpenAI/Azure namespaces, scalar cache locations and caller immutability. Parallel tests cover reordered children, conversation and previous-response continuation, schema-bearing functions and child cache positions. Custom declarations cannot carry function-only output schema options through the public Go API, so HTTP custom tests use valid custom declarations.

`mise run parity-check` was attempted. After selecting the trusted isolated config, mise could not resolve/install pinned tools (including mage 1.17.2) because outbound DNS and writes to its shared cache were unavailable in the sandbox. The broad parity pipeline did not complete. No conformance input or generated fixture was changed; existing OpenAI fixtures contain no error-text/error-json input requiring regeneration. These synthetic tests prove emitted request shape, not live-provider acceptance. Authentic multipart/cache recording remains #440.
