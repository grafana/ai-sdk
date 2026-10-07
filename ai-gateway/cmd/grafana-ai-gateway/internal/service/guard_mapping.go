package service

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type guardInput struct {
	Messages            []guardMessage `json:"messages,omitempty"`
	Tools               []guardTool    `json:"tools,omitempty"`
	SystemPrompt        string         `json:"system_prompt,omitempty"`
	Output              []guardMessage `json:"output,omitempty"`
	ConversationPreview string         `json:"conversation_preview,omitempty"`
}

type guardMessage struct {
	Role  provider.Role `json:"role"`
	Name  string        `json:"name,omitempty"`
	Parts []guardPart   `json:"parts,omitempty"`
}

type guardPartKind string

const (
	guardPartText       guardPartKind = "text"
	guardPartThinking   guardPartKind = "thinking"
	guardPartToolCall   guardPartKind = "tool_call"
	guardPartToolResult guardPartKind = "tool_result"
)

type guardPart struct {
	Kind       guardPartKind    `json:"kind"`
	Text       string           `json:"text,omitempty"`
	Thinking   string           `json:"thinking,omitempty"`
	ToolCall   *guardToolCall   `json:"tool_call,omitempty"`
	ToolResult *guardToolResult `json:"tool_result,omitempty"`
}

type guardToolCall struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	InputJSON json.RawMessage `json:"input_json,omitempty"`
}

type guardToolResult struct {
	ToolCallID  string          `json:"tool_call_id,omitempty"`
	Name        string          `json:"name,omitempty"`
	IsError     bool            `json:"is_error,omitempty"`
	Content     string          `json:"content,omitempty"`
	ContentJSON json.RawMessage `json:"content_json,omitempty"`
}

type guardTool struct {
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Type            provider.ToolType `json:"type,omitempty"`
	InputSchemaJSON []byte            `json:"input_schema_json,omitempty"`
	Deferred        bool              `json:"deferred,omitempty"`
}

type guardResponseInput struct {
	Messages            []guardResponseMessage `json:"messages,omitempty"`
	Tools               []guardTool            `json:"tools,omitempty"`
	SystemPrompt        string                 `json:"system_prompt,omitempty"`
	Output              []guardResponseMessage `json:"output,omitempty"`
	ConversationPreview string                 `json:"conversation_preview,omitempty"`
}

type guardResponseMessage struct {
	Role  provider.Role       `json:"role"`
	Name  string              `json:"name,omitempty"`
	Parts []guardResponsePart `json:"parts,omitempty"`
}

type guardResponsePart struct {
	Kind       guardPartKind            `json:"kind"`
	Text       string                   `json:"text,omitempty"`
	Thinking   string                   `json:"thinking,omitempty"`
	ToolCall   *guardResponseToolCall   `json:"tool_call,omitempty"`
	ToolResult *guardResponseToolResult `json:"tool_result,omitempty"`
}

type guardResponseToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	InputJSON []byte `json:"input_json,omitempty"`
}

type guardResponseToolResult struct {
	ToolCallID  string `json:"tool_call_id,omitempty"`
	Name        string `json:"name,omitempty"`
	IsError     bool   `json:"is_error,omitempty"`
	Content     string `json:"content,omitempty"`
	ContentJSON []byte `json:"content_json,omitempty"`
}

