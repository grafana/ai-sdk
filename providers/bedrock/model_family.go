package bedrock

import (
	"regexp"
	"strings"
)

// isAnthropicModel returns true when the Bedrock model ID refers to an
// Anthropic-hosted model. Bedrock prefixes Anthropic model IDs with
// `anthropic.` (e.g. `anthropic.claude-sonnet-4-5-20250929-v1:0`) or carries
// a cross-region prefix like `us.anthropic.`.
func isAnthropicModel(modelID string) bool {
	return strings.Contains(modelID, "anthropic")
}

func isAnthropicModelForCall(modelID string, family ModelFamily, hasBudgetTokens bool) bool {
	return family == ModelFamilyAnthropic || isAnthropicModel(modelID) ||
		(strings.Contains(modelID, ":application-inference-profile/") && hasBudgetTokens)
}

// isOpenAIModel returns true when the Bedrock model ID refers to an OpenAI
// model on Bedrock (e.g. `openai.gpt-oss-...`). Cross-region prefixes
// (`us.openai.`) also match.
var openAIModelPattern = regexp.MustCompile(`^(?:[^.]+\.)?openai\..+$`)
var openAIGptOSSModelPattern = regexp.MustCompile(`^(?:[^.]+\.)?openai\.gpt-oss-.+$`)

func isOpenAIModel(modelID string) bool {
	return openAIModelPattern.MatchString(modelID)
}

func isOpenAIGptOSSModel(modelID string) bool {
	return openAIGptOSSModelPattern.MatchString(modelID)
}

// isMistralModel returns true when the Bedrock model ID refers to a Mistral
// model on Bedrock (e.g. `mistral.mistral-large-2407-v1:0`). Cross-region
// prefixes (`us.mistral.`) also match. Mistral models require numeric-only
// 9-char tool call IDs (see normalize_tool_call_id.go).
func isMistralModel(modelID string) bool {
	return strings.Contains(modelID, "mistral.")
}

var legacyClaudeModelPattern = regexp.MustCompile(`claude-(instant($|-)|v?2($|[-.:])|3($|[-.]))`)
var olderClaude4ModelPattern = regexp.MustCompile(`claude-(sonnet|opus)-4(-|@)`)

func supportsStructuredOutputCapability(modelID string) bool {
	if rejectsNewerSchemaFields(modelID) {
		return true
	}
	for _, marker := range []string{
		"claude-sonnet-4-5",
		"claude-sonnet-4-6",
		"claude-opus-4-1",
		"claude-opus-4-5",
		"claude-opus-4-6",
		"claude-haiku-4-5",
	} {
		if strings.Contains(modelID, marker) {
			return true
		}
	}
	if olderClaude4ModelPattern.MatchString(modelID) {
		return false
	}
	return strings.Contains(modelID, "claude-") && !legacyClaudeModelPattern.MatchString(modelID)
}

var modelsRejectingNewerSchemaFields = []string{
	"claude-opus-4-7",
	"claude-opus-4-8",
	"claude-opus-5",
	"claude-fable-5",
	"claude-sonnet-5",
}

func rejectsNewerSchemaFields(modelID string) bool {
	for _, marker := range modelsRejectingNewerSchemaFields {
		if strings.Contains(modelID, marker) {
			return true
		}
	}
	return false
}

func rejectsNativeStructuredOutput(modelID string) bool {
	return rejectsNewerSchemaFields(modelID) || strings.Contains(modelID, "claude-sonnet-4-6") || strings.Contains(modelID, "claude-haiku-4-5")
}

func usesJSONInstructionForStructuredOutput(modelID string) bool {
	return rejectsNewerSchemaFields(modelID)
}

type anthropicReasoningCapabilities struct {
	maxOutputTokens          int
	supportsAdaptiveThinking bool
}

func getAnthropicReasoningCapabilities(modelID string) anthropicReasoningCapabilities {
	switch {
	case strings.Contains(modelID, "claude-opus-5"),
		strings.Contains(modelID, "claude-opus-4-8"),
		strings.Contains(modelID, "claude-opus-4-7"),
		strings.Contains(modelID, "claude-fable-5"),
		strings.Contains(modelID, "claude-sonnet-5"),
		strings.Contains(modelID, "claude-sonnet-4-6"),
		strings.Contains(modelID, "claude-opus-4-6"):
		return anthropicReasoningCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true}
	case strings.Contains(modelID, "claude-sonnet-4-5"),
		strings.Contains(modelID, "claude-opus-4-5"),
		strings.Contains(modelID, "claude-haiku-4-5"):
		return anthropicReasoningCapabilities{maxOutputTokens: 64000}
	case strings.Contains(modelID, "claude-opus-4-1"):
		return anthropicReasoningCapabilities{maxOutputTokens: 32000}
	case strings.Contains(modelID, "claude-sonnet-4-"):
		return anthropicReasoningCapabilities{maxOutputTokens: 64000}
	case strings.Contains(modelID, "claude-opus-4-"):
		return anthropicReasoningCapabilities{maxOutputTokens: 32000}
	case legacyClaudeModelPattern.MatchString(modelID):
		return anthropicReasoningCapabilities{maxOutputTokens: 4096}
	case strings.Contains(modelID, "claude-"):
		return anthropicReasoningCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true}
	default:
		return anthropicReasoningCapabilities{maxOutputTokens: 4096}
	}
}

func supportsAdaptiveThinking(modelID string) bool {
	return getAnthropicReasoningCapabilities(modelID).supportsAdaptiveThinking
}

func anthropicReasoningMaxOutputTokens(modelID string) int {
	return getAnthropicReasoningCapabilities(modelID).maxOutputTokens
}
