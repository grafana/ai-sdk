## Context

The `aisdk` root package and `provider` leaf package both define `RequestMetadata` and `ResponseMetadata` types. The root versions duplicate provider fields rather than composing them, leading to manual field-by-field copying in `streamtext.go` and a confusing type hierarchy. Additionally, `ConvertToModelMessages` carries an unused `context.Context` parameter inherited from early design but never consumed.

Current type relationship:
- `provider.RequestMetadata` has `Body json.RawMessage` -- `aisdk.RequestMetadata` is identical
- `provider.ResponseMetadata` has `ID`, `ModelID`, `Timestamp` -- `aisdk.ResponseMetadata` duplicates these and adds `Headers` and `Messages`
- `provider.GenerateResponse` already embeds `provider.ResponseMetadata` and adds `Headers` + `Body` -- the same embedding pattern we want for `aisdk.ResponseMetadata`

## Goals / Non-Goals

**Goals:**
- Eliminate `aisdk.RequestMetadata` -- single definition in `provider`
- Use struct embedding in `aisdk.ResponseMetadata` to compose rather than duplicate
- Remove dead `Messages` field from `aisdk.ResponseMetadata`
- Remove unused `context.Context` from `ConvertToModelMessages`
- Preserve identical wire format and runtime behavior

**Non-Goals:**
- Implementing `toResponseMessages()` upstream alignment (tracked separately if needed)
- Changing `provider.ResponseMetadata` or `provider.RequestMetadata` definitions
- Modifying `provider.GenerateResponse` or `provider.StreamResult` types

## Decisions

### 1. Remove `aisdk.RequestMetadata`, use `provider.RequestMetadata` directly

**Rationale**: Both definitions are byte-for-byte identical. There is no extension or additional fields in the aisdk version. Using the provider type directly eliminates duplication.

**Alternative considered**: Type alias (`type RequestMetadata = provider.RequestMetadata`). Rejected because it adds indirection for no benefit -- the type is only used in `StepResult.Request` and one assignment in `streamtext.go`. Direct use is clearer.

### 2. Embed `provider.ResponseMetadata` in `aisdk.ResponseMetadata`

The new shape:
```go
type ResponseMetadata struct {
    provider.ResponseMetadata
    Headers map[string]string `json:"headers,omitempty"`
}
```

**Rationale**: This mirrors how `provider.GenerateResponse` already embeds `provider.ResponseMetadata`. Field access (`resp.ID`, `resp.ModelID`, `resp.Timestamp`) remains unchanged through Go's promotion. The field-by-field copy in `streamtext.go` simplifies to assigning the embedded struct.

**Alternative considered**: Flatten and keep the aisdk struct independent. Rejected because it perpetuates the duplication the issue identifies, and the provider package is a dependency of aisdk anyway.

**JSON serialization**: Embedding preserves the same flat JSON shape via Go's standard marshal behavior. The `json` tags on `provider.ResponseMetadata` fields will be promoted and serialize identically.

### 3. Remove `Messages` field

**Rationale**: `ResponseMetadata.Messages` (type `[]provider.Message`, tag `json:"-"`) is never written or read in production code. The Go port handles multi-step message building through `appendToolResults()` rather than the upstream `toResponseMessages()` pattern. Dead code should be removed.

### 4. Drop `context.Context` from `ConvertToModelMessages`

New signature:
```go
func ConvertToModelMessages(messages []UIMessage, opts ...ConvertOptions) ([]provider.Message, error)
```

**Rationale**: The parameter is explicitly unused (`_ context.Context`). Go convention is to omit unused parameters. If context is needed in the future, adding it back is a straightforward signature change.

**Impact**: One production caller (`streamtext.go:212`) and test helpers in `convert_test.go`. All straightforward mechanical updates.

## Risks / Trade-offs

- **[Breaking public API]** Consumers using `aisdk.RequestMetadata` as a type must switch to `provider.RequestMetadata`. Consumers constructing `aisdk.ResponseMetadata` with struct literals must adjust for embedding. Consumers calling `ConvertToModelMessages` must drop the context argument. -> This is acceptable for a pre-1.0 library; these are type-level changes caught at compile time.
- **[Embedding promotes all fields]** If `provider.ResponseMetadata` gains new fields in the future, they automatically appear in `aisdk.ResponseMetadata`. -> This is the desired behavior -- the aisdk type is meant to be a superset.
- **[Removing Messages field]** If upstream alignment requires `toResponseMessages()` later, the field must be re-added. -> Acceptable; dead code shouldn't stay to anticipate potential future use (YAGNI). Re-adding is trivial.