func makeGuardInputContext(ctx context.Context, options provider.CallOptions, output []provider.ContentPart) (guardInput, error) {
	var input guardInput
	if err := ctx.Err(); err != nil {
		return input, err
	}
	if len(options.ProviderOptions) > 0 || len(options.StopSequences) > 0 {
		return input, errGuardUnsupported
	}
	for name := range options.Headers {
		if err := ctx.Err(); err != nil {
			return guardInput{}, err
		}
		if !strings.EqualFold(name, "User-Agent") {
			return input, errGuardUnsupported
		}
	}
	if format := options.ResponseFormat; format != nil {
		if (format.Type != provider.ResponseFormatText && format.Type != provider.ResponseFormatJSON) || len(format.Schema) > 0 || format.Name != "" || format.Description != "" {
			return input, errGuardUnsupported
		}
	}
	if !guardValidToolChoice(ctx, options.ToolChoice, options.Tools) {
		return input, errGuardUnsupported
	}
	names := map[string]bool{}
	for _, tool := range options.Tools {
		if err := ctx.Err(); err != nil {
			return guardInput{}, err
		}
		if tool.Type != provider.ToolTypeFunction || !guardIdentity(tool.Name) || !utf8.ValidString(tool.Description) || names[tool.Name] || len(tool.InputExamples) > 0 || tool.ID != "" || len(tool.Args) > 0 || len(tool.ProviderOptions) > 0 {
			return guardInput{}, errGuardUnsupported
		}
		names[tool.Name] = true
		schema, err := guardJSONValueContext(ctx, tool.InputSchema)
		if err != nil {
			return guardInput{}, errGuardUnsupported
		}
		if _, ok := schema.(map[string]any); !ok {
			return guardInput{}, errGuardUnsupported
		}
		input.Tools = append(input.Tools, guardTool{Name: tool.Name, Description: tool.Description, Type: provider.ToolTypeFunction, InputSchemaJSON: append([]byte(nil), tool.InputSchema...)})
	}
	for _, message := range options.Prompt {
		mapped, err := guardMapMessage(ctx, message)
		if err != nil {
			return guardInput{}, err
		}
		input.Messages = append(input.Messages, mapped)
	}
	if output != nil {
		mapped, err := guardMapMessage(ctx, provider.NewAssistantMessage(output...))
		if err != nil {
			return guardInput{}, err
		}
		input.Output = []guardMessage{mapped}
	}
	raw, err := json.Marshal(input)
	if err != nil || !guardValidStrings(ctx, raw) {
		return guardInput{}, errGuardUnsupported
	}
	return input, nil
}

func guardIdentity(s string) bool { return s != "" && strings.TrimSpace(s) == s && utf8.ValidString(s) }

func guardValidToolChoice(ctx context.Context, choice *provider.ToolChoice, tools []provider.Tool) bool {
	if choice == nil {
		return true
	}
	switch choice.Type {
	case provider.ToolChoiceAuto, provider.ToolChoiceNone:
		return choice.ToolName == ""
	case provider.ToolChoiceRequired:
		return choice.ToolName == "" && len(tools) > 0
	case provider.ToolChoiceTool:
		for _, tool := range tools {
			if ctx.Err() != nil {
				return false
			}
			if tool.Name == choice.ToolName {
				return true
			}
		}
	}
	return false
}

func guardMapMessage(ctx context.Context, message provider.Message) (guardMessage, error) {
	mapped := guardMessage{Role: message.Role}
	if err := ctx.Err(); err != nil {
		return mapped, err
	}
	if len(message.ProviderOptions) > 0 {
		return mapped, errGuardUnsupported
	}
	switch message.Role {
	case provider.RoleSystem, provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
	default:
		return mapped, errGuardUnsupported
	}
	for _, part := range message.Content {
		p, err := guardMapPart(ctx, message.Role, part)
		if err != nil {
			return guardMessage{}, err
		}
		mapped.Parts = append(mapped.Parts, p)
	}
	return mapped, nil
}

