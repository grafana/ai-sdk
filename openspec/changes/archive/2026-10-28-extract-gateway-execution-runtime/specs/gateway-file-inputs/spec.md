# Spec Delta

## MODIFIED Requirements

### Requirement: Scoped file options preserve semantic JSON
Supported message-level and ordinary file-part provider options, including nested tool-result file options, SHALL preserve namespace objects and opaque nested JSON at their original scope. No namespace SHALL be reserved at these scopes: `gateway`, `grafana` and `grafana-ai-sdk` SHALL reach native providers like any other namespace, because the host consumes only call-level `gateway` controls.

#### Scenario: Scoped opaque values survive
- **WHEN** supported message/file scopes carry ordinary provider options containing nested null, false, zero, empty strings, arrays, or objects
- **THEN** the model SHALL receive semantically identical values at the same scopes, including explicitly empty namespace objects

#### Scenario: File options remain available to native interpretation
- **WHEN** file-entry or function-tool options contain both the selected backend's namespace and an unrelated ordinary namespace or an unlisted ordinary field
- **THEN** all ordinary namespaces and fields SHALL reach the model at their original scopes without Gateway inventory filtering
- **AND** native consumption and concrete bypass protections SHALL determine behavior

#### Scenario: Host namespace is not forwarded
- **WHEN** a supported message or file scope includes a `gateway`, `grafana` or `grafana-ai-sdk` namespace
- **THEN** the model SHALL receive it unchanged at that scope, and it SHALL NOT be promoted to headers, call options or host controls
