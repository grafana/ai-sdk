## Why

[Issue #314](https://github.com/grafana/ai-sdk/issues/314) reports two wrapped-output parser differences against the registered `ai@7.0.116` runtime. `ChoiceOutput.ParsePartial` unmarshals `result` into a zero-value string, so a partial object with no result at all publishes a choice: with the single option `yes`, `{`, `{"other":` and `{"result":null,` all emit `yes`. Separately, the array and choice complete parsers validate the whole strict provider wrapper schema, so a valid `{"elements":["ok"],"extra":true}` or `{"result":"yes","extra":true}` response is rejected, while upstream checks the required wrapper field and validates its value.

Both are older parser bugs rather than upgrade regressions. The request schema stays strict upstream; runtime extraction and request constraints are separate contracts.

## What Changes

- Require a present, string-valued `result` before `ChoiceOutput.ParsePartial` publishes anything, keeping the existing exact-match, unique-prefix and ambiguous-prefix rules.
- Extract wrapped complete values by checking the required field and validating it, instead of validating the strict wrapper schema: `ChoiceOutput.ParseComplete` requires a string `result` within the option set, and `ArrayOutput[T].ParseComplete` requires an `elements` array and validates each element against the element schema.
- Keep both response formats strict, and keep every existing error wrapping `ErrNoObjectGenerated`.
- Add singleton-option partial regressions and complete-parse regressions for unrelated wrapper properties, missing, null and wrong-typed required fields.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `structured-output`: Specify runtime extraction for wrapped array and choice values, and the presence and type checks a partial choice snapshot requires.

## Impact

The implementation is confined to `output/choice.go`, `output/array.go` and `output/output_test.go`. No public Go API, dependency, baseline, conformance, provider or frontend change is required. Callers that relied on a rejected extra wrapper property now receive a parsed value, and callers that saw a choice published from a resultless partial snapshot no longer do.
