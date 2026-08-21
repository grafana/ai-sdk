package agentobservability

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
)

// systemPromptSeparator is the string the upstream agento11y Anthropic helper
// uses to join multiple system text blocks. Matching it preserves byte-equal
// parity between the ai-sdk path and the legacy Anthropic-typed path.
const systemPromptSeparator = "\n\n"

// messagesToAgento11y converts an ai-sdk prompt to a system prompt and
// agento11y.Message values. RoleSystem entries are folded into a single
// concatenated string joined by [systemPromptSeparator] rather than appearing
// as an agento11y.Message.
// Tool-result parts mixed into non-tool messages are split out into their own
// [agento11y.RoleTool] message — this mirrors how
// `agento11y/go-providers/anthropic.mapRequestMessages` handles the same
// shape on the Anthropic wire (tool_result blocks live inside user messages
// but the underlying SDK normalizes them to tool-role messages).
func messagesToAgento11y(prompt []provider.Message) (string, []agento11y.Message) {
	return messagesToAgento11yWithMedia(prompt, true)
}

func messagesToAgento11yWithMedia(prompt []provider.Message, includeMedia bool) (string, []agento11y.Message) {
	if len(prompt) == 0 {
		return "", nil
	}

	systemParts := make([]string, 0, 1)
	out := make([]agento11y.Message, 0, len(prompt))

	for i := range prompt {
		msg := prompt[i]
		if msg.Role == provider.RoleSystem {
			for _, part := range msg.Content {
				if part.Type == provider.ContentPartTypeText && part.Text != "" {
					systemParts = append(systemParts, part.Text)
				}
			}
			continue
		}

		role, ok := roleToAgento11y(msg.Role)
		if !ok {
			continue
		}

		normalParts, toolParts := convertMessageParts(msg.Content, includeMedia)
		if len(normalParts) > 0 {
			out = append(out, agento11y.Message{Role: role, Parts: normalParts})
		}
		if len(toolParts) > 0 {
			out = append(out, agento11y.Message{Role: agento11y.RoleTool, Parts: toolParts})
		}
	}

	system := strings.Join(systemParts, systemPromptSeparator)
	if len(out) == 0 {
		out = nil
	}
	return system, out
}

func roleToAgento11y(role provider.Role) (agento11y.Role, bool) {
	switch role {
	case provider.RoleUser:
		return agento11y.RoleUser, true
	case provider.RoleAssistant:
		return agento11y.RoleAssistant, true
	case provider.RoleTool:
		return agento11y.RoleTool, true
	default:
		return "", false
	}
}

// convertMessageParts splits message content parts into (non-tool-result parts,
// tool-result parts). The caller emits the second slice as its own
// [agento11y.RoleTool] message when non-empty.
func convertMessageParts(parts []provider.ContentPart, includeMedia bool) (normal, toolResults []agento11y.Part) {
	for i := range parts {
		part, kind, ok := contentPartToAgento11y(parts[i], includeMedia)
		if !ok {
			continue
		}
		if kind == agento11y.PartKindToolResult {
			toolResults = append(toolResults, part)
		} else {
			normal = append(normal, part)
		}
	}
	return normal, toolResults
}

// contentPartToAgento11y converts a single ai-sdk ContentPart into an agento11y.Part.
// The boolean discriminates "convert succeeded" from "unsupported / skip".
// The returned PartKind is also returned so callers can route tool-result
// parts into a separate message (matching the upstream Anthropic helper's
// splitting behavior).
func contentPartToAgento11y(part provider.ContentPart, includeMedia bool) (agento11y.Part, agento11y.PartKind, bool) {
	switch part.Type {
	case provider.ContentPartTypeText:
		if part.Text == "" {
			return agento11y.Part{}, "", false
		}
		return agento11y.TextPart(part.Text), agento11y.PartKindText, true

	case provider.ContentPartTypeReasoning:
		out := agento11y.ThinkingPart(part.Text)
		out.Metadata.ProviderType = "thinking"
		return out, agento11y.PartKindThinking, true

	case provider.ContentPartTypeToolCall:
		// Normalize input JSON to match upstream byte-equal output (the
		// upstream serializes the typed `any` input via json.Marshal, which
		// sorts object keys alphabetically).
		call := agento11y.ToolCall{
			ID:        part.ToolCallID,
			Name:      part.ToolName,
			InputJSON: normalizeJSONObject(part.Input),
		}
		out := agento11y.ToolCallPart(call)
		out.Metadata.ProviderType = providerTypeForToolCall(part)
		return out, agento11y.PartKindToolCall, true

	case provider.ContentPartTypeToolResult:
		content, isErr := toolResultContent(part.Output)
		out := agento11y.ToolResultPart(agento11y.ToolResult{
			ToolCallID:  part.ToolCallID,
			Name:        part.ToolName,
			IsError:     isErr,
			ContentJSON: content,
		})
		out.Metadata.ProviderType = "tool_result"
		return out, agento11y.PartKindToolResult, true

	case provider.ContentPartTypeFile:
		if !includeMedia {
			return agento11y.Part{}, "", false
		}
		out, ok := contentFilePartToAgento11y(part, "file")
		if !ok {
			return agento11y.Part{}, "", false
		}
		return out, agento11y.PartKindMedia, true

	case provider.ContentPartTypeReasoningFile:
		if !includeMedia {
			return agento11y.Part{}, "", false
		}
		out, ok := contentFilePartToAgento11y(part, "reasoning_file")
		if !ok {
			return agento11y.Part{}, "", false
		}
		return out, agento11y.PartKindMedia, true

	default:
		// Custom and tool approval parts are not represented on the underlying SDK wire.
		return agento11y.Part{}, "", false
	}
}

