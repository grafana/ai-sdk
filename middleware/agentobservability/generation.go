package agentobservability

import (
	"context"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
)

const (
	transportProviderMetadataKey = "ai_sdk.transport.provider"
	transportModelMetadataKey    = "ai_sdk.transport.model"
)

type generationModelIdentity struct {
	provider string
	model    string
}

// BuildGenerationStart assembles the agento11y.GenerationStart for a single
// model call. The middleware passes this to client.StartGeneration or
// client.StartStreamingGeneration before invoking the inner model.
//
// Field resolution order (highest precedence first):
//
//  1. The context-key DAG helpers from this package (WithGenerationID,
//     WithParentGenerationIDs).
//  2. The consumer-supplied ContextInfo.
//  3. The upstream agento11y.*FromContext helpers (UserIDFromContext,
//     AgentNameFromContext, AgentVersionFromContext).
//
// The Agent Observability client itself fills in any remaining defaults (StartedAt,
// OperationName for stream vs sync, etc.) when it processes the start.
func BuildGenerationStart(ctx context.Context, providerName, modelID string, ctxInfo ContextInfo) agento11y.GenerationStart {
	start := agento11y.GenerationStart{
		Model: agento11y.ModelRef{
			Provider: providerName,
			Name:     modelID,
		},
		ID:                  GenerationIDFromContext(ctx),
		ParentGenerationIDs: ParentGenerationIDsFromContext(ctx),
		UserID:              ctxInfo.UserID,
		AgentName:           ctxInfo.AgentName,
		AgentVersion:        ctxInfo.AgentVersion,
		Metadata:            cloneMetadataMap(ctxInfo.Metadata),
		Tags:                cloneStringMap(ctxInfo.Tags),
	}

	// Falling back to the agento11y.*FromContext helpers when the consumer
	// didn't supply a value preserves compatibility with consumers that wire
	// in user/agent information via the upstream agento11y context keys directly.
	if start.UserID == "" {
		if userID, ok := agento11y.UserIDFromContext(ctx); ok {
			start.UserID = userID
		}
	}
	if start.AgentName == "" {
		if name, ok := agento11y.AgentNameFromContext(ctx); ok {
			start.AgentName = name
		}
	}
	if start.AgentVersion == "" {
		if version, ok := agento11y.AgentVersionFromContext(ctx); ok {
			start.AgentVersion = version
		}
	}
	return start
}

// MapGenerateResult composes the request mapper, response mapper, and
// metadata derivation into a complete agento11y.Generation. The recorder fills
// in id / started_at / completed_at / trace_id / span_id; everything else on
// the returned struct is final.
func MapGenerateResult(params provider.CallOptions, result *provider.GenerateResult, ctxInfo ContextInfo) agento11y.Generation {
	return mapGenerateResultWithStart(params, result, ctxInfo, agento11y.GenerationStart{})
}

func mapGenerateResultWithStart(params provider.CallOptions, result *provider.GenerateResult, ctxInfo ContextInfo, start agento11y.GenerationStart) agento11y.Generation {
	system, input := messagesToAgento11y(params.Prompt)
	controls := controlsFromCallOptions(params)

	generation := agento11y.Generation{
		UserID:              ctxInfo.UserID,
		AgentName:           ctxInfo.AgentName,
		AgentVersion:        ctxInfo.AgentVersion,
		SystemPrompt:        system,
		Input:               input,
		Tools:               toolsToAgento11y(params.Tools),
		MaxTokens:           controls.MaxTokens,
		Temperature:         controls.Temperature,
		TopP:                controls.TopP,
		ToolChoice:          controls.ToolChoice,
		ThinkingEnabled:     thinkingEnabledFromAnthropic(params.ProviderOptions),
		ParentGenerationIDs: nil, // filled by recorder seed
		Tags:                cloneStringMap(ctxInfo.Tags),
		Metadata:            mergeMetadata(metadataFromProviderOptions(params), ctxInfo.Metadata),
	}

	if result != nil {
		generation.Output = contentToAgento11yOutput(result.Content)
		generation.Usage = usageToAgento11y(result.Usage)
		generation.StopReason = finishReasonToAgento11yStop(result.FinishReason)
		if result.Response != nil {
			generation.ResponseID = result.Response.ID
			if result.Response.ModelID != "" {
				generation.ResponseModel = result.Response.ModelID
			}
			applyModelIdentity(&generation, modelIdentityFromStart(start), modelIdentityFromResponse(result.Response.Provider, result.Response.ModelID))
		}
	}
	return generation
}

func modelIdentityFromStart(start agento11y.GenerationStart) generationModelIdentity {
	return generationModelIdentity{provider: start.Model.Provider, model: start.Model.Name}
}

func modelIdentityFromResponse(providerName, modelID string) generationModelIdentity {
	return generationModelIdentity{provider: providerName, model: modelID}
}

func (m generationModelIdentity) complete() bool {
	return m.provider != "" && m.model != ""
}

func applyModelIdentity(gen *agento11y.Generation, seed, response generationModelIdentity) {
	if gen == nil {
		return
	}
	final := seed
	responseComplete := response.complete()
	if responseComplete {
		final = response
	}
	if final.complete() {
		gen.Model = agento11y.ModelRef{Provider: final.provider, Name: final.model}
	}
	if responseComplete && seed.complete() && seed != response {
		addTransportMetadata(gen, seed)
	}
}

func addTransportMetadata(gen *agento11y.Generation, seed generationModelIdentity) {
	if gen.Metadata == nil {
		gen.Metadata = map[string]any{}
	}
	if _, ok := gen.Metadata[transportProviderMetadataKey]; !ok {
		gen.Metadata[transportProviderMetadataKey] = seed.provider
	}
	if _, ok := gen.Metadata[transportModelMetadataKey]; !ok {
		gen.Metadata[transportModelMetadataKey] = seed.model
	}
}

// mergeMetadata returns the union of two metadata maps. Override entries win
// on key conflict. Returns nil when both inputs are empty.
func mergeMetadata(base, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func cloneMetadataMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
