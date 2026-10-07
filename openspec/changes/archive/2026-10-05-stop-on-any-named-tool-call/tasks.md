## 1. Reference

- [x] 1.1 Fetch `stop-condition.ts` and `stop-condition.test.ts` at `ai@7.0.116` and list the behavior and test cases to match.

## 2. Implementation and tests

- [x] 2.1 Make `HasToolCall` variadic in `text.go`, matching with `slices.Contains` over the latest step's tool calls.
- [x] 2.2 Turn the `HasToolCall` unit test into a table covering no steps, a match, another tool, a match only in an earlier step, any of several names, none of several names, and no names.
- [x] 2.3 Add `TestStreamText_StopWhenAnyNamedToolIsCalled`: with `StepCountIs(5)` and `HasToolCall("finalAnswer", "search")`, a step that calls `search` ends the run after one step and one model call.
- [x] 2.4 Confirm the multi-name tests cannot compile against the old one-name signature.
- [x] 2.5 Update the agent loop guide.

## 3. Validate

- [x] 3.1 `go test -race ./ -count=1`, `mise run fmt-check`, `mise run vet`, `mise run lint`: clean.
- [x] 3.2 `openspec validate --all --strict`: the two failing items are pre-existing main specs this change does not touch.
