package execution

import "github.com/grafana/ai-sdk/fallback"

// Routing summarizes one invocation's overview by model, using a subset of the
// Vercel AI Gateway's documented providerMetadata.gateway.routing fields. It
// adds no attempt or outcome that the overview lacks, only the model each
// attempt belongs to.
type Routing struct {
	// OriginalModelID is the model ID the call requested.
	OriginalModelID string `json:"originalModelId"`
	// CanonicalSlug is the requested model's canonical ID, absent without
	// catalog resolution.
	CanonicalSlug string `json:"canonicalSlug,omitempty"`
	// ResolvedProvider is the provider of the first attempt.
	ResolvedProvider string `json:"resolvedProvider"`
	// FinalProvider is the provider of the selected attempt, absent when none
	// was selected.
	FinalProvider             string         `json:"finalProvider,omitempty"`
	ModelAttemptCount         int            `json:"modelAttemptCount"`
	TotalProviderAttemptCount int            `json:"totalProviderAttemptCount"`
	ModelAttempts             []ModelAttempt `json:"modelAttempts"`
}

// ModelAttempt is one model with at least one observed attempt.
type ModelAttempt struct {
	// ModelID is the requested ID or models entry as sent.
	ModelID string `json:"modelId"`
	// CanonicalSlug is the model's canonical ID, absent without catalog
	// resolution.
	CanonicalSlug        string            `json:"canonicalSlug,omitempty"`
	Success              bool              `json:"success"`
	ProviderAttemptCount int               `json:"providerAttemptCount"`
	ProviderAttempts     []ProviderAttempt `json:"providerAttempts"`
}

// ProviderAttempt is one observed attempt. Success means the attempt was
// selected, not that a stream completed.
type ProviderAttempt struct {
	Provider   string `json:"provider"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode,omitempty"`
}

// projectRouting groups overview's attempts by the model whose chain each
// belongs to.
func projectRouting(overview *Overview) *Routing {
	routing := &Routing{
		OriginalModelID:           overview.RequestedModelID,
		CanonicalSlug:             overview.CanonicalModelID,
		TotalProviderAttemptCount: len(overview.Attempts),
	}
	for i, attempt := range overview.Attempts {
		modelID, canonical := overview.RequestedModelID, overview.CanonicalModelID
		if attempt.fallbackModel != "" {
			modelID, canonical = attempt.fallbackModel, attempt.fallbackCanonical
		}
		if i == 0 {
			routing.ResolvedProvider = attempt.Provider
		}
		if len(routing.ModelAttempts) == 0 || routing.ModelAttempts[len(routing.ModelAttempts)-1].ModelID != modelID {
			routing.ModelAttempts = append(routing.ModelAttempts, ModelAttempt{ModelID: modelID, CanonicalSlug: canonical})
		}
		selected := attempt.Outcome == fallback.AttemptSelected
		providerAttempt := ProviderAttempt{Provider: attempt.Provider, Success: selected}
		if attempt.Error != nil {
			providerAttempt.StatusCode = attempt.Error.StatusCode
		}
		model := &routing.ModelAttempts[len(routing.ModelAttempts)-1]
		model.ProviderAttempts = append(model.ProviderAttempts, providerAttempt)
		model.ProviderAttemptCount++
		model.Success = model.Success || selected
		if selected {
			routing.FinalProvider = attempt.Provider
		}
	}
	routing.ModelAttemptCount = len(routing.ModelAttempts)
	return routing
}