func guardMapPart(ctx context.Context, role provider.Role, part provider.ContentPart) (guardPart, error) {
	var mapped guardPart
	if err := ctx.Err(); err != nil {
		return mapped, err
	}
	if !utf8.ValidString(part.Text) {
		return mapped, errGuardUnsupported
	}
	remainder := part
	remainder.Type = ""
	remainder.ProviderOptions = nil
	if len(part.ProviderOptions) > 0 && (part.Type != provider.ContentPartTypeReasoning || !guardReasoningMetadata(ctx, part.ProviderOptions) || part.Text == "") {
		return mapped, errGuardUnsupported
	}
	switch part.Type {
	case provider.ContentPartTypeText:
		if role == provider.RoleTool {
			return mapped, errGuardUnsupported
		}
		mapped.Kind = guardPartText
		mapped.Text = part.Text
		remainder.Text = ""
	case provider.ContentPartTypeReasoning:
		if role != provider.RoleAssistant {
			return mapped, errGuardUnsupported
		}
		mapped.Kind = guardPartThinking
		mapped.Thinking = part.Text
		remainder.Text = ""
	case provider.ContentPartTypeToolCall:
		if role != provider.RoleAssistant || !guardIdentity(part.ToolCallID) || !guardIdentity(part.ToolName) {
			return mapped, errGuardUnsupported
		}
		value, err := guardJSONValueContext(ctx, part.Input)
		if err != nil {
			return mapped, errGuardUnsupported
		}
		if _, ok := value.(map[string]any); !ok {
			return mapped, errGuardUnsupported
		}
		mapped.Kind = guardPartToolCall
		mapped.ToolCall = &guardToolCall{ID: part.ToolCallID, Name: part.ToolName, InputJSON: append(json.RawMessage(nil), part.Input...)}
		remainder.ToolCallID = ""
		remainder.ToolName = ""
		remainder.Input = nil
	case provider.ContentPartTypeToolResult:
		if (role != provider.RoleTool && role != provider.RoleAssistant) || !guardIdentity(part.ToolCallID) || !guardIdentity(part.ToolName) || part.Output == nil {
			return mapped, errGuardUnsupported
		}
		result, err := guardMapResult(ctx, part.Output)
		if err != nil {
			return mapped, err
		}
		result.ToolCallID = part.ToolCallID
		result.Name = part.ToolName
		mapped.Kind = guardPartToolResult
		mapped.ToolResult = &result
		remainder.ToolCallID = ""
		remainder.ToolName = ""
		remainder.Output = nil
	default:
		return mapped, errGuardUnsupported
	}
	if !reflect.DeepEqual(remainder, provider.ContentPart{}) {
		return guardPart{}, errGuardUnsupported
	}
	return mapped, nil
}

func guardReasoningMetadata(ctx context.Context, options provider.ProviderOptions) bool {
	if len(options) != 1 {
		return false
	}
	opt, ok := options["anthropic"].(provider.RawProviderOption)
	if !ok || opt.Key != "anthropic" {
		return false
	}
	value, err := guardJSONValueContext(ctx, opt.Raw)
	if err != nil {
		return false
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 1 {
		return false
	}
	signature, ok := object["signature"].(string)
	return ok && signature != ""
}

func guardMapResult(ctx context.Context, output *provider.ToolResultOutput) (guardToolResult, error) {
	var result guardToolResult
	if err := ctx.Err(); err != nil {
		return result, err
	}
	remainder := *output
	remainder.Type = ""
	remainder.ProviderOptions = nil
	if len(output.ProviderOptions) > 0 || !utf8.ValidString(output.Text) || !utf8.ValidString(output.Reason) {
		return result, errGuardUnsupported
	}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		result.Content = output.Text
		result.IsError = output.Type == provider.ToolOutputErrorText
		remainder.Text = ""
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		if _, err := guardJSONValueContext(ctx, output.JSON); err != nil {
			return result, errGuardUnsupported
		}
		result.ContentJSON = append(json.RawMessage(nil), output.JSON...)
		result.IsError = output.Type == provider.ToolOutputErrorJSON
		remainder.JSON = nil
	case provider.ToolOutputExecutionDenied:
		result.Content = output.Reason
		result.IsError = true
		remainder.Reason = ""
	case provider.ToolOutputContent:
		for _, value := range output.Content {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if value.Type != provider.ToolContentText || !utf8.ValidString(value.Text) || len(value.ProviderOptions) > 0 || value.Data != nil || value.MediaType != "" || value.Filename != nil {
				return result, errGuardUnsupported
			}
		}
		raw, err := json.Marshal(output.Content)
		if err != nil {
			return result, errGuardUnsupported
		}
		result.ContentJSON = raw
		remainder.Content = nil
	default:
		return result, errGuardUnsupported
	}
	if !reflect.DeepEqual(remainder, provider.ToolResultOutput{}) {
		return guardToolResult{}, errGuardUnsupported
	}
	return result, nil
}

