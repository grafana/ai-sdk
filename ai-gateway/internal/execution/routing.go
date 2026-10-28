package execution

import "github.com/grafana/ai-sdk/fallback"

// Routing summarizes one invocation's overview by model, using a subset of the
// Vercel AI Gateway's documented providerMetadata.gateway.routing fields. It
// holds no fact that the overview lacks.
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

// projectRouting groups overview's attempts by the model groups expected at
// their positions. It returns nil when an attempt cannot be assigned.
func projectRouting(overview *Overview, groups []modelGroup) *Routing {
	slots, open, ok := groupSlots(groups)
	if !ok || (!open && len(overview.Attempts) > len(slots)) {
		return nil
	}
	routing := &Routing{
		OriginalModelID:           overview.RequestedModelID,
		CanonicalSlug:             overview.CanonicalModelID,
		TotalProviderAttemptCount: len(overview.Attempts),
	}
	group := -1
	for i, attempt := range overview.Attempts {
		index := 0
		if !open {
			index = slots[i].group
		}
		if index != group {
			group = index
			routing.ModelAttempts = append(routing.ModelAttempts, ModelAttempt{ModelID: groups[group].requested, CanonicalSlug: groups[group].canonical})
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
	if len(overview.Attempts) != 0 {
		routing.ResolvedProvider = overview.Attempts[0].Provider
	}
	return routing
}
