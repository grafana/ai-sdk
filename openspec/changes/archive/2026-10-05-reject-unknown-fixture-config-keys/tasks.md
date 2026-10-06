## 1. Confirm the gap

- [x] 1.1 Confirm Go `LoadConfig` and TypeScript `loadConfig` accept undeclared keys, and that generation, recording and both Go runners load fixtures only through them.
- [x] 1.2 Compare the two key sets and find the drift: TypeScript reads `allowSystemInMessages`, one recorded fixture sets it, and Go declares no such field.

## 2. Shared evidence and failing tests

- [x] 2.1 Add `testdata/fixture-config/all-keys.yaml` with every key at every level and made-up keys inside every payload map.
- [x] 2.2 Add ten single-typo fixtures under `testdata/fixture-config/invalid/`, each starting with `# unknown: <key>`, covering the top level, tools, messages, message parts, model-output content, provider tools, stream options, approvals, UI messages and the response format.
- [x] 2.3 Add Go `TestLoadConfig_RejectsUnknownFields` and `TestLoadConfig_AcceptsEveryAlignedKey`, the second also requiring every Go yaml tag to appear in all-keys at its level.
- [x] 2.4 Add the TypeScript `fixture config keys` suite with the same rejection cases and the same all-keys coverage check for every key list.
- [x] 2.5 Run the Go rejection test against the unstrict loader: all ten cases fail. Run it with strict top-level decoding but plain `Node.Decode` in `MessageConfig.UnmarshalYAML`: the message and message-part cases still fail.

## 3. Make both loaders strict

- [x] 3.1 Decode `LoadConfig` with `KnownFields(true)` and route both decodes in `MessageConfig.UnmarshalYAML` through `decodeKnownFields`.
- [x] 3.2 Declare `AllowSystemInMessages` on the Go `Config`. Removing it makes `TestLoadConfig_AcceptsEveryAlignedKey` fail with `field allowSystemInMessages not found in type conformance.Config`.
- [x] 3.3 Add TypeScript `parseConfig`, the compiler-checked key lists and the structured walk, and call it from `loadConfig`.
- [x] 3.4 Document the rule and how to add a key in the conformance README.

## 4. Validate

- [x] 4.1 Load all 139 fixtures through both strict loaders: Go through `mise run test-conformance`, TypeScript through `loadConfig`; none rejected.
- [x] 4.2 `go test -tags conformance ./` in `test/conformance` and `tsx --test common.test.mts` (40 of 40): pass.
- [x] 4.3 `pnpm --filter @ai-sdk/conformance-tools typecheck`, `mise run fmt-check`, `mise run vet` and `mise run lint`: clean.
- [x] 4.4 `openspec validate --all --strict`: this change validates; the two failing items are pre-existing main specs it does not touch.
