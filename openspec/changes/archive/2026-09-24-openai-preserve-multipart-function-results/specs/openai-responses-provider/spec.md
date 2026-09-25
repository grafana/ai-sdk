## ADDED Requirements

### Requirement: Function result output conversion
The OpenAI Responses provider SHALL convert ordinary function tool results to `function_call_output` with the original `call_id` and existing result `caller`. Scalar text, error-text, JSON, error-JSON, and execution-denied outputs SHALL retain their current string and output-schema JSON-encoding rules. A scalar output-level cache breakpoint SHALL take precedence over the tool-result part breakpoint, both resolved under the active OpenAI/Azure provider-options namespace; either SHALL wrap the string as an `input_text` array element carrying `prompt_cache_breakpoint`. Without a selected breakpoint, scalar output SHALL remain a string. Content-array output SHALL instead preserve content order as typed `input_text`, `input_image`, and `input_file` elements, with cache breakpoints read from each content element's provider options rather than result/output-level scalar options.

#### Scenario: Scalar results retain cache precedence and schema encoding
- **WHEN** a function output is text, JSON, error-text, error-JSON, or execution-denied and both its output and tool-result part carry distinct active-namespace cache breakpoints
- **THEN** its `function_call_output.output` is an `input_text` array carrying the output-level breakpoint and the correctly encoded scalar text
- **AND** without an output-level breakpoint the result-part breakpoint applies, and without either breakpoint output remains a string
- **AND** a function tool with an output schema quotes text/error-text/denial as JSON string literals, while JSON/error-JSON remains JSON-serialized and denial without a reason uses the default reason

#### Scenario: Generic multipart text and file content
- **WHEN** a generic function result contains mixed text, image data/URL, and non-image file data/URL content elements with per-element cache options
- **THEN** output contains ordered `input_text` text, `input_image` data URI or `image_url` (including optional image detail), and `input_file` `filename` plus base64 `file_data` or `file_url` entries
- **AND** inline non-image data without a filename uses `data`
- **AND** each surviving element carries only its own active-namespace cache breakpoint, even when the result/output also has a scalar breakpoint

#### Scenario: Generic uploaded references resolve by active namespace
- **WHEN** a generic function result contains image and non-image file references with an entry for the active OpenAI or Azure provider namespace
- **THEN** the corresponding typed `input_image` and `input_file` elements carry that entry as `file_id` and preserve image detail and element cache breakpoint
- **AND** a reference missing the active provider entry fails conversion instead of falling back to another namespace or silently dropping the file

#### Scenario: Unsupported generic content is dropped with warning
- **WHEN** a generic multipart function result contains an unsupported content type or file data variant
- **THEN** each unsupported element is omitted with an `other` warning naming `unsupported tool content part type: <type>` or `unsupported tool content part type: file with data type: <type>` respectively
- **AND** supported elements remain in their original order under the same `call_id`

### Requirement: Custom tool result output conversion
The provider SHALL retain `custom_tool_call_output` and the original `call_id` for configured custom provider tools. Scalar text, error-text, JSON, error-JSON, and execution-denied outputs SHALL remain strings unless a cache breakpoint is selected from output-level options before tool-result part options in the active namespace; when selected, the output SHALL be an `input_text` array. Custom multipart text, inline file/image data, and file/image URLs SHALL retain typed content, image detail and per-content cache options. For custom multipart uploaded file references, the provider SHALL warn `unsupported custom tool content part type: file with data type: reference` and omit the reference content; it SHALL NOT claim or emit `file_id` support for custom outputs under the registered upstream baseline.

#### Scenario: Custom scalar breakpoint and output identity
- **WHEN** a custom tool result has a scalar output with output-level or result-part active-namespace cache breakpoint
- **THEN** `custom_tool_call_output` retains its `call_id` and wraps the scalar string in one `input_text` with output-level precedence
- **AND** without either breakpoint scalar output remains a string, without function output-schema quoting

#### Scenario: Custom multipart reference warns and drops
- **WHEN** custom multipart output contains a supported text/image/file data or URL element alongside an uploaded reference
- **THEN** supported elements remain typed and ordered with their individual cache breakpoints
- **AND** the uploaded reference is absent and an `other` warning has message `unsupported custom tool content part type: file with data type: reference`
- **AND** no custom reference is represented as `file_id`

### Requirement: Parallel wrapper function result serialization
Grouped internal parallel function-tool results SHALL retain their original wrapper `call_id`, child index order, and existing continuation behavior. Each child SHALL use ordinary function-result conversion before its output is serialized: scalar strings remain strings, while multipart typed arrays become JSON-serialized strings. The wrapper SHALL join child strings with newlines. A selected scalar child output/result-part breakpoint SHALL instead produce ordered `input_text` wrapper elements (newline-prefixed after the first), carrying only the corresponding scalar child breakpoint. Multipart child-level scalar breakpoints SHALL NOT apply; multipart content element breakpoints SHALL survive inside the serialized JSON string. Unsupported multipart items SHALL emit warnings without reordering other child output. Existing hosted-tool and invalid/incomplete parallel-group dispatch SHALL remain unchanged.

#### Scenario: Ordered multipart and scalar children
- **WHEN** a grouped parallel wrapper receives child results out of index order, including generic multipart output with an uploaded reference and a scalar child
- **THEN** the wrapper emits one `function_call_output` with the wrapper `call_id` and newline-joined child outputs in original child index order
- **AND** the multipart child output is JSON-serialized typed content with its active-namespace `file_id` and per-content cache hints rather than discarded or emitted as separate native blocks

#### Scenario: Scalar child breakpoints preserve their positions
- **WHEN** one or more grouped parallel children have selected scalar output/result-part cache breakpoints
- **THEN** the wrapper output is an ordered array of `input_text` child strings with later children newline-prefixed and each selected breakpoint on its corresponding element
- **AND** multipart child result/output-level cache options do not cause wrapper-level breakpoints
