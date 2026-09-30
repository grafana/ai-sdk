## 1. Confirm baseline and capture independent request expectations

- [x] 1.1 Re-read `test/conformance/upstream.yaml` and `PARITY.md`; resolve the exact registered OpenAI package source/tests without changing a shared upstream checkout. Confirm the ordinary nine-kind allowlist and three function fallbacks; reassess this plan if the registered behavior changed from 4.0.72.
- [x] 1.2 Run `mise deps` before code validation/capture. Using the existing pinned Node environment at `test/conformance/tools`, capture credential-free TypeScript `doGenerate`/`doStream` HTTP requests for `openai.shell`, `openai.local_shell`, and `openai.tool_search`, each selected by canonical name and configured alias with identical deterministic model/prompt/options.
- [x] 1.3 Commit request goldens under `providers/openai/testdata/forced_tool_choice/` with a reproducible capture recipe, exact inputs, pin/tag/commit, regeneration command and any projection/normalization policy. Explicitly label them synthetic request evidence, not provider recordings or live acceptance. Preserve the complete `tools`/`tool_choice` projection and document unrelated body differences rather than repairing them here; verify repeat capture yields identical goldens.

## 2. Establish failing provider regressions before the serializer fix

- [x] 2.1 Extend `TestPrepareTools_ProviderToolChoiceVariants` in `prepare_tools_test.go` to cover the three kinds by canonical name and configured alias. Replace discriminator-only checks with exact whole-choice assertions, including canonical `name`, and verify declarations remain their original provider kinds. Include both canonical-named and aliased tool declarations.
- [x] 2.2 Add/retain exact controls for all nine ordinary hosted choices with aliases, custom named choice, ordinary/unmapped function choices, `auto`/`none`/`required`, and no-tools omission. Retain allowed-tools override coverage: shell/local-shell stay hosted entries, tool-search stays warned/dropped and empty surviving selections still error before HTTP.
- [x] 2.3 Add `TestForcedToolChoice_RequestSnapshots` in a focused provider test file using `NewResponses`, a fake `roundTripFunc` transport and actual `DoGenerate`/`DoStream` request encoding. Exercise the twelve kind/selection/mode cases, compare against pinned-TS golden request projections, assert the full choice object, unchanged declarations and expected streaming-mode field, and drain every synthetic stream.
- [x] 2.4 Run `cd providers/openai && go test -run 'TestPrepareTools_ProviderToolChoiceVariants|TestForcedToolChoice_RequestSnapshots' .` against the unfixed serializer; record failures showing hosted shell/local-shell/tool-search choices differ from the independent function-shaped expectations. Do not accept Go-generated goldens as the oracle.

## 3. Apply the narrow ordinary-choice correction

- [x] 3.1 Replace `hostedToolChoiceType`'s complete-name-map scan with the exact ordinary hosted allowlist and remove the unreachable shell forced-choice constructor branch; keep the existing function fallback, custom handling, and typed apply-patch/programmatic constructors.
- [x] 3.2 Verify the complete `providerToolNames` table, alias/response mappings, declaration conversion, `allowed_tools.go` and override ordering remain unchanged. Re-run the focused red tests and all control tests, requiring green exact-choice and request-golden assertions.

## 4. Validate and report the evidence boundary

- [x] 4.1 Run `cd providers/openai && go test ./...` and `go vet ./...`; format only touched Go files. Confirm repeated focused HTTP snapshot runs leave committed goldens unchanged and no credentials or volatile headers are captured.
- [x] 4.2 Run `mise run test-conformance` and `mise run parity-check` against the registered baseline. Do not regenerate unrelated expectations or modify any provider `input*.chunks.txt`; existing auto-choice fixtures must remain green without claiming they cover forced choices.
- [x] 4.3 Review the final diff against the exact upstream ordinary-choice source/tests, issue #32 acceptance criteria, and this delta spec. Record red/green output and snapshot provenance; update `PARITY.md` only if a stable evidence boundary changes, never with a dated assessment or duplicated issue inventory. State that synthetic requests do not establish live OpenAI acceptance.
