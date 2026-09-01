package openaicompatible

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"github.com/grafana/ai-sdk/internal/mediatype"
	"github.com/grafana/ai-sdk/provider"
)

func (m *model) buildRequest(opts provider.CallOptions, streaming bool) (map[string]any, []provider.Warning, error) {
	warnings := deprecatedProviderOptionWarnings(opts.ProviderOptions, m.providerName)

	openAIOpts, err := readOpenAIOptions(opts.ProviderOptions, m.providerName)
	if err != nil {
		return nil, warnings, err
	}

	messages, err := convertPrompt(opts.Prompt, resolveMetadataKey(opts.ProviderOptions, m.providerName))
	if err != nil {
		return nil, warnings, err
	}

	tools, toolChoice, toolWarnings := prepareTools(opts.Tools, opts.ToolChoice)
	warnings = append(warnings, toolWarnings...)

	body := map[string]any{
		"model": m.modelID,
	}

	if openAIOpts.User != "" {
		body["user"] = openAIOpts.User
	}
	if opts.MaxOutputTokens != nil {
		body["max_tokens"] = *opts.MaxOutputTokens
	}
	if opts.Temperature != nil {
		body["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		body["top_p"] = *opts.TopP
	}
	if opts.TopK != nil {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "topK",
			Details: "OpenAI-compatible Chat Completions APIs do not support topK.",
		})
	}
	if opts.FrequencyPenalty != nil {
		body["frequency_penalty"] = *opts.FrequencyPenalty
	}
	if opts.PresencePenalty != nil {
		body["presence_penalty"] = *opts.PresencePenalty
	}
	if len(opts.StopSequences) > 0 {
		body["stop"] = append([]string{}, opts.StopSequences...)
	}
	if opts.Seed != nil {
		body["seed"] = *opts.Seed
	}
	if rf, rfWarnings := m.convertResponseFormat(opts.ResponseFormat, openAIOpts); rf != nil {
		body["response_format"] = rf
		warnings = append(warnings, rfWarnings...)
	} else {
		warnings = append(warnings, rfWarnings...)
	}

	for k, v := range openAIOpts.extraFields {
		body[k] = v
	}
	delete(body, "reasoning_effort")
	delete(body, "verbosity")
	delete(body, "messages")
	delete(body, "tools")
	delete(body, "tool_choice")

	if effort := reasoningEffort(opts.Reasoning, openAIOpts); effort != "" {
		body["reasoning_effort"] = effort
	}
	if openAIOpts.TextVerbosity != "" {
		body["verbosity"] = openAIOpts.TextVerbosity
	}

	body["messages"] = messages

	if len(tools) > 0 {
		body["tools"] = tools
		if toolChoice != nil {
			body["tool_choice"] = toolChoice
		}
	}

	if streaming {
		body["stream"] = true
		delete(body, "stream_options")
		if m.includeUsage {
			body["stream_options"] = streamOptions{IncludeUsage: true}
		}
	}

	if m.transformRequestBody != nil {
		transformed, err := m.transformRequestBody(body)
		if err != nil {
			return nil, warnings, fmt.Errorf("openai: transforming request body: %w", err)
		}
		if transformed == nil {
			return nil, warnings, fmt.Errorf("openai: transforming request body returned nil")
		}
		body = transformed
	}

	return body, warnings, nil
}

func readOpenAIOptions(opts provider.ProviderOptions, providerName string) (OpenAIOptions, error) {
	var result OpenAIOptions
	for _, spec := range openAIOptionKeySpecs(providerName) {
		key := spec.key
		oo, ok, err := provider.ResolveOption[OpenAIOptions](opts, key)
		if err != nil {
			return OpenAIOptions{}, fmt.Errorf("openai: reading provider options %q: %w", key, err)
		}
		if ok {
			if spec.passUnknown {
				extra, err := unknownProviderOptionFields(opts[key])
				if err != nil {
					return OpenAIOptions{}, fmt.Errorf("openai: reading provider option fields %q: %w", key, err)
				}
				if len(extra) > 0 {
					if oo.extraFields == nil {
						oo.extraFields = make(map[string]any, len(extra))
					}
					for k, v := range extra {
						if _, exists := oo.extraFields[k]; !exists {
							oo.extraFields[k] = v
						}
					}
				}
			}
			mergeOpenAIOptions(&result, oo)
		}
	}
	return result, nil
}

