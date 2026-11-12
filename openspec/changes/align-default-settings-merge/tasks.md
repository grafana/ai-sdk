# Tasks

## 1. Shared provider-option merge

- [ ] 1.1 Move the step provider-option merge from `provider_options.go` into a root-module internal package and switch `StreamText` to it; verify `GOFLAGS=-buildvcs=false go test ./...` passes with the existing step-merge tests unchanged in behavior
- [ ] 1.2 Port the `merge-objects.test.ts` cases that apply to JSON (flat, nested, arrays replaced, null handling, complex nested, empty objects, nil inputs, non-mutation, prototype keys at top level and nested) to the internal package; verify with `go test ./internal/...`

## 2. DefaultSettings merge rules

- [ ] 2.1 Merge `ProviderOptions` through the shared package and return its errors; port the upstream provider-option cases ("merge provider metadata with default settings", "merge complex provider metadata objects", "nested provider metadata objects", empty default, empty param, both undefined) and add typed-option `ResolveOption` and prototype-key cases; verify with `go test ./middleware/...`
- [ ] 2.2 Merge `ResponseFormat` by active members per the spec; add tests for JSON inheriting the default schema and name, caller schema winning, text over a JSON default, and nil caller; verify with `go test ./middleware/...`
- [ ] 2.3 Port the remaining upstream cases (temperature keep zero, default when unset, param precedence, maxOutputTokens, stopSequences, topP, headers merge, empty default headers, empty param headers, both undefined). Add Go cases for empty `StopSequences`/`Tools` kept, automatic `ToolChoice` over a named default, and `Reasoning`. Verify with `go test ./middleware/...`
- [ ] 2.4 Update the `DefaultSettings` and `DefaultSettingsOptions` godoc for the merge rules, `RawProviderOption` results and the error return; verify with `mise run lint`

## 3. Parity records

- [ ] 3.1 Add the `default-settings-go-representation` entry to `test/conformance/upstream.yaml` `knownGaps`; verify with `mise run validate-parity-baseline`

## 4. Integration checks

- [ ] 4.1 Run `mise run check`, `mise run lint` and `mise run parity-check`, and report any sub-step the sandbox cannot run
- [ ] 4.2 Run `openspec validate align-default-settings-merge --strict`

## Workflow follow-up

- Open a draft PR titled `fix(middleware): align DefaultSettings merging with upstream defaultSettingsMiddleware` that notes the caller-visible changes.
- Archive the change after merge so the main spec reflects the new rules.
