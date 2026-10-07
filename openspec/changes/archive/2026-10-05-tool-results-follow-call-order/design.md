## Context

At `0e8f35e`, `StreamTextResult` appends to `step.ToolResults` in four places: provider-executed results and rejected calls while the model stream is read, auto-denied approvals in the call loop before any tool starts, and executed results after every tool finishes. Each kind lands in the order it was handled, so the slice only followed call order when a step held one kind.

A probe on `main` with calls `[good, missing]` showed `step.ToolResults` as `[missing good]`, while the next request listed tool results as `good, missing`, because the request is built from recorded response content. `buildContent` looks results up by call ID, so `step.Content` does not depend on the slice order either.

Upstream `ai@7.0.116` builds `step.toolResults` from content in arrival order. The Go spec already promised call order, so this keeps the Go contract and records the difference.

## Decisions

### 1. Sort once, where every path meets

All four append sites finish before `buildContent` runs in the step's completion block, so a single stable sort there covers every mix, including steps where a tool-choice violation skips execution. Threading call indices through each append site would touch four code paths for the same result.

### 2. Results outside the step go first

A provider can report a result for a call made in an earlier step. Such a result has no position in this step's calls; it keeps its relative order ahead of the step's own results, which is where it arrived. Results that share a call ID, such as a preliminary result followed by a final one, keep their order because the sort is stable.

### 3. Keep call order instead of adopting arrival order

Switching Go to upstream's arrival order would make the field depend on goroutine scheduling for concurrent tools and contradict the existing spec and the tests built on it. Call order stays, recorded as a `documented-deviation` in `upstream.yaml`.
