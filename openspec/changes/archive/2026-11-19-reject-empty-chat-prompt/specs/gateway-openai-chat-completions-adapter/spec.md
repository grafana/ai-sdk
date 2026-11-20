## MODIFIED Requirements

### Requirement: Explicit unary request subset

The adapter SHALL validate a closed schema before typed mapping. It SHALL
represent text, client-owned functions and continuations, structured response
formats, reasoning effort and supported sampling settings. Unsupported fields,
multiple choices, storage true, invalid settings and a prompt without content
SHALL fail before model resolution. Duplicate keys SHALL use their last value;
incorrectly cased keys SHALL fail.

#### Scenario: Duplicate and incorrectly cased fields
- **WHEN** a request repeats the model key or includes `Model` instead
- **THEN** the repeated exact key uses its last value and the incorrectly cased
  key fails validation.

#### Scenario: Function continuation
- **WHEN** history contains an assistant function call followed by its matching
  tool result
- **THEN** the adapter maps the result to that function's name and call ID;
  an orphan result is refused.

#### Scenario: Structured response name and self-contained schema
- **WHEN** a supported JSON schema response format supplies its required name
- **THEN** the mapped provider response format preserves the name's presence;
  local references, unions and nested schema vocabulary are retained. External
  resource loading is disabled during compilation.

#### Scenario: Function with no parameters
- **WHEN** a function declaration omits parameters
- **THEN** the adapter supplies an empty object parameter schema.

#### Scenario: Prompt without content
- **WHEN** no message has non-empty text or a function call, such as a single
  user message with empty content
- **THEN** the request fails with `400 invalid_request_error` before model
  resolution; a prompt with any non-empty text or function call is mapped.
