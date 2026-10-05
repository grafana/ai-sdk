## 1. Confirm the registered reference

- [x] 1.1 Read `test/conformance/upstream.yaml` and `PARITY.md`; confirm the registered `ai` version and that core output parsing needs Go unit coverage rather than provider fixtures. Record the gap against the `7.0.118` version named in the issue.
- [x] 1.2 Fetch `packages/ai/src/generate-text/output.ts` at tag `ai@7.0.116` and derive the required semantics for `array.parseCompleteOutput`, `choice.parseCompleteOutput` and `choice.parsePartialOutput`.

## 2. Establish failing regressions

- [x] 2.1 Add `TestChoiceOutput_ParsePartial_RequiresStringResult` with a single option, covering `{`, `{"other":`, `{"result":null,`, `{"result":null}`, `{"result":1}`, `{"other":"yes"}` and `{}`, plus the unique-prefix and empty-string cases.
- [x] 2.2 Add `TestChoiceOutput_ParseComplete_WrapperExtraction` and `TestArrayOutput_ParseComplete_WrapperExtraction`: an unrelated wrapper property parses, while missing, null, wrong-typed and out-of-option values, non-object documents and an invalid element fail with `ErrNoObjectGenerated`.
- [x] 2.3 Add `TestWrappedOutputs_RequestSchemasStayStrict` asserting `additionalProperties: false` and the required field in both response formats.
- [x] 2.4 Run the new tests against the unfixed parsers and record the failures: `git stash push output/choice.go output/array.go && go test ./output/ -run '...'` reported `--- FAIL` for all three new tests, including the three resultless partial subtests.

## 3. Align the parsers

- [x] 3.1 In `output/choice.go`, add `unmarshalWrapperObject` and `wrapperString`, require a present string `result` in `ParsePartial`, and replace wrapper-schema validation in `ParseComplete` with the field and option-set checks. Keep the prefix rules, now expressed with `strings.HasPrefix`.
- [x] 3.2 In `output/array.go`, replace wrapper-schema validation in `ParseComplete` with the `elements` array check, per-element schema validation and typed unmarshalling.
- [x] 3.3 Leave `ArrayOutput.ParsePartial`, both `ResponseFormat` methods and the object and JSON modes unchanged.

## 4. Validate

- [x] 4.1 `go test ./output/ -count=1`: ok.
- [x] 4.2 `go vet ./output/` and `gofmt -l output`: clean.
- [x] 4.3 `go test ./...`: the only failure is `internal/releasecheck`, which scans untracked local `dogfood/` modules excluded through `.git/info/exclude` and fails the same way on a clean checkout.
- [x] 4.4 `openspec validate --all --strict`: this change validates; the two failing items, `spec/gateway-reasoning-content` and `spec/provider-v4-core-types`, are pre-existing main specs this change does not touch.
