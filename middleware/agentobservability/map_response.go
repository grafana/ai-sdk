package agentobservability

import (
	"encoding/json"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/internal/ptr"
	"github.com/grafana/ai-sdk/provider"
)

// Stop-reason strings produced by finishReasonToAgento11yStop. These match the
// values the legacy internal/llm/claude/* path emitted so existing dashboards
// and BigQuery filters keep working.
const (
	stopReasonEndTurn      = "end_turn"
	stopReasonMaxTokens    = "max_tokens"
	stopReasonToolUse      = "tool_use"
	stopReasonStopSequence = "stop_sequence"
	stopReasonError        = "error"
	stopReasonOther        = "other"
)

// contentToAgento11yOutput converts a generate result's content slice into the
// single assistant-role agento11y.Message recorded as Generation.Output.
//
// Mirrors `agento11y/go-providers/anthropic.mapResponseMessages` modulo the
// tool-result splitting (we do the same split into a separate RoleTool
// message when the response carries tool-result parts; that path is rare for
// generate responses but valid for provider-executed multi-turn).
func contentToAgento11yOutput(content []provider.GenerateContentPart) []agento11y.Message {
	if len(content) == 0 {
		return nil
	}

	assistantParts := make([]agento11y.Part, 0, len(content))
	toolParts := make([]agento11y.Part, 0, 1)

	for i := range content {
		part, kind, ok := generateContentPartToAgento11y(content[i])
		if !ok {
			continue
		}
		if kind == agento11y.PartKindToolResult {
			toolParts = append(toolParts, part)
			continue
		}
		assistantParts = append(assistantParts, part)
	}

	out := make([]agento11y.Message, 0, 2)
	if len(assistantParts) > 0 {
		out = append(out, agento11y.Message{
			Role:  agento11y.RoleAssistant,
			Parts: assistantParts,
		})
	}
	if len(toolParts) > 0 {
		out = append(out, agento11y.Message{
			Role:  agento11y.RoleTool,
			Parts: toolParts,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func generateContentPartToAgento11y(part provider.GenerateContentPart) (agento11y.Part, agento11y.PartKind, bool) {
	switch part.Type {
	case provider.ContentText:
		if part.Text == "" {
			return agento11y.Part{}, "", false
		}
		return agento11y.TextPart(part.Text), agento11y.PartKindText, true

	case provider.ContentReasoning:
		out := agento11y.ThinkingPart(part.Text)
		out.Metadata.ProviderType = "thinking"
		return out, agento11y.PartKindThinking, true

	case provider.ContentToolCall:
		call := agento11y.ToolCall{
			ID:        part.ToolCallID,
			Name:      part.ToolName,
			InputJSON: normalizeJSONObject(part.Input),
		}
		out := agento11y.ToolCallPart(call)
		out.Metadata.ProviderType = providerTypeForGenerateToolCall(part)
		return out, agento11y.PartKindToolCall, true

	case provider.ContentToolResult:
		isErr := part.IsError
		var contentJSON json.RawMessage
		if len(part.Result) > 0 {
			contentJSON = cloneRawJSON(part.Result)
		}
		out := agento11y.ToolResultPart(agento11y.ToolResult{
			ToolCallID:  part.ToolCallID,
			Name:        part.ToolName,
			IsError:     isErr,
			ContentJSON: contentJSON,
		})
		out.Metadata.ProviderType = "tool_result"
		return out, agento11y.PartKindToolResult, true

	case provider.ContentFile:
		out, ok := generateFilePartToAgento11y(part, "file")
		if !ok {
			return agento11y.Part{}, "", false
		}
		return out, agento11y.PartKindMedia, true

	case provider.ContentReasoningFile:
		out, ok := generateFilePartToAgento11y(part, "reasoning_file")
		if !ok {
			return agento11y.Part{}, "", false
		}
		return out, agento11y.PartKindMedia, true

	default:
		// Source, custom, and tool approval parts are not represented on the underlying SDK wire.
		return agento11y.Part{}, "", false
	}
}

func providerTypeForGenerateToolCall(part provider.GenerateContentPart) string {
	if part.ProviderExecuted {
		return "server_tool_use"
	}
	return "tool_use"
}

// usageToAgento11y converts provider.Usage into agento11y.TokenUsage. The mapping
// matches the upstream Anthropic helper field-for-field so byte-equal generation
// JSON output is preserved across the two paths.
func usageToAgento11y(usage provider.Usage) agento11y.TokenUsage {
	out := agento11y.TokenUsage{
		InputTokens:           intPtrOrZero(usage.InputTokens.Total),
		OutputTokens:          intPtrOrZero(usage.OutputTokens.Total),
		CacheReadInputTokens:  intPtrOrZero(usage.InputTokens.CacheRead),
		CacheWriteInputTokens: intPtrOrZero(usage.InputTokens.CacheWrite),
		ReasoningTokens:       intPtrOrZero(usage.OutputTokens.Reasoning),
	}
	out.TotalTokens = out.InputTokens + out.OutputTokens
	return out
}

func intPtrOrZero(value *int) int64 {
	return int64(ptr.Deref(value, 0))
}

// finishReasonToAgento11yStop maps an ai-sdk FinishReason to the canonical stop
// reason string the SDK expects. The strings match what the legacy
// internal/llm/claude/* path emitted so existing dashboards keep working.
//
// When the provider exposes a non-empty Raw form (the provider-native stop
// reason, e.g. "stop_sequence" or "pause_turn") we prefer it because Anthropic
// uses values beyond the unified ai-sdk enum; otherwise we translate the
// Unified value.
func finishReasonToAgento11yStop(reason provider.FinishReason) string {
	if reason.Raw != "" {
		return reason.Raw
	}
	switch reason.Unified {
	case provider.FinishReasonStop:
		return stopReasonEndTurn
	case provider.FinishReasonLength:
		return stopReasonMaxTokens
	case provider.FinishReasonToolCalls:
		return stopReasonToolUse
	case provider.FinishReasonError:
		return stopReasonError
	case provider.FinishReasonContentFilter:
		return stopReasonOther
	case provider.FinishReasonOther:
		return stopReasonOther
	default:
		return ""
	}
}
