## ADDED Requirements

### Requirement: UI JSON decoding preserves required-field distinctions

UI message JSON decoding SHALL reject absent required fields and null where the registered upstream shape requires a non-null value, for the message envelope and represented parts including nested text citation sources and tool approvals. Required strings SHALL NOT be inferred from their nonzero values. Opaque fields that permit JSON null SHALL distinguish a present null from omission. These checks SHALL apply to UI JSON, not to Go-built values or provider and SSE codecs.

#### Scenario: Malformed UI history stops before model I/O

- **WHEN** UI history supplied over JSON omits a required field or supplies null instead of a required non-null value
- **THEN** decoding SHALL report the field with message or part context
- **AND** the request SHALL NOT invoke the provider

#### Scenario: Empty and opaque fields retain their meaning

- **WHEN** otherwise valid UI history contains a required empty string or a permitted opaque null
- **THEN** decoding SHALL accept it without synthesizing or dropping the required field in its supported round trip

#### Scenario: Go JSON matching and compatibility remain intact

- **WHEN** valid represented UI history uses ordinary Go JSON struct-field matching, duplicate members, or additive fields
- **THEN** decoding SHALL retain the existing matching and compatibility rules
- **AND** an unrepresented nonempty part discriminator, including `steering-message`, SHALL remain round-trippable as raw JSON without becoming supported Agent history