func applyGuardTransformContext(ctx context.Context, options provider.CallOptions, original guardInput, raw json.RawMessage) (provider.CallOptions, error) {
	if err := ctx.Err(); err != nil {
		return provider.CallOptions{}, err
	}
	if original.Output != nil {
		return provider.CallOptions{}, errGuardTransform
	}
	current, err := makeGuardInputContext(ctx, options, nil)
	if err != nil {
		return provider.CallOptions{}, errGuardTransform
	}
	if !guardSameValue(ctx, reflect.ValueOf(original), reflect.ValueOf(current)) {
		return provider.CallOptions{}, errGuardTransform
	}
	var transformed guardResponseInput
	if decodeGuardJSONContext(ctx, raw, &transformed) != nil {
		return provider.CallOptions{}, errGuardTransform
	}
	if transformed.SystemPrompt != original.SystemPrompt || transformed.ConversationPreview != original.ConversationPreview || transformed.Output != nil || len(transformed.Messages) != len(original.Messages) || len(transformed.Tools) != len(original.Tools) {
		return provider.CallOptions{}, errGuardTransform
	}
	out, err := guardCloneOptions(ctx, options)
	if err != nil {
		return provider.CallOptions{}, err
	}
	for i, tool := range transformed.Tools {
		if err := ctx.Err(); err != nil {
			return provider.CallOptions{}, err
		}
		before := original.Tools[i]
		if tool.Name != before.Name || tool.Type != before.Type || tool.Deferred != before.Deferred || !guardSafeJSONContext(ctx, before.InputSchemaJSON, tool.InputSchemaJSON) {
			return provider.CallOptions{}, errGuardTransform
		}
		value, err := guardJSONValueContext(ctx, tool.InputSchemaJSON)
		if err != nil {
			return provider.CallOptions{}, errGuardTransform
		}
		if _, ok := value.(map[string]any); !ok {
			return provider.CallOptions{}, errGuardTransform
		}
		out.Tools[i].Description = tool.Description
		out.Tools[i].InputSchema = append(json.RawMessage(nil), tool.InputSchemaJSON...)
	}
	for i, message := range transformed.Messages {
		if err := ctx.Err(); err != nil {
			return provider.CallOptions{}, err
		}
		before := original.Messages[i]
		if message.Role != before.Role || message.Name != before.Name || len(message.Parts) != len(before.Parts) {
			return provider.CallOptions{}, errGuardTransform
		}
		for j, part := range message.Parts {
			next, err := guardTransformPart(ctx, out.Prompt[i].Content[j], before.Parts[j], part)
			if err != nil {
				return provider.CallOptions{}, err
			}
			out.Prompt[i].Content[j] = next
		}
	}
	if _, err := makeGuardInputContext(ctx, out, nil); err != nil {
		return provider.CallOptions{}, errGuardTransform
	}
	return out, nil
}

