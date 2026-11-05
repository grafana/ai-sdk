## MODIFIED Requirements

### Requirement: Function result output conversion

Ordinary results SHALL use function_call_output with original call_id and existing result caller. Scalar error-text and error-JSON SHALL serialize as JSON objects with one error property containing the original text or JSON value, regardless of output schema. Text, JSON and execution-denied SHALL retain their existing string and output-schema encoding rules. Conversion SHALL NOT mutate caller inputs. Scalar output SHALL remain string unless a selected active OpenAI/Azure breakpoint wraps it in input_text with prompt_cache_breakpoint.

#### Scenario: Scalar results retain cache precedence and schema encoding
- **WHEN** a function output is text, JSON, error-text, error-JSON, or execution-denied and both its output and tool-result part carry distinct active-namespace cache breakpoints
- **THEN** its `function_call_output.output` is an `input_text` array carrying the output-level breakpoint and the correctly encoded scalar text
- **AND** without an output-level breakpoint the result-part breakpoint applies, and without either breakpoint output remains a string
- **AND** a function tool with an output schema quotes text/denial as JSON string literals, while successful JSON remains JSON-serialized and errors retain their JSON object envelope and denial without a reason uses the default reason

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

Configured custom results SHALL retain custom_tool_call_output and original call_id. Error-text and error-JSON SHALL serialize as JSON objects containing the original value under error, independent of output schemas and without mutating caller inputs. Scalar text/error-text/JSON/error-JSON/execution-denied SHALL stay strings unless a selected active-namespace breakpoint (output before result part) makes an input_text array. Multipart text, inline image/file data and image/file URLs SHALL retain typed content, image detail and per-content cache options.

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

Grouped parallel function results SHALL retain wrapper call_id, child index order and continuation behavior. Each child SHALL undergo ordinary result conversion, including JSON error envelopes for failed children: scalar strings stay strings, multipart typed arrays become JSON strings; wrapper SHALL join strings with newlines. Unsupported multipart items SHALL warn without reordering. Hosted-tool and invalid/incomplete group dispatch SHALL remain unchanged.

#### Scenario: Ordered multipart and scalar children
- **WHEN** a grouped parallel wrapper receives child results out of index order, including generic multipart output with an uploaded reference and a scalar child
- **THEN** the wrapper emits one `function_call_output` with the wrapper `call_id` and newline-joined child outputs in original child index order
- **AND** the multipart child output is JSON-serialized typed content with its active-namespace `file_id` and per-content cache hints rather than discarded or emitted as separate native blocks

#### Scenario: Scalar child breakpoints preserve their positions
- **WHEN** one or more grouped parallel children have selected scalar output/result-part cache breakpoints
- **THEN** the wrapper output is an ordered array of `input_text` child strings with later children newline-prefixed and each selected breakpoint on its corresponding element
- **AND** multipart child result/output-level cache options do not cause wrapper-level breakpoints

#### Scenario: Error values survive unary and streaming continuation
- **WHEN** function, custom or applicable parallel children return error-text or error-json, including empty text, null, scalar or structured JSON
- **THEN** unary and streaming requests contain serialized objects with an error property preserving the value
- **AND** successful text/JSON, execution-denied and multipart outputs retain their distinct behavior
- **AND** active OpenAI/Azure cache hints wrap the serialized error while inactive namespace hints are ignored