// providerTypeForToolCall recovers the Anthropic provider-type discriminator
// for tool calls. ProviderExecuted indicates a server-side tool (web_search,
// code_execution, …); MCP tools are not currently flagged separately at the
// ai-sdk content layer, so they fall through to "tool_use".
func providerTypeForToolCall(part provider.ContentPart) string {
	if part.ProviderExecuted != nil && *part.ProviderExecuted {
		return "server_tool_use"
	}
	return "tool_use"
}

// toolResultContent serializes a provider.ToolResultOutput into the JSON form
// agento11y.ToolResult.ContentJSON expects, plus the IsError flag. Text outputs
// become JSON-encoded strings; JSON outputs pass through.
func toolResultContent(output *provider.ToolResultOutput) (json.RawMessage, bool) {
	if output == nil {
		return nil, false
	}
	isErr := isErrorOutputType(output.Type)
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		raw, err := json.Marshal(output.Text)
		if err != nil {
			return nil, isErr
		}
		return raw, isErr
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		return cloneRawJSON(output.JSON), isErr
	case provider.ToolOutputContent:
		raw, err := json.Marshal(output.Content)
		if err != nil {
			return nil, isErr
		}
		return raw, isErr
	case provider.ToolOutputExecutionDenied:
		raw, err := json.Marshal(output.Reason)
		if err != nil {
			return nil, isErr
		}
		return raw, isErr
	}
	// Unknown output type: best-effort marshal.
	raw, err := json.Marshal(output)
	if err != nil {
		return nil, isErr
	}
	return raw, isErr
}

func isErrorOutputType(t provider.ToolResultOutputType) bool {
	return t == provider.ToolOutputErrorText ||
		t == provider.ToolOutputErrorJSON ||
		t == provider.ToolOutputExecutionDenied
}

func cloneRawJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}

// normalizeJSONObject re-marshals a JSON-encoded object so its keys are
// emitted in alphabetical order. Returns the input unchanged when it cannot
// be decoded as a generic value (preserves opaque payloads).
func normalizeJSONObject(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return cloneRawJSON(in)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return cloneRawJSON(in)
	}
	return out
}

