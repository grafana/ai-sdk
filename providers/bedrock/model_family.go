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

// isNova2ReasoningModel returns true when the Bedrock model ID refers to Amazon
// Nova 2 Lite, including regional and inference-profile prefixes.
func isNova2ReasoningModel(modelID string) bool {
	return strings.Contains(modelID, "amazon.nova-2-lite-v1:0")
}

// isMistralModel returns true when the Bedrock model ID refers to a Mistral
// model on Bedrock (e.g. `mistral.mistral-large-2407-v1:0`). Cross-region
// prefixes (`us.mistral.`) also match. Mistral models require
// 9-character alphanumeric tool call IDs (see normalize_tool_call_id.go).
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

func rejectsSamplingParameters(modelID string) bool {
	if rejectsNewerSchemaFields(modelID) || strings.Contains(modelID, "claude-opus-5-5") || strings.Contains(modelID, "claude-fable-5-1") {
		return true
	}
	if !strings.Contains(modelID, "claude-") || legacyClaudeModelPattern.MatchString(modelID) {
		return false
	}
	for _, marker := range []string{"claude-sonnet-4-5", "claude-sonnet-4-6", "claude-opus-4-1", "claude-opus-4-5", "claude-opus-4-6", "claude-haiku-4-5"} {
		if strings.Contains(modelID, marker) {
			return false
		}
	}
	return !olderClaude4ModelPattern.MatchString(modelID)
}

var knownClaudeModelMarkers = []string{
	"claude-sonnet-5",
	"claude-opus-5",
	"claude-haiku-5-5",
	"claude-fable-5",
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-sonnet-4-6",
	"claude-opus-4-6",
	"claude-sonnet-4-5",
	"claude-opus-4-5",
	"claude-haiku-4-5",
	"claude-opus-4-1",
	"claude-3-haiku",
}

// isKnownClaudeModel mirrors getModelCapabilities isKnownModel from
// @ai-sdk/anthropic 4.0.82, which upstream @ai-sdk/amazon-bedrock 5.0.118
// reuses for structured-output routing.
func isKnownClaudeModel(modelID string) bool {
	for _, marker := range knownClaudeModelMarkers {
		if strings.Contains(modelID, marker) {
			return true
		}
	}
	return olderClaude4ModelPattern.MatchString(modelID)
}

var strictToolLegacyClaudePattern = regexp.MustCompile(`claude-(?:instant|v?2|3)(?:[-.:]|$)`)

var strictToolClaudeModelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`claude-(?:opus|sonnet)-4(?:-\d{8})?(?:-v\d+)?(?:[:.]|$)`),
	regexp.MustCompile(`claude-opus-4-(?:1|5|6)(?:[-.:]|$)`),
	regexp.MustCompile(`claude-sonnet-4-(?:5|6)(?:[-.:]|$)`),
	regexp.MustCompile(`claude-haiku-4-5(?:[-.:]|$)`),
}

var unreliableNativeStructuredOutputPattern = regexp.MustCompile(`claude-(?:sonnet-4-6|haiku-4-5)(?:[-.:]|$)`)

// supportsStrictTools reports whether Bedrock accepts tool `strict` for the
// model. Only known supported Claude generations qualify; non-Claude IDs and
// opaque inference profiles keep strict support. Mirrors upstream
// amazon-bedrock-anthropic-model-support.ts (@ai-sdk/amazon-bedrock 5.0.118).
func supportsStrictTools(modelID string) bool {
	if !strings.Contains(modelID, "claude-") || strictToolLegacyClaudePattern.MatchString(modelID) {
		return true
	}
	for _, pattern := range strictToolClaudeModelPatterns {
		if pattern.MatchString(modelID) {
			return true
		}
	}
	return false
}

func supportsNativeStructuredOutput(modelID string) bool {
	return supportsStrictTools(modelID) && !unreliableNativeStructuredOutputPattern.MatchString(modelID)
}

// rejectsForcedToolUse reports Anthropic models that reject tool_choice
// "any" and named-tool choices with a 400. Mirrors @ai-sdk/anthropic 4.0.67
// getModelCapabilities, which upstream @ai-sdk/amazon-bedrock 5.0.99 reuses.
func rejectsForcedToolUse(modelID string) bool {
	return strings.Contains(modelID, "claude-sonnet-5-5")
}

// supportsBetweenToolsThinking reports Anthropic models that accept thinking
// type "between_tools", their lowest thinking setting. These models reject
// thinking type "disabled".
func supportsBetweenToolsThinking(modelID string) bool {
	return strings.Contains(modelID, "claude-sonnet-5-5")
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
