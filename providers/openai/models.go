package openai

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// modelCapabilities describes capability flags for an OpenAI model, mirroring
// upstream getOpenAILanguageModelCapabilities.
type modelCapabilities struct {
	isReasoningModel               bool
	systemMessageMode              string // "system" or "developer"
	supportsFlexProcessing         bool
	supportsPriorityProcessing     bool
	supportsNonReasoningParameters bool
}

var (
	oSeriesModelPattern = regexp.MustCompile(`^o(\d+)(?:-|$)`)
	gptModelPattern     = regexp.MustCompile(`^gpt-(\d+)(?:\.(\d+))?(?:-(.+))?$`)
)

type gptVersion struct {
	major   int
	minor   *int
	variant string
}

// getModelCapabilities returns capability flags using anchored OpenAI model
// family parsing.
func getModelCapabilities(modelID string) modelCapabilities {
	// Bedrock Mantle namespaces OpenAI model IDs without changing their capabilities.
	modelID = strings.TrimPrefix(modelID, "openai.")
	oVersion, hasOSeriesVersion := parseOSeriesVersion(modelID)
	gpt, hasGPTVersion := parseGPTVersion(modelID)
	isGPTChat := hasGPTVersion && gpt.minor == nil && strings.HasPrefix(gpt.variant, "chat")
	isGPTNano := hasGPTVersion && strings.HasPrefix(gpt.variant, "nano")

	isReasoning := hasOSeriesVersion || (hasGPTVersion && gpt.major >= 5 && !isGPTChat)
	supportsFlex := (hasOSeriesVersion && oVersion >= 3) || (hasGPTVersion && gpt.major >= 5 && !isGPTChat)
	supportsPriority := strings.HasPrefix(modelID, "gpt-4") ||
		(hasGPTVersion && gpt.major >= 5 && !isGPTNano && !isGPTChat) ||
		(hasOSeriesVersion && oVersion >= 3)
	supportsNonReasoningParams := hasGPTVersion &&
		(gpt.major > 5 || (gpt.major == 5 && gpt.minor != nil && *gpt.minor >= 1))

	mode := "system"
	if isReasoning {
		mode = "developer"
	}
	return modelCapabilities{
		isReasoningModel:               isReasoning,
		systemMessageMode:              mode,
		supportsFlexProcessing:         supportsFlex,
		supportsPriorityProcessing:     supportsPriority,
		supportsNonReasoningParameters: supportsNonReasoningParams,
	}
}

func parseOSeriesVersion(modelID string) (int, bool) {
	match := oSeriesModelPattern.FindStringSubmatch(modelID)
	if match == nil {
		return 0, false
	}
	version, err := strconv.Atoi(match[1])
	return version, err == nil
}

func parseGPTVersion(modelID string) (gptVersion, bool) {
	match := gptModelPattern.FindStringSubmatch(modelID)
	if match == nil {
		return gptVersion{}, false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return gptVersion{}, false
	}
	version := gptVersion{major: major, variant: match[3]}
	if match[2] != "" {
		minor, err := strconv.Atoi(match[2])
		if err != nil {
			return gptVersion{}, false
		}
		version.minor = &minor
	}
	return version, true
}

// knownResponsesModelIDs lists the OpenAI Responses model ids from upstream
// @ai-sdk/openai's OpenAIResponsesModelId union and openaiResponsesModelIds
// const. The list is advisory; NewResponses accepts any string.
var knownResponsesModelIDs = []string{
	"gpt-3.5-turbo",
	"gpt-3.5-turbo-0125",
	"gpt-3.5-turbo-1106",
	"gpt-4.1",
	"gpt-4.1-2025-04-14",
	"gpt-4.1-mini",
	"gpt-4.1-mini-2025-04-14",
	"gpt-4.1-nano",
	"gpt-4.1-nano-2025-04-14",
	"gpt-4o",
	"gpt-4o-2024-05-13",
	"gpt-4o-2024-08-06",
	"gpt-4o-2024-11-20",
	"gpt-4o-audio-preview",
	"gpt-4o-audio-preview-2024-12-17",
	"gpt-4o-mini",
	"gpt-4o-mini-2024-07-18",
	"gpt-4o-mini-search-preview",
	"gpt-4o-mini-search-preview-2025-03-11",
	"gpt-4o-search-preview",
	"gpt-4o-search-preview-2025-03-11",
	"gpt-5",
	"gpt-5-2025-08-07",
	"gpt-5-chat-latest",
	"gpt-5-codex",
	"gpt-5-mini",
	"gpt-5-mini-2025-08-07",
	"gpt-5-nano",
	"gpt-5-nano-2025-08-07",
	"gpt-5-pro",
	"gpt-5-pro-2025-10-06",
	"gpt-5.1",
	"gpt-5.1-2025-11-13",
	"gpt-5.1-chat-latest",
	"gpt-5.1-codex",
	"gpt-5.1-codex-max",
	"gpt-5.1-codex-mini",
	"gpt-5.2",
	"gpt-5.2-2025-12-11",
	"gpt-5.2-chat-latest",
	"gpt-5.2-codex",
	"gpt-5.2-pro",
	"gpt-5.2-pro-2025-12-11",
	"gpt-5.3-chat-latest",
	"gpt-5.3-codex",
	"gpt-5.4",
	"gpt-5.4-2026-03-05",
	"gpt-5.4-mini",
	"gpt-5.4-mini-2026-03-17",
	"gpt-5.4-nano",
	"gpt-5.4-nano-2026-03-17",
	"gpt-5.4-pro",
	"gpt-5.4-pro-2026-03-05",
	"gpt-5.5",
	"gpt-5.5-2026-04-23",
	"gpt-5.6",
	"gpt-5.6-luna",
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"o1",
	"o1-2024-12-17",
	"o3",
	"o3-2025-04-16",
	"o3-mini",
	"o3-mini-2025-01-31",
	"o4-mini",
	"o4-mini-2025-04-16",
}

// ModelIDs returns a sorted copy of the advisory known model id list.
func ModelIDs() []string {
	out := make([]string, len(knownResponsesModelIDs))
	copy(out, knownResponsesModelIDs)
	sort.Strings(out)
	return out
}