func guardTransformPart(ctx context.Context, source provider.ContentPart, before guardPart, after guardResponsePart) (provider.ContentPart, error) {
	if err := ctx.Err(); err != nil {
		return provider.ContentPart{}, err
	}
	if before.Kind != after.Kind {
		return provider.ContentPart{}, errGuardTransform
	}
	expected := guardResponsePart{Kind: before.Kind, Text: before.Text, Thinking: before.Thinking}
	switch before.Kind {
	case guardPartText:
		expected.Text = after.Text
		source.Text = after.Text
	case guardPartThinking:
	case guardPartToolCall:
		if after.ToolCall == nil || !guardSafeJSONContext(ctx, before.ToolCall.InputJSON, after.ToolCall.InputJSON) {
			return provider.ContentPart{}, errGuardTransform
		}
		expected.ToolCall = &guardResponseToolCall{ID: before.ToolCall.ID, Name: before.ToolCall.Name, InputJSON: after.ToolCall.InputJSON}
		source.Input = append(json.RawMessage(nil), after.ToolCall.InputJSON...)
	case guardPartToolResult:
		if after.ToolResult == nil {
			return provider.ContentPart{}, errGuardTransform
		}
		b, a := before.ToolResult, after.ToolResult
		expected.ToolResult = &guardResponseToolResult{ToolCallID: b.ToolCallID, Name: b.Name, IsError: b.IsError, Content: a.Content, ContentJSON: a.ContentJSON}
		switch source.Output.Type {
		case provider.ToolOutputText, provider.ToolOutputErrorText:
			if len(a.ContentJSON) > 0 {
				return provider.ContentPart{}, errGuardTransform
			}
			source.Output.Text = a.Content
		case provider.ToolOutputExecutionDenied:
			if len(a.ContentJSON) > 0 {
				return provider.ContentPart{}, errGuardTransform
			}
			source.Output.Reason = a.Content
		case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
			if a.Content != "" || !guardSafeJSONContext(ctx, b.ContentJSON, a.ContentJSON) {
				return provider.ContentPart{}, errGuardTransform
			}
			source.Output.JSON = append(json.RawMessage(nil), a.ContentJSON...)
		case provider.ToolOutputContent:
			if a.Content != "" {
				return provider.ContentPart{}, errGuardTransform
			}
			var values []struct {
				Type provider.ToolResultContentType `json:"type"`
				Text string                         `json:"text"`
			}
			if decodeGuardJSONContext(ctx, a.ContentJSON, &values) != nil || len(values) != len(source.Output.Content) {
				return provider.ContentPart{}, errGuardTransform
			}
			for i, value := range values {
				if err := ctx.Err(); err != nil {
					return provider.ContentPart{}, err
				}
				if value.Type != provider.ToolContentText {
					return provider.ContentPart{}, errGuardTransform
				}
				source.Output.Content[i].Text = value.Text
			}
		default:
			return provider.ContentPart{}, errGuardTransform
		}
	default:
		return provider.ContentPart{}, errGuardTransform
	}
	if !guardSameValue(ctx, reflect.ValueOf(expected), reflect.ValueOf(after)) {
		return provider.ContentPart{}, errGuardTransform
	}
	return source, nil
}

func guardSafeJSONContext(ctx context.Context, before, after []byte) bool {
	a, err := guardJSONValueContext(ctx, before)
	if err != nil {
		return false
	}
	b, err := guardJSONValueContext(ctx, after)
	if err != nil {
		return false
	}
	return guardSafeValue(ctx, a, b)
}