type openAIOptionKeySpec struct {
	key         string
	passUnknown bool
}

func openAIOptionKeySpecs(providerName string) []openAIOptionKeySpec {
	name := providerOptionsName(providerName)
	camelName := toCamelCase(name)

	candidates := []openAIOptionKeySpec{
		{key: "openai-compatible"},
		{key: "openaiCompatible"},
		{key: name, passUnknown: true},
		{key: camelName, passUnknown: true},
	}

	specs := make([]openAIOptionKeySpec, 0, len(candidates))
	seen := map[string]int{}
	for _, candidate := range candidates {
		if candidate.key == "" {
			continue
		}
		if idx, ok := seen[candidate.key]; ok {
			specs[idx].passUnknown = specs[idx].passUnknown || candidate.passUnknown
			continue
		}
		seen[candidate.key] = len(specs)
		specs = append(specs, candidate)
	}
	return specs
}

func providerOptionsName(providerName string) string {
	name, _, _ := strings.Cut(providerName, ".")
	return strings.TrimSpace(name)
}

func deprecatedProviderOptionWarnings(opts provider.ProviderOptions, providerName string) []provider.Warning {
	if len(opts) == 0 {
		return nil
	}

	var warnings []provider.Warning
	if _, ok := opts["openai-compatible"]; ok {
		warnings = append(warnings, deprecatedProviderOptionWarning("openai-compatible", "openaiCompatible"))
	}

	rawName := providerOptionsName(providerName)
	camelName := toCamelCase(rawName)
	if rawName != "" && rawName != "openai-compatible" && rawName != camelName {
		if _, ok := opts[rawName]; ok {
			warnings = append(warnings, deprecatedProviderOptionWarning(rawName, camelName))
		}
	}

	return warnings
}

func deprecatedProviderOptionWarning(rawName, camelName string) provider.Warning {
	return provider.Warning{
		Type:    provider.WarnDeprecated,
		Setting: fmt.Sprintf("providerOptions key '%s'", rawName),
		Message: fmt.Sprintf("Use '%s' instead.", camelName),
	}
}

func toCamelCase(s string) string {
	var b strings.Builder
	upperNext := false
	for _, r := range s {
		if r == '-' || r == '_' {
			upperNext = true
			continue
		}
		if upperNext && r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		b.WriteRune(r)
		upperNext = false
	}
	return b.String()
}

var openAIOptionFields = map[string]struct{}{
	"user":             {},
	"reasoningEffort":  {},
	"textVerbosity":    {},
	"strictJsonSchema": {},
}

