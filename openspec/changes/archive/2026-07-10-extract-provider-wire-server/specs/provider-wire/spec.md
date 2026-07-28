## MODIFIED Requirements

### Requirement: Wire package location and scope

The repository SHALL define a `gateway/providerwire/` Go package that owns the complete JSON+HTTP/SSE transport for the remote `provider.LanguageModel` protocol. The package SHALL contain the route/header constants, request/response envelopes, SSE stream-part encoding/decoding, error-envelope helpers, and reusable server handler. It MUST NOT contain protobuf, Connect, or other binary-format machinery. The former `provider/wire/` package SHALL be deleted, and the repository MUST NOT provide aliases, compatibility re-exports, or a forwarding shim at its old import path. Moving the exported helpers is an intentional source-breaking import-path change; canonical encoded bytes and protocol shapes SHALL remain unchanged, except that the SSE reader SHALL apply the explicitly specified final-line EOF correction.

#### Scenario: Package import path
- **WHEN** a Go file in this repository or in `providers/grafana/` imports the wire helpers
- **THEN** it SHALL import `github.com/grafana/ai-sdk/gateway/providerwire`

#### Scenario: No protobuf machinery
- **WHEN** the `gateway/providerwire/` directory is inspected
- **THEN** it SHALL NOT contain `.proto` files, generated `.pb.go` files, `wirepb/`, `wirepbconnect/`, `buf.gen.yaml`, or any Connect-related artifacts

#### Scenario: Old package is deleted without compatibility
- **WHEN** the repository is inspected after the move
- **THEN** `provider/wire/` SHALL not exist and no package SHALL alias or re-export `gateway/providerwire` symbols from the old path

#### Scenario: Canonical wire output remains stable across the source break
- **WHEN** existing request, response, error-envelope, or SSE values are encoded through `gateway/providerwire`
- **THEN** their encoded bytes and protocol shapes SHALL match the former `provider/wire` implementation

### Requirement: Auth metadata is provider-side, not wire-side

The wire package SHALL NOT define authentication helpers, token exchange, or auth-header constants. Headers such as `Authorization` and `X-Grafana-Id` are responsibility of the provider implementation that uses the wire package (e.g. `providers/grafana/`).

#### Scenario: Wire package has no auth helpers
- **WHEN** the `gateway/providerwire/` package is inspected
- **THEN** no symbol SHALL relate to auth, tokens, CAP, or `X-Grafana-Id`

### Requirement: Wire package has no orchestration knowledge

The wire package SHALL NOT depend on the root `aisdk` orchestration package, on `@ai-sdk/react`-style UI message chunks, or on tool-execution machinery. It SHALL depend only on the standard library and the transport-agnostic `provider/` package.

#### Scenario: Dependency boundary
- **WHEN** the wire package's `import` statements are inspected
- **THEN** no import SHALL be from `github.com/grafana/ai-sdk/aisdk` or any other orchestration-layer package

### Requirement: SSE encoder/decoder helpers

The `gateway/providerwire` package SHALL export `WriteSSEStreamPart(w io.Writer, part provider.StreamPart) error` for the server side and `NewSSEReader(r io.Reader)` with `SSEReader.Next()` for the client side. Each SHALL handle exactly one event boundary per call and SHALL round-trip every `provider.StreamPartType` losslessly. When the underlying reader returns both non-empty final-line bytes and `io.EOF`, `Next` SHALL process those bytes before deciding whether the stream ended cleanly, so a valid unterminated final event is decoded and an invalid one returns a decode error rather than a false clean EOF.

#### Scenario: Round-trip every StreamPartType

- **WHEN** every defined `provider.StreamPartType` value is encoded via `WriteSSEStreamPart` and then decoded via `SSEReader.Next`
- **THEN** the decoded `StreamPart` SHALL equal the original for every type

#### Scenario: Round-trip APICallError on PartError

- **WHEN** a `StreamPart{Type: PartError, APICallError: &APICallError{StatusCode: 500, IsRetryable: true, Message: "boom"}}` is encoded and decoded
- **THEN** the decoded part SHALL have `Type == PartError`, a non-nil `APICallError`, `StatusCode == 500`, `IsRetryable == true`, and `Message == "boom"`

#### Scenario: Unterminated final data line is decoded

- **WHEN** the final SSE event has a valid `data:` line but no trailing newline or blank event boundary
- **THEN** `SSEReader.Next` SHALL decode and return that final `provider.StreamPart`

#### Scenario: Unterminated multiline final event is decoded

- **WHEN** an SSE event has multiple `data:` lines and its final line has no trailing newline
- **THEN** `SSEReader.Next` SHALL join all data lines and decode the final `provider.StreamPart`

#### Scenario: Invalid unterminated final event is observable

- **WHEN** the final `data:` line has no trailing newline and contains invalid JSON
- **THEN** `SSEReader.Next` SHALL return a decoding error rather than `io.EOF`

### Requirement: Wire round-trip test suite

The `gateway/providerwire` package SHALL include the moved wire test suite that exercises round-trip serialization for every defined `provider.StreamPartType` value, every `ContentPartType` value, every `ToolType` value, every notable `CallOptions` field, and every `APICallError` field. Tests MUST use white-box JSON+SSE encoding and decoding through the public encode/decode helpers, and their existing byte expectations SHALL remain unchanged by the package move.

#### Scenario: Per-StreamPartType round-trip
- **WHEN** the test suite runs
- **THEN** at least one assertion SHALL exist per defined `provider.StreamPartType` value confirming JSON round-trip with no field loss

#### Scenario: Per-ContentPartType round-trip
- **WHEN** the test suite runs
- **THEN** at least one assertion SHALL exist per defined `ContentPartType` value confirming JSON round-trip with no field loss

#### Scenario: Per-CallOptions-field round-trip
- **WHEN** the test suite runs
- **THEN** every notable `provider.CallOptions` field (`Prompt`, `Tools`, `ToolChoice`, `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `StopSequences`, `ResponseFormat`, `Seed`, `Reasoning`, `IncludeRawChunks`, `Headers`, `ProviderOptions`) SHALL have at least one assertion confirming wire round-trip
