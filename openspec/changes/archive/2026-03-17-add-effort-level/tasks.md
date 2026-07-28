## 1. Add Effort field to AnthropicOptions

- [x] 1.1 Add `Effort string` field with JSON tag `effort,omitempty` to `AnthropicOptions` in `anthropic/options.go`

## 2. Wire effort through request building

- [x] 2.1 In `applyProviderOptions` in `anthropic/convert_request.go`, set `p.OutputConfig.Effort` to `BetaOutputConfigEffort(ao.Effort)` when `ao.Effort` is non-empty
- [x] 2.2 Append `effort-2025-11-24` beta header via `appendBetaUnique` when effort is set

## 3. Tests

- [x] 3.1 Add test for effort passthrough: provider options with `{"effort":"high"}` results in `p.OutputConfig.Effort` being set
- [x] 3.2 Add test for effort beta header: verify `effort-2025-11-24` is present in betas when effort is set
- [x] 3.3 Add test for effort with adaptive thinking: both `thinking` and `output_config.effort` are set correctly
- [x] 3.4 Add test for no effort: verify `output_config` is not set when effort is omitted

## 4. Verify

- [x] 4.1 Run `make test` and confirm all tests pass
- [x] 4.2 Run `make vet` and confirm no lint issues