func unknownProviderOptionFields(opt provider.ProviderOption) (map[string]any, error) {
	raw, ok := opt.(provider.RawProviderOption)
	if !ok || len(raw.Raw) == 0 {
		return nil, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw.Raw, &fields); err != nil {
		return nil, err
	}

	out := make(map[string]any)
	for k, rawValue := range fields {
		if _, known := openAIOptionFields[k]; known {
			continue
		}
		var v any
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func mergeOpenAIOptions(dst *OpenAIOptions, src OpenAIOptions) {
	if src.User != "" {
		dst.User = src.User
	}
	if src.ReasoningEffort != "" {
		dst.ReasoningEffort = src.ReasoningEffort
	}
	if src.TextVerbosity != "" {
		dst.TextVerbosity = src.TextVerbosity
	}
	if src.StrictJSONSchema != nil {
		dst.StrictJSONSchema = src.StrictJSONSchema
	}
	if len(src.extraFields) > 0 {
		if dst.extraFields == nil {
			dst.extraFields = make(map[string]any, len(src.extraFields))
		}
		for k, v := range src.extraFields {
			dst.extraFields[k] = v
		}
	}
}

func convertPrompt(prompt []provider.Message, providerOptionsKey string) ([]chatMessage, error) {
	messages := make([]chatMessage, 0, len(prompt))
	for _, msg := range prompt {
		switch msg.Role {
		case provider.RoleSystem:
			text, err := collectText(msg.Content, "system")
			if err != nil {
				return nil, err
			}
			extra, err := openAICompatibleMetadata(msg.ProviderOptions)
			if err != nil {
				return nil, err
			}
			messages = append(messages, chatMessage{Role: "system", Content: text, ExtraFields: extra})
		case provider.RoleUser:
			converted, extra, err := convertUserContent(msg.Content, msg.ProviderOptions)
			if err != nil {
				return nil, err
			}
			messages = append(messages, chatMessage{Role: "user", Content: converted, ExtraFields: extra})
		case provider.RoleAssistant:
			converted, err := convertAssistantMessage(msg.Content, msg.ProviderOptions, providerOptionsKey)
			if err != nil {
				return nil, err
			}
			messages = append(messages, converted)
		case provider.RoleTool:
			converted, err := convertToolMessages(msg.Content)
			if err != nil {
				return nil, err
			}
			messages = append(messages, converted...)
		default:
			return nil, fmt.Errorf("openai: unsupported message role %q", msg.Role)
		}
	}
	return messages, nil
}

func collectText(parts []provider.ContentPart, role string) (string, error) {
	var b strings.Builder
	for _, part := range parts {
		if part.Type != provider.ContentPartTypeText {
			return "", fmt.Errorf("openai: %s messages only support text parts, got %q", role, part.Type)
		}
		b.WriteString(part.Text)
	}
	return b.String(), nil
}

func convertUserContent(parts []provider.ContentPart, messageOptions provider.ProviderOptions) (any, map[string]any, error) {
	if len(parts) == 1 && parts[0].Type == provider.ContentPartTypeText {
		extra, err := openAICompatibleMetadata(parts[0].ProviderOptions)
		return parts[0].Text, extra, err
	}

	messageExtra, err := openAICompatibleMetadata(messageOptions)
	if err != nil {
		return nil, nil, err
	}

	out := make([]chatContentPart, 0, len(parts))
	for _, part := range parts {
		partExtra, err := openAICompatibleMetadata(part.ProviderOptions)
		if err != nil {
			return nil, nil, err
		}
		switch part.Type {
		case provider.ContentPartTypeText:
			out = append(out, chatContentPart{Type: "text", Text: part.Text, ExtraFields: partExtra})
		case provider.ContentPartTypeFile:
			converted, err := convertFileContent(part)
			if err != nil {
				return nil, nil, err
			}
			converted.ExtraFields = partExtra
			out = append(out, converted)
		default:
			return nil, nil, fmt.Errorf("openai: user messages do not support content part %q", part.Type)
		}
	}
	return out, messageExtra, nil
}

func convertAssistantMessage(parts []provider.ContentPart, messageOptions provider.ProviderOptions, providerOptionsKey string) (chatMessage, error) {
	var text strings.Builder
	var reasoning strings.Builder
	var toolCalls []chatToolCall

	messageExtra, err := openAICompatibleMetadata(messageOptions)
	if err != nil {
		return chatMessage{}, err
	}

	for _, part := range parts {
		switch part.Type {
		case provider.ContentPartTypeText:
			text.WriteString(part.Text)
		case provider.ContentPartTypeReasoning:
			reasoning.WriteString(part.Text)
		case provider.ContentPartTypeToolCall:
			input := strings.TrimSpace(string(part.Input))
			if input == "" || !json.Valid([]byte(input)) {
				input = "{}"
			}
			toolCall := chatToolCall{
				ID:   part.ToolCallID,
				Type: "function",
				Function: toolCallFunction{
					Name:      part.ToolName,
					Arguments: input,
				},
			}
			partExtra, err := openAICompatibleMetadata(part.ProviderOptions)
			if err != nil {
				return chatMessage{}, err
			}
			toolCall.ExtraFields = partExtra
			thoughtSignature, err := googleThoughtSignature(part.ProviderOptions, providerOptionsKey)
			if err != nil {
				return chatMessage{}, err
			}
			if thoughtSignature != "" {
				toolCall.ExtraContent = &extraContent{
					Google: &googleExtraContent{ThoughtSignature: thoughtSignature},
				}
			}
			toolCalls = append(toolCalls, toolCall)
		case provider.ContentPartTypeToolApprovalRequest, provider.ContentPartTypeToolApprovalResponse:
			continue
		default:
			return chatMessage{}, fmt.Errorf("openai: assistant messages do not support content part %q", part.Type)
		}
	}

	content := text.String()
	msg := chatMessage{Role: "assistant", Content: content, ExtraFields: messageExtra}
	if len(toolCalls) > 0 {
		if content == "" {
			var nullContent *string
			msg.Content = nullContent
		}
		msg.ToolCalls = toolCalls
	}
	if reasoning.Len() > 0 {
		msg.ReasoningContent = reasoning.String()
	}
	return msg, nil
}

func openAICompatibleMetadata(opts provider.ProviderOptions) (map[string]any, error) {
	meta, ok, err := provider.ResolveOption[map[string]any](opts, "openaiCompatible")
	if err != nil {
		return nil, fmt.Errorf("openai: reading openaiCompatible provider metadata: %w", err)
	}
	if !ok || len(meta) == 0 {
		return nil, nil
	}
	return meta, nil
}

type googleOptions struct {
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

func googleThoughtSignature(opts provider.ProviderOptions, providerOptionsKey string) (string, error) {
	if providerOptionsKey != "" && providerOptionsKey != "google" {
		custom, ok, err := provider.ResolveOption[googleOptions](opts, providerOptionsKey)
		if err != nil {
			return "", fmt.Errorf("openai: reading %s provider options: %w", providerOptionsKey, err)
		}
		if ok {
			return custom.ThoughtSignature, nil
		}
	}
	google, ok, err := provider.ResolveOption[googleOptions](opts, "google")
	if err != nil {
		return "", fmt.Errorf("openai: reading google provider options: %w", err)
	}
	if !ok {
		return "", nil
	}
	return google.ThoughtSignature, nil
}

func convertToolMessages(parts []provider.ContentPart) ([]chatMessage, error) {
	messages := make([]chatMessage, 0, len(parts))
	for _, part := range parts {
		if part.Type == provider.ContentPartTypeToolApprovalResponse {
			continue
		}
		if part.Type != provider.ContentPartTypeToolResult {
			return nil, fmt.Errorf("openai: tool messages do not support content part %q", part.Type)
		}
		content, err := toolResultOutputString(part.Output)
		if err != nil {
			return nil, err
		}
		extra, err := openAICompatibleMetadata(part.ProviderOptions)
		if err != nil {
			return nil, err
		}
		messages = append(messages, chatMessage{
			Role:        "tool",
			ToolCallID:  part.ToolCallID,
			Content:     content,
			ExtraFields: extra,
		})
	}
	return messages, nil
}

func convertFileContent(part provider.ContentPart) (chatContentPart, error) {
	if part.Data == nil {
		return chatContentPart{}, fmt.Errorf("openai: file part has no data")
	}
	if err := part.Data.Validate(); err != nil {
		return chatContentPart{}, fmt.Errorf("openai: invalid file part: %w", err)
	}
	var err error
	part, err = normalizeDataURLFilePart(part)
	if err != nil {
		return chatContentPart{}, err
	}

	topLevel := topLevelMediaType(part.MediaType)
	switch topLevel {
	case "image":
		resolvedMediaType := mediaType(part.MediaType)
		if part.Data.URL == "" {
			var err error
			resolvedMediaType, err = resolveFullMediaType(part)
			if err != nil {
				return chatContentPart{}, err
			}
		}
		url, err := dataURL(resolvedMediaType, part.Data)
		if err != nil {
			return chatContentPart{}, err
		}
		return chatContentPart{Type: "image_url", ImageURL: &imageURLPart{URL: url}}, nil
	case "video":
		resolvedMediaType := mediaType(part.MediaType)
		if part.Data.URL == "" {
			var err error
			resolvedMediaType, err = resolveFullMediaType(part)
			if err != nil {
				return chatContentPart{}, err
			}
		}
		url, err := dataURL(resolvedMediaType, part.Data)
		if err != nil {
			return chatContentPart{}, err
		}
		return chatContentPart{Type: "video_url", VideoURL: &videoURLPart{URL: url}}, nil
	case "audio":
		if part.Data.URL != "" {
			return chatContentPart{}, fmt.Errorf("openai: audio file URL parts are not supported")
		}
		resolvedMediaType, err := resolveFullMediaType(part)
		if err != nil {
			return chatContentPart{}, err
		}
		format := audioFormat(resolvedMediaType)
		if format == "" {
			return chatContentPart{}, fmt.Errorf("openai: unsupported audio media type %q", resolvedMediaType)
		}
		data, err := base64Data(part.Data)
		if err != nil {
			return chatContentPart{}, err
		}
		return chatContentPart{Type: "input_audio", InputAudio: &inputAudioPart{Data: data, Format: format}}, nil
	case "application":
		if part.Data.URL != "" {
			return chatContentPart{}, fmt.Errorf("openai: PDF file URL parts are not supported")
		}
		resolvedMediaType, err := resolveFullMediaType(part)
		if err != nil {
			return chatContentPart{}, err
		}
		if resolvedMediaType != "application/pdf" {
			return chatContentPart{}, fmt.Errorf("openai: unsupported file media type %q", resolvedMediaType)
		}
		data, err := base64Data(part.Data)
		if err != nil {
			return chatContentPart{}, err
		}
		filename := part.Filename
		if filename == "" {
			filename = "document.pdf"
		}
		return chatContentPart{
			Type: "file",
			File: &filePart{Filename: filename, FileData: "data:application/pdf;base64," + data},
		}, nil
	case "text":
		text, err := textFileContent(part.Data)
		if err != nil {
			return chatContentPart{}, err
		}
		return chatContentPart{Type: "text", Text: text}, nil
	default:
		return chatContentPart{}, fmt.Errorf("openai: unsupported file media type %q", part.MediaType)
	}
}

func topLevelMediaType(value string) string {
	mt := mediaType(value)
	before, _, _ := strings.Cut(mt, "/")
	return before
}

func normalizeDataURLFilePart(part provider.ContentPart) (provider.ContentPart, error) {
	if part.Data == nil {
		return part, nil
	}
	rawURL := strings.TrimSpace(part.Data.URL)
	if !strings.HasPrefix(strings.ToLower(rawURL), "data:") {
		return part, nil
	}
	mediaType, base64Content, ok := splitDataURL(rawURL)
	if !ok || mediaType == "" {
		return provider.ContentPart{}, fmt.Errorf("openai: invalid data URL in file part")
	}
	part.MediaType = mediaType
	data := provider.Base64DataContent(base64Content)
	part.Data = &data
	return part, nil
}

func splitDataURL(value string) (mediaType string, base64Content string, ok bool) {
	header, content, found := strings.Cut(value, ",")
	if !found {
		return "", "", false
	}
	prefix, mediaType, found := strings.Cut(header, ":")
	if !found || strings.ToLower(prefix) != "data" {
		return "", "", false
	}
	mediaType, _, _ = strings.Cut(mediaType, ";")
	return mediaType, content, true
}

func mediaType(value string) string {
	mt, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(mt)
}

func resolveFullMediaType(part provider.ContentPart) (string, error) {
	mt := mediaType(part.MediaType)
	if isFullMediaType(mt) {
		return mt, nil
	}
	if part.Data == nil || part.Data.URL != "" {
		return "", fmt.Errorf("openai: file of media type %q must specify subtype since it is not passed as inline bytes", part.MediaType)
	}
	if detected := detectMediaType(part.Data, topLevelMediaType(part.MediaType)); detected != "" {
		return detected, nil
	}
	return "", fmt.Errorf("openai: file of media type %q must specify subtype since it could not be auto-detected", part.MediaType)
}

func isFullMediaType(value string) bool {
	_, subtype, ok := strings.Cut(mediaType(value), "/")
	return ok && subtype != "" && subtype != "*"
}

func detectMediaType(data *provider.DataContent, topLevel string) string {
	return mediatype.Detect(data.Bytes, data.Base64, topLevel)
}

func audioFormat(value string) string {
	switch mediaType(value) {
	case "audio/wav":
		return "wav"
	case "audio/mp3", "audio/mpeg":
		return "mp3"
	default:
		return ""
	}
}

func dataURL(mediaType string, data *provider.DataContent) (string, error) {
	if data.URL != "" {
		return data.URL, nil
	}
	encoded, err := base64Data(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("data:%s;base64,%s", mediaType, encoded), nil
}

func base64Data(data *provider.DataContent) (string, error) {
	switch {
	case data.Bytes != nil:
		return base64.StdEncoding.EncodeToString(data.Bytes), nil
	case data.Base64 != "":
		return data.Base64, nil
	default:
		return "", fmt.Errorf("openai: expected binary data")
	}
}

func textFileContent(data *provider.DataContent) (string, error) {
	switch {
	case data.URL != "":
		return data.URL, nil
	case data.Bytes != nil:
		return string(data.Bytes), nil
	case data.Base64 != "":
		decoded, err := base64.StdEncoding.DecodeString(data.Base64)
		if err != nil {
			return "", fmt.Errorf("openai: decoding text file base64: %w", err)
		}
		return string(decoded), nil
	default:
		return "", fmt.Errorf("openai: expected text file data")
	}
}

func toolResultOutputString(output *provider.ToolResultOutput) (string, error) {
	if output == nil {
		return "null", nil
	}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		return output.Text, nil
	case provider.ToolOutputExecutionDenied:
		if output.Reason != "" {
			return output.Reason, nil
		}
		return "Tool call execution denied.", nil
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		if len(output.JSON) == 0 {
			return "null", nil
		}
		return string(output.JSON), nil
	case provider.ToolOutputContent:
		b, err := json.Marshal(output.Content)
		if err != nil {
			return "", fmt.Errorf("openai: marshaling tool result content: %w", err)
		}
		return string(b), nil
	default:
		return "", fmt.Errorf("openai: unsupported tool result output type %q", output.Type)
	}
}

func prepareTools(tools []provider.Tool, choice *provider.ToolChoice) ([]chatTool, any, []provider.Warning) {
	if len(tools) == 0 {
		return nil, nil, nil
	}

	var warnings []provider.Warning
	out := make([]chatTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type == provider.ToolTypeProvider {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "provider-defined tool " + tool.ID,
			})
			continue
		}
		parameters := tool.InputSchema
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		strict := tool.Strict
		out = append(out, chatTool{
			Type: "function",
			Function: toolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  parameters,
				Strict:      strict,
			},
		})
	}

	if len(out) == 0 || choice == nil {
		return out, nil, warnings
	}

	switch choice.Type {
	case provider.ToolChoiceAuto:
		return out, "auto", warnings
	case provider.ToolChoiceNone:
		return out, "none", warnings
	case provider.ToolChoiceRequired:
		return out, "required", warnings
	case provider.ToolChoiceTool:
		return out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": choice.ToolName,
			},
		}, warnings
	default:
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "toolChoice",
			Details: fmt.Sprintf("unsupported tool choice %q", choice.Type),
		})
		return out, nil, warnings
	}
}

