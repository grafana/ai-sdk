## 1. Provider types

- [x] 1.1 Add `Provider string` (json:"provider,omitempty") to `provider.ResponseMetadata` in `provider/language_model.go`
- [x] 1.2 Add `Provider string` (json:"provider,omitempty") to `provider.StreamPart` in `provider/stream_part.go`, near `ResponseID`/`ModelID`

## 2. Anthropic provider population

- [x] 2.1 Thread the model's `providerName` into `convertResponse` (`providers/anthropic/model.go` -> `convert_response.go`) and set `GenerateResponse.Provider`
- [x] 2.2 Thread the model's `providerName` into `consumeStream`/`streamAdapter` (`providers/anthropic/model.go` -> `convert_stream.go`) and set `Provider` on the `PartResponseMeta` emitted for `message_start`

## 3. Orchestration propagation

- [x] 3.1 In `streamtext.go` `PartResponseMeta` handling, copy `part.Provider` into the `ResponseMetadata{...}` so `StreamTextResult.Response()` and step results expose it

## 4. Tests

- [x] 4.1 Anthropic: assert `GenerateResult.Response.Provider` is set for both direct (`anthropic`) and Vertex (`anthropic.vertex`) models
- [x] 4.2 Anthropic: assert the streamed `PartResponseMeta.Provider` is set
- [x] 4.3 Provider-wire: round-trip a `PartResponseMeta` with `Provider` set; assert `reflect.DeepEqual`; assert empty provider omitted
- [x] 4.4 Fallback: assert that when a non-primary candidate serves the request, the response/stream metadata carries the serving candidate's provider

## 5. Verify

- [x] 5.1 `make fmt vet test` (root + providers/anthropic) green
- [x] 5.2 `openspec validate response-metadata-provider --strict` passes
