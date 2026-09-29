## MODIFIED Requirements

### Requirement: Synthetic terminal adapter errors
Before a valid finish is written, an unsupported stream family, lifecycle violation, invalid provider output, premature channel close, mapping failure, frame overflow, total timeout, idle timeout, or caller cancellation SHALL cancel provider work and produce at most one synthetic terminal safe error when the writer remains usable. Lifecycle, mapping, framing, premature-EOF, and invalid-output failures SHALL use the canonical internal category; cancellation and timeout SHALL retain their closed categories. A terminal error SHALL not close active blocks synthetically or emit a synthetic finish. If a terminal event has already been written or the writer has failed, no further event SHALL be attempted.

#### Scenario: Provider channel closes before finish
- **WHEN** the provider stream closes without a valid finish
- **THEN** the handler SHALL attempt one terminal internal error and SHALL not emit a finish or `[DONE]`

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives reasoning, provider-executed/dynamic/preliminary tool behavior, file, custom, raw, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Provider errors precede an adapter failure
- **WHEN** one or more non-terminal provider errors are written before a later lifecycle failure
- **THEN** those provider errors SHALL remain in order and at most one additional synthetic terminal error SHALL end the stream