func (m *model) convertResponseFormat(format *provider.ResponseFormat, opts OpenAIOptions) (*responseFormat, []provider.Warning) {
	if format == nil || format.Type == provider.ResponseFormatText {
		return nil, nil
	}
	if format.Type != provider.ResponseFormatJSON {
		return nil, []provider.Warning{{
			Type:    provider.WarnUnsupported,
			Feature: "responseFormat",
			Details: fmt.Sprintf("unsupported response format %q", format.Type),
		}}
	}

	if len(format.Schema) == 0 {
		return &responseFormat{Type: "json_object"}, nil
	}
	if !m.supportsStructuredOutputs {
		return &responseFormat{Type: "json_object"}, []provider.Warning{{
			Type:    provider.WarnUnsupported,
			Feature: "responseFormat",
			Details: "JSON schema response format requires WithStructuredOutputs(true); using json_object.",
		}}
	}

	strict := true
	if opts.StrictJSONSchema != nil {
		strict = *opts.StrictJSONSchema
	}
	name := format.Name
	if name == "" {
		name = "response"
	}
	return &responseFormat{
		Type: "json_schema",
		JSONSchema: &jsonSchemaFormat{
			Name:        name,
			Description: format.Description,
			Schema:      format.Schema,
			Strict:      strict,
		},
	}, nil
}

func reasoningEffort(reasoning *provider.ReasoningEffort, opts OpenAIOptions) string {
	if opts.ReasoningEffort != "" {
		return opts.ReasoningEffort
	}
	if reasoning == nil {
		return ""
	}
	switch *reasoning {
	case provider.ReasoningProviderDefault:
		return ""
	default:
		return string(*reasoning)
	}
}
