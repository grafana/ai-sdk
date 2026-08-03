package anthropic

import (
	"regexp"
	"slices"
	"sort"
	"strings"
)

// directAnthropicModelIDs lists model ids from upstream @ai-sdk/anthropic's
// AnthropicModelId union. The list is advisory; New accepts any string.
var directAnthropicModelIDs = []string{
	"claude-3-haiku-20240307",
	"claude-fable-5",
	"claude-haiku-4-5",
	"claude-haiku-4-5-20251001",
	"claude-opus-4-0",
	"claude-opus-4-1",
	"claude-opus-4-1-20250805",
	"claude-opus-4-20250514",
	"claude-opus-4-5",
	"claude-opus-4-5-20251101",
	"claude-opus-4-6",
	"claude-opus-4-7",
	"claude-opus-4-8",
	"claude-opus-5",
	"claude-sonnet-4-0",
	"claude-sonnet-4-20250514",
	"claude-sonnet-4-5",
	"claude-sonnet-4-5-20250929",
	"claude-sonnet-4-6",
	"claude-sonnet-5",
}

// vertexModelMap maps direct Anthropic model IDs and short aliases used for
// Vertex resolution to their Vertex AI model IDs. Most models use a date-pinned
// format (e.g. @20250514) for stability; some newer models are served without a
// date suffix. The Vertex model set follows current Google Cloud docs; upstream
// Vercel does not expose a Vertex model list.
var vertexModelMap = map[string]string{
	"claude-3-5-haiku":           "claude-3-5-haiku@20241022",
	"claude-3-5-sonnet":          "claude-3-5-sonnet-v2@20241022",
	"claude-3-7-sonnet":          "claude-3-7-sonnet@20250219",
	"claude-3-haiku":             "claude-3-haiku@20240307",
	"claude-3-haiku-20240307":    "claude-3-haiku@20240307",
	"claude-3-opus":              "claude-3-opus@20240229",
	"claude-fable-5":             "claude-fable-5",
	"claude-haiku-4-5":           "claude-haiku-4-5@20251001",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5@20251001",
	"claude-opus-4":              "claude-opus-4@20250514",
	"claude-opus-4-0":            "claude-opus-4@20250514",
	"claude-opus-4-1":            "claude-opus-4-1@20250805",
	"claude-opus-4-1-20250805":   "claude-opus-4-1@20250805",
	"claude-opus-4-20250514":     "claude-opus-4@20250514",
	"claude-opus-4-5":            "claude-opus-4-5@20251101",
	"claude-opus-4-5-20251101":   "claude-opus-4-5@20251101",
	"claude-opus-4-6":            "claude-opus-4-6",
	"claude-opus-4-7":            "claude-opus-4-7",
	"claude-opus-4-8":            "claude-opus-4-8",
	"claude-sonnet-4":            "claude-sonnet-4@20250514",
	"claude-sonnet-4-0":          "claude-sonnet-4@20250514",
	"claude-sonnet-4-20250514":   "claude-sonnet-4@20250514",
	"claude-sonnet-4-5":          "claude-sonnet-4-5@20250929",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5@20250929",
	"claude-sonnet-4-6":          "claude-sonnet-4-6",
	"claude-sonnet-5":            "claude-sonnet-5",
}

// ModelIDs returns the curated list of model IDs accepted by the direct
// Anthropic API. Source:
// https://docs.claude.com/en/docs/about-claude/models/overview.
//
// The list is advisory: [New] accepts arbitrary strings. Returned slice is
// sorted and safe to mutate.
func ModelIDs() []string {
	ids := slices.Clone(directAnthropicModelIDs)
	sort.Strings(ids)
	return ids
}