// toolsToAgento11y converts ai-sdk Tool definitions to agento11y.ToolDefinition.
// Function tools map directly (empty Type). Provider tools preserve their
// type identifier in [agento11y.ToolDefinition.Type] so Agent Observability can annotate
// them downstream. The [DeferLoading] flag is read from
// tool.ProviderOptions["anthropic"].deferLoading when present.
func toolsToAgento11y(tools []provider.Tool) []agento11y.ToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	out := make([]agento11y.ToolDefinition, 0, len(tools))
	for i := range tools {
		t := tools[i]
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		def := agento11y.ToolDefinition{Name: name}
		if t.Description != nil {
			def.Description = *t.Description
		}
		if t.Type == provider.ToolTypeProvider {
			def.Type = t.ID
		}
		if len(t.InputSchema) > 0 {
			// Normalize the schema bytes by round-tripping through map[string]any
			// so JSON object keys come out alphabetized — matches what the
			// upstream agento11y Anthropic helper produces when it marshals a
			// Go-typed schema struct. Falling back to the original bytes on
			// decode failure preserves opaque non-object schemas (e.g. arrays).
			def.InputSchema = normalizeJSONObject(t.InputSchema)
		}
		if defer_ := readToolDeferLoading(t.ProviderOptions); defer_ {
			def.Deferred = true
		}
		out = append(out, def)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readToolDeferLoading(opts provider.ProviderOptions) bool {
	raw, ok := readAnthropicRaw(opts)
	if !ok {
		return false
	}
	var probe struct {
		DeferLoading *bool `json:"deferLoading"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.DeferLoading != nil && *probe.DeferLoading
}

// requestControls captures the optional CallOptions knobs that flow through
// to agento11y.GenerationStart. Pointer types preserve "unset" vs "explicit zero"
// in line with the upstream wire schema.
type requestControls struct {
	MaxTokens   *int64
	Temperature *float64
	TopP        *float64
	ToolChoice  *string
}

// controlsFromCallOptions extracts the request controls from CallOptions.
// Mirrors the field set the upstream agento11y Anthropic helper records on
// agento11y.Generation.
func controlsFromCallOptions(params provider.CallOptions) requestControls {
	var out requestControls
	if params.MaxOutputTokens != nil {
		if value, ok := params.MaxOutputTokens.Int64(); ok {
			out.MaxTokens = &value
		}
	}
	if params.Temperature != nil {
		v := *params.Temperature
		out.Temperature = &v
	}
	if params.TopP != nil {
		v := *params.TopP
		out.TopP = &v
	}
	if choice := toolChoiceToAgento11y(params.ToolChoice); choice != "" {
		out.ToolChoice = &choice
	}
	return out
}

// toolChoiceToAgento11y produces the canonical string the SDK stores in
// Generation.ToolChoice for an ai-sdk ToolChoice value. The output matches
// the wire form the Anthropic SDK serializes — bare lowercased keywords for
// auto/none/required, and a JSON object literal for tool-specific selections.
func toolChoiceToAgento11y(choice *provider.ToolChoice) string {
	if choice == nil {
		return ""
	}
	switch choice.Type {
	case provider.ToolChoiceAuto:
		return "auto"
	case provider.ToolChoiceNone:
		return "none"
	case provider.ToolChoiceRequired:
		return "any"
	case provider.ToolChoiceTool:
		name := strings.TrimSpace(choice.ToolName)
		if name == "" {
			return ""
		}
		raw, err := json.Marshal(map[string]string{
			"type": "tool",
			"name": name,
		})
		if err != nil {
			return ""
		}
		return string(raw)
	default:
		return ""
	}
}

// readAnthropicRaw returns the raw JSON of opts["anthropic"], handling both
// the typed (anthropic.AnthropicOptions) and RawProviderOption forms.
// Returning the raw bytes lets the middleware decode just the subset it
// needs without importing the anthropic provider module.
func readAnthropicRaw(opts provider.ProviderOptions) (json.RawMessage, bool) {
	if opts == nil {
		return nil, false
	}
	v, ok := opts["anthropic"]
	if !ok || v == nil {
		return nil, false
	}
	if raw, ok := v.(provider.RawProviderOption); ok {
		return raw.Raw, len(raw.Raw) > 0
	}
	encoded, err := json.Marshal(v)
	if err != nil || len(encoded) == 0 {
		return nil, false
	}
	return encoded, true
}

// thinkingBudgetFromAnthropic decodes the budget tokens out of
// params.ProviderOptions["anthropic"]. It accepts both the camelCase wire
// form used by ai-sdk's anthropic provider (`thinking.budgetTokens`) and the
// snake_case form used by the upstream Anthropic SDK (`thinking.budget_tokens`)
// so the middleware does not have to know which producer assembled the
// options blob. The mapper does not import `providers/anthropic`.
func thinkingBudgetFromAnthropic(opts provider.ProviderOptions) *int64 {
	raw, ok := readAnthropicRaw(opts)
	if !ok {
		return nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}
	thinkingRaw, ok := probe["thinking"]
	if !ok {
		return nil
	}
	var thinking map[string]json.RawMessage
	if err := json.Unmarshal(thinkingRaw, &thinking); err != nil {
		return nil
	}
	for _, key := range []string{"budget_tokens", "budgetTokens"} {
		if bRaw, ok := thinking[key]; ok {
			if budget := coerceInt64FromJSON(bRaw); budget != nil {
				return budget
			}
		}
	}
	return nil
}

// thinkingEnabledFromAnthropic decodes the thinking.type field, returning a
// non-nil *bool only when the type is recognized as "enabled", "adaptive",
// or "disabled" (matching the upstream Anthropic helper's logic). Unknown
// values map to nil, leaving Generation.ThinkingEnabled unset.
func thinkingEnabledFromAnthropic(opts provider.ProviderOptions) *bool {
	raw, ok := readAnthropicRaw(opts)
	if !ok {
		return nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}
	thinkingRaw, ok := probe["thinking"]
	if !ok {
		return nil
	}
	var thinking struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(thinkingRaw, &thinking); err != nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(thinking.Type)) {
	case "enabled", "adaptive":
		v := true
		return &v
	case "disabled":
		v := false
		return &v
	default:
		return nil
	}
}

func coerceInt64FromJSON(raw json.RawMessage) *int64 {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	// Try int first (preserves precision for large values).
	if v, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return &v
	}
	// Fall back to JSON number (handles 1e3 etc.).
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return nil
	}
	v, err := num.Int64()
	if err != nil {
		return nil
	}
	return &v
}
