## MODIFIED Requirements

### Requirement: Partial output streaming

StreamTextResult SHALL expose PartialOutputStream() as a channel of json.RawMessage partial snapshots from configured output mode. Partial failures SHALL be silently skipped. Partials SHALL parse only the step's first text part, which is the first text part any text event names, including `text-start`; deltas of other text parts SHALL pass through unparsed. A snapshot SHALL be published when it differs from the last snapshot published in the same step; each step SHALL start with no previous snapshot, as upstream's `start-step` reset does. Every published snapshot SHALL arrive exactly once in production order, losslessly even if consumed after generation. The stream SHALL close only after generation finishes and queued snapshots are delivered.

#### Scenario: Receive partial output during streaming

- **WHEN** `Output` is set and the LLM streams text deltas that form partial JSON
- **THEN** `PartialOutputStream()` SHALL emit `json.RawMessage` values for each successfully parsed partial snapshot
- **AND** only emit when the parsed result differs from the previous emission in the same step
- **AND** deliver each emitted snapshot exactly once and in production order

#### Scenario: A later step repeats the previous step's partial

- **WHEN** step 1 streams `{"name":"a`, makes a tool call, and step 2 streams `{"name":"a` and then `"}`
- **THEN** `PartialOutputStream()` SHALL emit `{"name":"a"}` once for each step
- **AND** step 2 SHALL emit the text-delta `{"name":"a` when it parses, and `"}` at `text-end`

#### Scenario: Array partial before any valid element

- **WHEN** array output streams `{"elements":[`
- **THEN** `PartialOutputStream()` SHALL emit `[]`, not `null`

#### Scenario: Another text part interleaves with the output text

- **WHEN** a step starts text parts `t1` and `t2`, and `t2` streams `note` between `t1`'s deltas `{"name":` and `"a"}`
- **THEN** `PartialOutputStream()` SHALL emit `{}` and then `{"name":"a"}`
- **AND** the `t2` delta SHALL be emitted when it arrives

#### Scenario: The first text part starts before any delta

- **WHEN** a step starts `t1` and then `t2`, and `t2` streams a delta before `t1` does
- **THEN** partials SHALL parse `t1`'s text, not `t2`'s

#### Scenario: Partial output consumption starts after the channel buffer is exceeded

- **WHEN** generation produces more distinct partial snapshots than the public channel buffer can hold before the consumer starts reading
- **THEN** `PartialOutputStream()` SHALL deliver every snapshot exactly once and in production order
- **AND** structured output generation and result completion SHALL NOT block on the delayed consumer

#### Scenario: No Output set

- **WHEN** `Output` is nil
- **THEN** `PartialOutputStream()` SHALL return a closed channel immediately
