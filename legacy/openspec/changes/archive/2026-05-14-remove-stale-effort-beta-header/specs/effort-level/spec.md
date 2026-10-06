## REMOVED Requirements

### Requirement: Effort beta header
**Reason**: The `effort-2025-11-24` beta header was retired upstream when the extended-thinking `effort` parameter went GA on 2025-11-24. Direct Anthropic silently ignores the stale header, but Vertex AI's strict validator rejects requests carrying it with HTTP 400. Upstream removed it in `vercel/ai@e5c4f40` (anthropic canary.46).
**Migration**: No caller action required. The `output_config.effort` request body field continues to drive the feature end-to-end on direct Anthropic, Bedrock, and Vertex.

### Requirement: Reasoning effort beta header
**Reason**: Same as above — the beta header is no longer required (and is actively rejected by Vertex AI) regardless of whether `effort` is supplied via `AnthropicOptions.Effort` or derived from `CallOptions.Reasoning` on adaptive-capable models. The `interleaved-thinking-2025-05-14` beta header continues to be appended when reasoning enables thinking, as that feature is still beta.
**Migration**: No caller action required. Callers verifying request betas SHOULD assert that `effort-2025-11-24` is absent; assertions on `interleaved-thinking-2025-05-14` remain unchanged.

## ADDED Requirements

### Requirement: No effort beta header is appended
The Anthropic provider SHALL NOT append any beta header related to the `effort` parameter when `output_config.effort` is set on the request. This applies to both the provider-options path (`AnthropicOptions.Effort`) and the reasoning-mapping path (`CallOptions.Reasoning` on adaptive-thinking-capable models). The `output_config.effort` request body field alone drives the feature.

#### Scenario: No effort beta when AnthropicOptions.Effort is set
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"effort":"high"}`
- **THEN** the built request params SHALL contain `output_config.effort` set to `"high"`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`

#### Scenario: No effort beta when reasoning maps to effort on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"high"` and the model is `claude-sonnet-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"high"`
- **AND** the request betas SHALL include `interleaved-thinking-2025-05-14`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`

#### Scenario: No effort beta when reasoning xhigh maps to xhigh on Opus 4-7
- **WHEN** `CallOptions.Reasoning` is `"xhigh"` and the model is `claude-opus-4-7`
- **THEN** the request SHALL contain `output_config.effort` set to `"xhigh"`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`

#### Scenario: No effort beta when reasoning is budget-based
- **WHEN** `CallOptions.Reasoning` is `"high"` on a budget-based model (e.g., `claude-sonnet-4-5`)
- **THEN** the request betas SHALL include `interleaved-thinking-2025-05-14`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`
- **AND** no `output_config.effort` SHALL be present
