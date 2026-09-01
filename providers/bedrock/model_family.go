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

var openAIModelPattern = regexp.MustCompile(`^(?:[^.]+\.)?(openai\..+)$`)

// isOpenAIModel returns true when the Bedrock model ID refers to an OpenAI
// model on Bedrock (e.g. `openai.gpt-oss-...`). Cross-region prefixes
// (`us.openai.`) also match.
func isOpenAIModel(modelID string) bool {
	return openAIModelPattern.MatchString(modelID)
}

func isOpenAIGPTOSSModel(modelID string) bool {
	matches := openAIModelPattern.FindStringSubmatch(modelID)
	return len(matches) == 2 && strings.HasPrefix(matches[1], "openai.gpt-oss-")
}

// isMistralModel returns true when the Bedrock model ID refers to a Mistral
// model on Bedrock (e.g. `mistral.mistral-large-2407-v1:0`). Cross-region
// prefixes (`us.mistral.`) also match. Mistral models require numeric-only
// 9-char tool call IDs (see normalize_tool_call_id.go).
func isMistralModel(modelID string) bool {
	return strings.Contains(modelID, "mistral.")
}

var legacyClaudeModelPattern = regexp.MustCompile(`claude-(instant($|-)|v?2($|[-.:])|3($|[-.]))`)

// supportsNativeStructuredOutput returns true when the model supports
// Converse `additionalModelRequestFields.output_config.format` for JSON
// schema output. We approximate upstream's `getModelCapabilities` here by
// detecting recent Anthropic models. The upstream list is fluid; we err on
// the side of fallback (synthetic json tool) when uncertain.
func supportsNativeStructuredOutput(modelID string) bool {
	if !isAnthropicModel(modelID) || rejectsNativeStructuredOutput(modelID) {
		return false
	}
	// Claude Sonnet 4 / 4.5, Opus 4 / 4.5, Haiku 4 / 4.5 support native JSON
	// schema output. Conservative substring match keeps the implementation
	// simple while accepting Bedrock cross-region prefixes.
	for _, marker := range []string{
		"claude-sonnet-4",
		"claude-opus-4",
		"claude-haiku-4",
	} {
		if strings.Contains(modelID, marker) {
			return true
		}
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
	return rejectsNewerSchemaFields(modelID)
}

func usesJSONInstructionForStructuredOutput(modelID string) bool {
	return rejectsNewerSchemaFields(modelID)
}

type anthropicReasoningCapabilities struct {
	maxOutputTokens          int
	supportsAdaptiveThinking bool
}

func getAnthropicReasoningCapabilities(modelID string) anthropicReasoningCapabilities {
	if !isAnthropicModel(modelID) {
		return anthropicReasoningCapabilities{maxOutputTokens: 4096}
	}
	switch {
	case strings.Contains(modelID, "claude-opus-4-8"),
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