// VertexModelIDs returns the curated list of Anthropic model IDs accepted by the
// Vertex AI partner channel, in the resolved Vertex form used by [NewVertex].
// Source:
// https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude.
//
// The list is advisory: [NewVertex] accepts arbitrary strings. Returned slice is
// sorted and safe to mutate.
func VertexModelIDs() []string {
	seen := make(map[string]struct{}, len(vertexModelMap))
	ids := make([]string, 0, len(vertexModelMap))
	for _, id := range vertexModelMap {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// DualAvailableModelIDs returns model IDs in the form accepted by [New] that are
// present on both the direct Anthropic API and the Vertex AI partner channel.
//
// Use this when constructing fallback chains with [New] and [NewVertex].
// Returned slice is sorted and safe to mutate.
func DualAvailableModelIDs() []string {
	ids := make([]string, 0, len(directAnthropicModelIDs))
	for _, id := range directAnthropicModelIDs {
		if _, ok := vertexModelMap[id]; ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// ResolveVertexModelID maps a model ID accepted by [New] to the canonical model
// ID form expected by Vertex AI partner-channel requests through [NewVertex].
// Known models get curated Vertex IDs, date-pinned where applicable; already
// pinned IDs are returned unchanged; unknown unpinned models get @latest
// appended.
func ResolveVertexModelID(modelID string) string {
	if mapped, ok := vertexModelMap[modelID]; ok {
		return mapped
	}
	if strings.Contains(modelID, "@") {
		return modelID
	}
	return modelID + "@latest"
}

// modelCapabilities holds per-model limits and feature flags used when building
// Anthropic requests. It mirrors @ai-sdk/anthropic getModelCapabilities.
type modelCapabilities struct {
	maxOutputTokens                        int
	supportsAdaptiveThinking               bool
	supportsStructuredOutput               bool
	rejectsSamplingParams                  bool
	supportsXHighEffort                    bool
	rejectsThinkingDisabledAboveHighEffort bool
	isKnownModel                           bool
}

var legacyClaudeModelPattern = regexp.MustCompile(`claude-(instant($|-)|v?2($|[-.:])|3($|[-.]))`)

// getModelCapabilities returns metadata for model ID string matching against
// known Claude families (substring checks, most specific first).
func getModelCapabilities(modelID string) modelCapabilities {
	switch {
	case strings.Contains(modelID, "claude-opus-5"):
		return modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, rejectsThinkingDisabledAboveHighEffort: true, isKnownModel: true}
	case strings.Contains(modelID, "claude-opus-4-8") ||
		strings.Contains(modelID, "claude-opus-4-7") ||
		strings.Contains(modelID, "claude-fable-5") ||
		strings.Contains(modelID, "claude-sonnet-5"):
		return modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, isKnownModel: true}
	case strings.Contains(modelID, "claude-sonnet-4-6") || strings.Contains(modelID, "claude-opus-4-6"):
		return modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, isKnownModel: true}
	case strings.Contains(modelID, "claude-sonnet-4-5") ||
		strings.Contains(modelID, "claude-opus-4-5") ||
		strings.Contains(modelID, "claude-haiku-4-5"):
		return modelCapabilities{maxOutputTokens: 64000, supportsStructuredOutput: true, isKnownModel: true}
	case strings.Contains(modelID, "claude-opus-4-1"):
		return modelCapabilities{maxOutputTokens: 32000, supportsStructuredOutput: true, isKnownModel: true}
	case strings.Contains(modelID, "claude-sonnet-4-"):
		return modelCapabilities{maxOutputTokens: 64000, isKnownModel: true}
	case strings.Contains(modelID, "claude-opus-4-"):
		return modelCapabilities{maxOutputTokens: 32000, isKnownModel: true}
	case strings.Contains(modelID, "claude-3-haiku"):
		return modelCapabilities{maxOutputTokens: 4096, isKnownModel: true}
	case legacyClaudeModelPattern.MatchString(modelID):
		return modelCapabilities{maxOutputTokens: 4096}
	case strings.Contains(modelID, "claude-"):
		return modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, rejectsThinkingDisabledAboveHighEffort: true}
	default:
		return modelCapabilities{maxOutputTokens: 4096}
	}
}
