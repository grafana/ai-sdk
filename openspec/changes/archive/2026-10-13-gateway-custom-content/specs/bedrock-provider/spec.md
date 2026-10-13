# Spec Delta

## ADDED Requirements

### Requirement: Custom tool-result content rejection
When tools remain active after tool preparation, the Bedrock Converse adapter SHALL reject user- or tool-role tool-result content containing a custom entry before any HTTP request. The error SHALL wrap the provider unsupported-functionality sentinel and name the unsupported tool content type, instead of dropping the entry with a warning. Bedrock Mantle SHALL keep its existing warning.

#### Scenario: Custom tool-result entry
- **WHEN** DoGenerate or DoStream with active tools receives tool-result content with text and custom entries
- **THEN** the call SHALL fail with zero HTTP requests and the error SHALL satisfy errors.Is for the unsupported-functionality sentinel

#### Scenario: No active tools
- **WHEN** a request has no tools, or its tools are removed by `toolChoice: none`, and its tool results contain a custom entry
- **THEN** the tool content SHALL be filtered as today without rejection, matching upstream filtering before conversion

#### Scenario: Other unsupported content keeps existing behavior
- **WHEN** tool-result content contains only text and file entries, including file entries the adapter already warns about
- **THEN** the existing conversion and warnings SHALL remain unchanged

#### Scenario: Mantle keeps its warning
- **WHEN** a Bedrock Mantle Responses model receives tool-result content with a custom entry
- **THEN** it SHALL keep the existing `unsupported tool content part type: custom` warning rather than rejecting

### Requirement: Existing unsupported conversion errors carry the sentinel
Bedrock Converse prompt-conversion errors whose registered upstream counterpart throws UnsupportedFunctionalityError SHALL wrap the provider unsupported-functionality sentinel, keeping their existing messages and pre-I/O timing. This covers provider-reference file parts, non-S3 file URLs, and unsupported image, video and file media types. Existing warnings SHALL remain warnings.

#### Scenario: Unsupported user file data source
- **WHEN** a user file part carries a non-S3 URL or a provider reference, or an unsupported image, video or document media type
- **THEN** conversion SHALL fail before any HTTP request with an error satisfying errors.Is for the sentinel and the existing message

#### Scenario: Warning sites unchanged
- **WHEN** a request hits an existing Bedrock warning, such as separated system messages or unsupported tool-result file data
- **THEN** the adapter SHALL keep warning and SHALL NOT return an error
