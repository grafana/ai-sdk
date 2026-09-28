## Why

Issue #260 remains open and reproducible in the current Go API: `output.GenerateObject` and `output.StreamObject` cannot recover an otherwise usable generated value after JSON parsing or schema validation fails. The registered `ai@7.0.109` baseline (`4e8c387622ee1bb0d55841664416d38754d5c9a3`) offers an opt-in `repairText` callback on the corresponding legacy APIs; Go currently returns the validation error without attempting repair.

## What Changes

- Add an opt-in, Go-idiomatic text repair callback to the shared `GenerateText`/`StreamText` structured-output configuration, reachable both through direct `WithOutput` calls and the typed `output.GenerateObject`/`output.StreamObject` wrappers.
- If complete output JSON parsing/schema validation fails, call the callback once with original generated text and the parsing/validation error; on an accepted repair, run the same complete-output validation once on returned text. Schema-valid text rejected solely by Go typed conversion does not trigger repair. A declined repair preserves the original error; callback errors and invalid repaired text remain observable as output errors.
- Keep raw model text, content, stream chunks, partial values, usage and response metadata unchanged; only the validated final output value/error may differ. Without the callback, behavior is unchanged.
- Add focused regression tests against synthetic model responses; do not invent provider recordings or alter SSE framing.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `structured-output`: Opt-in, single-attempt complete-output repair, including wrapper/direct-call consistency, validation failure behavior and raw-output preservation.

## Impact

- Root orchestration: `options.go`, `output.go`, `streamtext.go` and focused output tests; `output/value.go` wrappers already forward shared options, so their signatures need not change.
- Root `ErrInvalidOutputText` marker distinguishes repairable JSON/schema failure from a typed conversion failure that also wraps `ErrNoObjectGenerated`; output modes (`output/object.go`, `array.go`, `choice.go`, `json.go`) preserve inspectable parser/validator causes. `Output`'s three-method interface and provider `LanguageModel` contracts stay unchanged.
- No new dependencies or breaking API changes. This does not change the #226 stop/non-stop final-parse gate or the #114 Gateway response-format transport. Frontend UI chunk format does not change, so no new wire fixture is needed unless implementation changes that assertion.
