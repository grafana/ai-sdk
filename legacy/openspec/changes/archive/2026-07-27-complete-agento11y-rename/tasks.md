## 1. Change scaffold

- [x] 1.1 Create the change artifacts: `.openspec.yaml`, `proposal.md`, `design.md`, `tasks.md`.
- [x] 1.2 Write delta specifications for `agent-observability-middleware`, `grafana-provider-options`, and `context-enrichment-middleware`.
- [x] 1.3 Pass strict OpenSpec validation for the new change.
- [x] 1.4 Classify the change against the registered upstream baseline in `test/conformance/upstream.yaml` and the coverage map in `test/conformance/PARITY.md`, and record the result in `design.md`.

## 2. Internal identifiers and the test environment variable

- [x] 2.1 Rename the 18 unexported `…ToSigil…` / `…FromSigil` mappers and the `sigilMsg` loop variable to their `Agento11y` forms across `map_request.go`, `map_response.go`, `media.go`, and `hooks.go`, updating every call site in `generation.go`, `map_stream.go`, and `hooks.go`.
- [x] 2.2 Rename the 26 affected test functions and update the comments that name mappers directly.
- [x] 2.3 Replace `SIGIL_REGEN` with `AGENTO11Y_REGEN` in the `os.Getenv` call, both skip messages, and both assertion failure messages.
- [x] 2.4 Update the normative mapper names in the active `agent-observability-middleware` specification.
- [x] 2.5 Confirm the module tests pass and that `AGENTO11Y_REGEN=1` regeneration leaves every `expected_*.json` snapshot unchanged.

## 3. Hooks telemetry names

- [x] 3.1 Add failing span coverage in `recording_test.go` using an OpenTelemetry span recorder for the span name and the allow, deny, and transform attributes.
- [x] 3.2 Rename the span to `aisdk.hooks.preflight` and the attributes to `aisdk.hooks.result`, `aisdk.hooks.action`, and `aisdk.hooks.rule_id`, leaving the Go identifiers unchanged.
- [x] 3.3 Rewrite the justification comments so they state that the agento11y client owns the generation span and `agento11y.generation.id`, and that these attribute keys belong to ai-sdk.
- [x] 3.4 Promote `go.opentelemetry.io/otel/sdk` to a direct dependency and confirm the module tests pass.

## 4. Provider-options wire key

- [x] 4.1 Update the marshal assertions and add unmarshal coverage for `"agentObservability"`, plus a regression case proving the old key does not populate the field.
- [x] 4.2 Change the struct tag to `json:"agentObservability,omitempty"`.
- [x] 4.3 Update the enrichment pass-through test and the enrichment package documentation to `grafana.agentObservability`.
- [x] 4.4 Run the Grafana provider and enrichment middleware test tasks.

## 5. Documentation and active specifications

- [x] 5.1 Correct the two stale SDK-ownership claims in `middleware/agentobservability/doc.go`.
- [x] 5.2 Update the active `agent-observability-middleware` specification for `agento11y.generation.id`, the new span name and attribute keys, and `AGENTO11Y_REGEN`.
- [x] 5.3 Update the active `grafana-provider-options` and `context-enrichment-middleware` specifications for `"agentObservability"`.
- [x] 5.4 Pass strict OpenSpec validation and the documentation linter.

## 6. Renovate label

- [ ] 6.1 **Manual, human-only:** rename the GitHub label `sigil-conformance-review` to `agento11y-conformance-review` in `grafana/ai-sdk`.
- [x] 6.2 Update the configured label in `renovate.json`, and do not merge it before the GitHub label rename lands.

## 7. Verification

- [x] 7.1 Confirm `rg -in 'sigil' --glob '!openspec/changes/**' --glob '!*.sum' .` returns no matches.
- [x] 7.2 Run format, vet, lint, docs lint, all tests, build, tidy, the Agent Observability conformance task without regeneration, `mise run validate-parity-baseline`, and strict OpenSpec validation.
- [x] 7.3 Confirm `middleware/agentobservability/testdata/` is unchanged and no archived change was modified.
- [ ] 7.4 **Manual, human-only:** audit Grafana Cloud dashboards, alert rules, and TraceQL queries for `aisdk.sigil.hooks.preflight` and `sigil.hooks.*` from the ai-sdk path, or record the risk as accepted.