func guardSafeValue(ctx context.Context, before, after any) bool {
	if ctx.Err() != nil {
		return false
	}
	switch b := before.(type) {
	case json.Number:
		a, ok := after.(json.Number)
		if !ok {
			return false
		}
		return guardNumber(b) == guardNumber(a) && ctx.Err() == nil
	case string:
		_, ok := after.(string)
		return ok
	case map[string]any:
		a, ok := after.(map[string]any)
		if !ok {
			return false
		}
		if len(a) == 1 && a["redacted"] == "[REDACTED]" {
			return true
		}
		if len(a) != len(b) {
			return false
		}
		for key, value := range b {
			next, ok := a[key]
			if !ok || !guardSafeValue(ctx, value, next) {
				return false
			}
		}
		return true
	case []any:
		if after == "[REDACTED]" {
			return true
		}
		a, ok := after.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i, value := range b {
			if !guardSafeValue(ctx, value, a[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(before, after)
	}
}

func guardSameValue(ctx context.Context, before, after reflect.Value) bool {
	if ctx.Err() != nil {
		return false
	}
	if !before.IsValid() || !after.IsValid() {
		return before.IsValid() == after.IsValid()
	}
	if before.Type() != after.Type() {
		return false
	}
	switch before.Kind() {
	case reflect.Pointer:
		if before.IsNil() || after.IsNil() {
			return before.IsNil() == after.IsNil()
		}
		return guardSameValue(ctx, before.Elem(), after.Elem())
	case reflect.Struct:
		for i := 0; i < before.NumField(); i++ {
			if !guardSameValue(ctx, before.Field(i), after.Field(i)) {
				return false
			}
		}
		return true
	case reflect.String:
		left, right := before.String(), after.String()
		if len(left) != len(right) {
			return false
		}
		for i := 0; i < len(left); i += 4096 {
			end := min(i+4096, len(left))
			if ctx.Err() != nil || left[i:end] != right[i:end] {
				return false
			}
		}
		return true
	case reflect.Slice:
		if before.IsNil() != after.IsNil() || before.Len() != after.Len() {
			return false
		}
		if before.Type().Elem().Kind() == reflect.Uint8 {
			left, right := before.Bytes(), after.Bytes()
			for i := 0; i < len(left); i += 4096 {
				end := min(i+4096, len(left))
				if ctx.Err() != nil || !bytes.Equal(left[i:end], right[i:end]) {
					return false
				}
			}
			return true
		}
		for i := 0; i < before.Len(); i++ {
			if !guardSameValue(ctx, before.Index(i), after.Index(i)) {
				return false
			}
		}
		return true
	default:
		return before.Comparable() && before.Interface() == after.Interface()
	}
}

func guardClonePointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func guardCloneMap[M ~map[string]V, V any](in M) M {
	if in == nil {
		return nil
	}
	out := make(M, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func guardCloneMetadata(in provider.ProviderOptions) provider.ProviderOptions {
	out := guardCloneMap(in)
	for key, option := range in {
		raw := option.(provider.RawProviderOption)
		raw.Raw = slices.Clone(raw.Raw)
		out[key] = raw
	}
	return out
}

func guardCloneOptions(ctx context.Context, options provider.CallOptions) (provider.CallOptions, error) {
	if err := ctx.Err(); err != nil {
		return provider.CallOptions{}, err
	}
	out := options
	out.Headers = guardCloneMap(options.Headers)
	out.ProviderOptions = guardCloneMetadata(options.ProviderOptions)
	out.StopSequences = slices.Clone(options.StopSequences)
	out.ToolChoice = guardClonePointer(options.ToolChoice)
	out.MaxOutputTokens = guardClonePointer(options.MaxOutputTokens)
	out.Temperature = guardClonePointer(options.Temperature)
	out.TopP = guardClonePointer(options.TopP)
	out.TopK = guardClonePointer(options.TopK)
	out.PresencePenalty = guardClonePointer(options.PresencePenalty)
	out.FrequencyPenalty = guardClonePointer(options.FrequencyPenalty)
	out.Seed = guardClonePointer(options.Seed)
	out.ResponseFormat = guardClonePointer(options.ResponseFormat)
	if out.ResponseFormat != nil {
		out.ResponseFormat.Schema = slices.Clone(options.ResponseFormat.Schema)
	}
	out.Prompt = slices.Clone(options.Prompt)
	for i, message := range options.Prompt {
		if err := ctx.Err(); err != nil {
			return provider.CallOptions{}, err
		}
		out.Prompt[i].ProviderOptions = guardCloneMetadata(message.ProviderOptions)
		out.Prompt[i].Content = slices.Clone(message.Content)
		for j, part := range message.Content {
			if err := ctx.Err(); err != nil {
				return provider.CallOptions{}, err
			}
			next := part
			next.Input = slices.Clone(part.Input)
			next.ProviderOptions = guardCloneMetadata(part.ProviderOptions)
			if part.Output != nil {
				next.Output = guardClonePointer(part.Output)
				next.Output.JSON = slices.Clone(part.Output.JSON)
				next.Output.Content = slices.Clone(part.Output.Content)
				next.Output.ProviderOptions = guardCloneMetadata(part.Output.ProviderOptions)
				for k, value := range part.Output.Content {
					if err := ctx.Err(); err != nil {
						return provider.CallOptions{}, err
					}
					next.Output.Content[k].ProviderOptions = guardCloneMetadata(value.ProviderOptions)
				}
			}
			out.Prompt[i].Content[j] = next
		}
	}
	out.Tools = slices.Clone(options.Tools)
	for i, tool := range options.Tools {
		if err := ctx.Err(); err != nil {
			return provider.CallOptions{}, err
		}
		out.Tools[i].ProviderOptions = guardCloneMetadata(tool.ProviderOptions)
		out.Tools[i].Args = guardCloneMap(tool.Args)
		out.Tools[i].InputSchema = slices.Clone(tool.InputSchema)
		out.Tools[i].InputExamples = slices.Clone(tool.InputExamples)
		out.Tools[i].Strict = guardClonePointer(tool.Strict)
	}
	return out, ctx.Err()
}
