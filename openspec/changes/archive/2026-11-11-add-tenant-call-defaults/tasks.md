# Tasks

## 1. Public types

- [x] 1.1 Add `default_instructions.go` and `default_settings.go` to `ai-gateway/pkg/extension/tenanttype`: the schema and example JSON, descriptions, `EnabledByDefault: false`, and the exported `DefaultInstructions()` and `DefaultSettings()` accessors. Compile schemas without panicking, and add both types to `registry`. Update the `All` and package godoc. Verify with `tenanttype` tests: `All()` returns both, in name order; the members are `defaultInstructions` and `defaultSettings`; each example passes its schema; and each schema accepts and refuses the boundary values in design.md.
- [x] 1.2 Hand-write the `defaultInstructions` and `defaultSettings` members in `kinds/middleware_members.cue`, then run `mise run generate-ai-gateway-kinds`. Verify that the `manifestdata` member-mirror and manifest tests pass for both types, and update `TestManifest_MiddlewareSchema` to expect `type`, `enabled` and the two members.

## 2. Implementations

- [x] 2.1 Add `ai-gateway/pkg/internal/calldefaults`: the `transformer` adapter, the two factories (stop sequences clipped) and `Definitions()`, each bound with `FailSkip`. Verify with unit tests:
  - Instructions: a system message first, a system message later, none (both prepended, in order), and the input prompt not mutated.
  - Settings, ported from upstream's cases: unset fields filled; a call's `0` and `stopSequences: []` kept; headers, provider options, tools, tool choice, response format and reasoning unchanged; an append to the returned stop sequences leaves the next call unaffected.
  - Decode: safe integers exact, every member admission accepts decodes, a malformed member returned as an error, and no collectors registered.
  - Definitions: tenant scope, nil `Keys`, a `Decode`, `skip`.
- [x] 2.2 Return `calldefaults.Definitions()` from `gateway.DefaultExtensions()` and update its godoc. Verify with the `pkg/gateway` tests: the shipped definitions pass `ValidateDefinitions` against `tenanttype.All()`, and the `Empty` assertion is replaced.

## 3. Gateway evidence in Go

- [x] 3.1 Admission table with the shipped definitions, through `tenantconfig` and the protocol test host. It covers every refusal and boundary listed in the design's test plan, each refusal naming its location and holding no sentinel. Replace `TestMiddlewareAdmission_ShippedTypesRefuseEveryCreate` with the new shipped scenario.
- [x] 3.2 A gateway test through the test host with the shipped definitions, run over three namespaces (enabled, disabled and deleted, none). Each catalog call's fake-Anthropic body shows the expected system blocks and settings. A call with its own system message keeps only that message. Rework `TestTenantConfiguration_ShippedDefinitionsReadNothing` into "reads once per namespace, calls unchanged", plus a no-tenant-definition test that makes no reads. Check the self-hosted test's warning with the shipped build.
- [x] 3.3 A pipeline test with the shipped definitions and a fake reader: an operator-catalog call and a request-BYOK call, unary and stream, both reach the model with the defaults. A stored member that fails the schema skips that extension, the call runs, and the failure is counted.

## 4. End-to-end suite

- [x] 4.1 `tests/fake-anthropic.mjs`: a unary call whose last user text is `e2e-echo-request` gets back the JSON of `system`, `temperature`, `max_tokens` and `stop_sequences`. Every other call keeps the fixed text. Update the file comment.
- [x] 4.2 `tests/middleware.spec.ts`: the serial shipped-types block from design.md's end-to-end plan, covering:
  - the folder and folderless saves;
  - the round-trip, including the raw `seed` text;
  - Grafana refusals with sentinels, including `headers`;
  - the admission refusals with their locations;
  - a valid update, a disable-only update and a delete;
  - Viewer and Editor `200`/`403`/`403`;
  - the runtime effect with two bounded polls.

  Clean up before and after. Verify with `pnpm typecheck` and `pnpm lint` in `ai-gateway`, and with `playwright test --list`.
- [x] 4.3 `tests/accounts.ts`: tracing off for tests that use `accounts`, through a worker-scoped override. Verify locally with a scratch spec that the option reaches those tests, then delete the scratch spec.
- [x] 4.4 `scripts/e2e.sh`: print the matched API-access record. Verify with `shellcheck` if it's available.

## 5. Documentation

- [x] 5.1 Add `docs/guides/gateway-stack-defaults.md`, the org admin guide listed in design.md. Link it from `docs/README.md`, the extension guide's tenant section and the footers. Update the glossary and the architecture walkthrough. Verify that `mise run lint-docs` passes.
- [x] 5.2 Add the manual development check and the stack-catalog follow-up to the PR body.

## 6. Integration checks

- [x] 6.1 With `GOFLAGS=-buildvcs=false`, run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-ai-gateway-frontend`, `verify-ai-gateway-boundary` and `validate-parity-baseline`, and verify they pass. Record the CI-only checks: the plugin build and validation, the end-to-end suite, and `module-policy.sh`.
- [x] 6.2 Run `openspec validate add-tenant-call-defaults --strict` and verify it passes. Archive the change in the same PR after review.

## Workflow follow-up

- The end-to-end suite runs only in CI. The PR records its result, including the observed folder-annotation status and whether Grafana's messages echo values.
- Root `DefaultSettings` alignment is tracked in [#791](https://github.com/grafana/ai-sdk/issues/791).
