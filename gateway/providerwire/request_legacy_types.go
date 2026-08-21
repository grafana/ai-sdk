package providerwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/grafana/ai-sdk/provider"
)

type legacyNumberKind uint8

const (
	legacyNumberInvalid legacyNumberKind = iota
	legacyNumberInteger
	legacyNumberFloat
)

type legacyNumber struct {
	kind    legacyNumberKind
	integer int64
	float   float64
}

func legacyNumberFromProvider(number provider.LanguageModelNumber) (legacyNumber, error) {
	if integer, ok := number.Int64(); ok {
		return legacyNumber{kind: legacyNumberInteger, integer: integer}, nil
	}
	if floating, ok := number.Float64(); ok && !math.IsNaN(floating) && !math.IsInf(floating, 0) {
		return legacyNumber{kind: legacyNumberFloat, float: floating}, nil
	}
	return legacyNumber{}, errors.New("invalid language model number")
}

func (number legacyNumber) toProvider() (provider.LanguageModelNumber, error) {
	switch number.kind {
	case legacyNumberInteger:
		return provider.LanguageModelNumberFromInt64(number.integer), nil
	case legacyNumberFloat:
		return provider.LanguageModelNumberFromFloat64(number.float)
	default:
		return provider.LanguageModelNumber{}, errors.New("invalid language model number")
	}
}

func legacyNumberPointerFromProvider(number *provider.LanguageModelNumber) (*legacyNumber, error) {
	if number == nil {
		return nil, nil
	}
	mapped, err := legacyNumberFromProvider(*number)
	if err != nil {
		return nil, err
	}
	return &mapped, nil
}

func providerNumberPointerFromLegacy(number *legacyNumber) (*provider.LanguageModelNumber, error) {
	if number == nil {
		return nil, nil
	}
	mapped, err := number.toProvider()
	if err != nil {
		return nil, err
	}
	return &mapped, nil
}

func (number legacyNumber) MarshalJSON() ([]byte, error) {
	switch number.kind {
	case legacyNumberInteger:
		return strconv.AppendInt(nil, number.integer, 10), nil
	case legacyNumberFloat:
		if math.IsNaN(number.float) || math.IsInf(number.float, 0) {
			return nil, errors.New("non-finite language model number")
		}
		return json.Marshal(number.float)
	default:
		return nil, errors.New("invalid language model number")
	}
}

func (number *legacyNumber) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("language model number contains trailing data")
	}
	token, ok := value.(json.Number)
	if !ok {
		return errors.New("language model number must be numeric")
	}
	if isLegacyPlainInteger(token.String()) {
		if integer, err := strconv.ParseInt(token.String(), 10, 64); err == nil {
			*number = legacyNumber{kind: legacyNumberInteger, integer: integer}
			return nil
		}
	}
	floating, err := strconv.ParseFloat(token.String(), 64)
	if err != nil || math.IsNaN(floating) || math.IsInf(floating, 0) {
		return errors.New("language model number must be finite")
	}
	converted, err := provider.LanguageModelNumberFromFloat64(floating)
	if err != nil {
		return err
	}
	mapped, err := legacyNumberFromProvider(converted)
	if err != nil {
		return err
	}
	*number = mapped
	return nil
}

func isLegacyPlainInteger(value string) bool {
	if value == "0" || value == "-0" {
		return true
	}
	start := 0
	if len(value) > 0 && value[0] == '-' {
		start = 1
	}
	if start == len(value) || value[start] < '1' || value[start] > '9' {
		return false
	}
	for index := start + 1; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

type legacyCallOptions struct {
	Prompt           []legacyMessage            `json:"prompt,omitempty"`
	Tools            []legacyTool               `json:"tools,omitempty"`
	ToolChoice       *legacyToolChoice          `json:"toolChoice,omitempty"`
	MaxOutputTokens  *legacyNumber              `json:"maxOutputTokens,omitempty"`
	Temperature      *float64                   `json:"temperature,omitempty"`
	TopP             *float64                   `json:"topP,omitempty"`
	TopK             *legacyNumber              `json:"topK,omitempty"`
	PresencePenalty  *float64                   `json:"presencePenalty,omitempty"`
	FrequencyPenalty *float64                   `json:"frequencyPenalty,omitempty"`
	StopSequences    []string                   `json:"stopSequences,omitempty"`
	ResponseFormat   *legacyResponseFormat      `json:"responseFormat,omitempty"`
	Seed             *legacyNumber              `json:"seed,omitempty"`
	Reasoning        *provider.ReasoningEffort  `json:"reasoning,omitempty"`
	IncludeRawChunks *bool                      `json:"includeRawChunks,omitempty"`
	Headers          map[string]string          `json:"headers,omitempty"`
	ProviderOptions  map[string]json.RawMessage `json:"providerOptions,omitempty"`
}

type legacyResponseFormat struct {
	Type        provider.ResponseFormatType `json:"type"`
	Schema      json.RawMessage             `json:"schema,omitempty"`
	Name        *string                     `json:"name,omitempty"`
	Description *string                     `json:"description,omitempty"`
}

type legacyTool struct {
	Type            provider.ToolType          `json:"type"`
	Name            string                     `json:"name"`
	Description     *string                    `json:"description,omitempty"`
	InputSchema     json.RawMessage            `json:"inputSchema,omitempty"`
	InputExamples   []provider.InputExample    `json:"inputExamples,omitempty"`
	Strict          *bool                      `json:"strict,omitempty"`
	ID              string                     `json:"id,omitempty"`
	Args            map[string]json.RawMessage `json:"args,omitempty"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions,omitempty"`
}

type legacyToolChoice struct {
	Type     provider.ToolChoiceType `json:"type"`
	ToolName string                  `json:"toolName,omitempty"`
}

type legacyMessage struct {
	Role            provider.Role              `json:"role"`
	Content         []legacyContentPart        `json:"content"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions,omitempty"`
}

func (message legacyMessage) MarshalJSON() ([]byte, error) {
	if message.Role != provider.RoleSystem {
		type alias legacyMessage
		return json.Marshal(alias(message))
	}
	var text string
	for _, part := range message.Content {
		if part.Type == provider.ContentPartTypeText {
			text += part.Text
		}
	}
	return json.Marshal(struct {
		Role            provider.Role              `json:"role"`
		Content         string                     `json:"content"`
		ProviderOptions map[string]json.RawMessage `json:"providerOptions,omitempty"`
	}{Role: message.Role, Content: text, ProviderOptions: message.ProviderOptions})
}

func (message *legacyMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role            provider.Role              `json:"role"`
		Content         json.RawMessage            `json:"content"`
		ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	message.Role = raw.Role
	message.ProviderOptions = raw.ProviderOptions
	message.Content = nil
	trimmed := bytes.TrimSpace(raw.Content)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		if raw.Role != provider.RoleSystem {
			return fmt.Errorf("string message content is only valid for the system role, got role %q", raw.Role)
		}
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		message.Content = []legacyContentPart{{Type: provider.ContentPartTypeText, Text: text}}
		return nil
	}
	return json.Unmarshal(trimmed, &message.Content)
}

type legacyContentPart struct {
	Type             provider.ContentPartType   `json:"type"`
	Text             string                     `json:"text,omitempty"`
	Data             *legacyDataContent         `json:"data,omitempty"`
	Filename         *string                    `json:"filename,omitempty"`
	MediaType        string                     `json:"mediaType,omitempty"`
	Kind             string                     `json:"kind,omitempty"`
	SourceType       provider.SourceType        `json:"sourceType,omitempty"`
	ID               string                     `json:"id,omitempty"`
	URL              string                     `json:"url,omitempty"`
	Title            string                     `json:"title,omitempty"`
	ToolCallID       string                     `json:"toolCallId,omitempty"`
	ToolName         string                     `json:"toolName,omitempty"`
	Input            json.RawMessage            `json:"input,omitempty"`
	Output           *legacyToolResultOutput    `json:"output,omitempty"`
	ProviderExecuted *bool                      `json:"providerExecuted,omitempty"`
	ApprovalID       string                     `json:"approvalId,omitempty"`
	Signature        string                     `json:"signature,omitempty"`
	IsAutomatic      bool                       `json:"isAutomatic,omitempty"`
	Approved         *bool                      `json:"approved,omitempty"`
	Reason           *string                    `json:"reason,omitempty"`
	ProviderOptions  map[string]json.RawMessage `json:"providerOptions,omitempty"`
}

func legacyCallOptionsFromProvider(options provider.CallOptions) (legacyCallOptions, error) {
	legacy := legacyCallOptions{
		Temperature: clonePointer(options.Temperature), TopP: clonePointer(options.TopP),
		PresencePenalty: clonePointer(options.PresencePenalty), FrequencyPenalty: clonePointer(options.FrequencyPenalty),
		StopSequences: cloneSlice(options.StopSequences), Reasoning: clonePointer(options.Reasoning),
		IncludeRawChunks: clonePointer(options.IncludeRawChunks), Headers: cloneStringMap(options.Headers),
	}
	var err error
	legacy.ProviderOptions, err = legacyProviderOptionsFromProvider(options.ProviderOptions)
	if err != nil {
		return legacyCallOptions{}, err
	}
	legacy.MaxOutputTokens, err = legacyNumberPointerFromProvider(options.MaxOutputTokens)
	if err != nil {
		return legacyCallOptions{}, fmt.Errorf("maxOutputTokens: %w", err)
	}
	legacy.TopK, err = legacyNumberPointerFromProvider(options.TopK)
	if err != nil {
		return legacyCallOptions{}, fmt.Errorf("topK: %w", err)
	}
	legacy.Seed, err = legacyNumberPointerFromProvider(options.Seed)
	if err != nil {
		return legacyCallOptions{}, fmt.Errorf("seed: %w", err)
	}
	if options.Prompt != nil {
		legacy.Prompt = make([]legacyMessage, len(options.Prompt))
	}
	for index, message := range options.Prompt {
		mapped, err := legacyMessageFromProvider(message)
		if err != nil {
			return legacyCallOptions{}, fmt.Errorf("prompt %d: %w", index, err)
		}
		legacy.Prompt[index] = mapped
	}
	if options.Tools != nil {
		legacy.Tools = make([]legacyTool, len(options.Tools))
	}
	for index, tool := range options.Tools {
		mapped, err := legacyToolFromProvider(tool)
		if err != nil {
			return legacyCallOptions{}, fmt.Errorf("tool %d: %w", index, err)
		}
		legacy.Tools[index] = mapped
	}
	if options.ToolChoice != nil {
		legacy.ToolChoice = &legacyToolChoice{Type: options.ToolChoice.Type, ToolName: options.ToolChoice.ToolName}
	}
	if options.ResponseFormat != nil {
		legacy.ResponseFormat = &legacyResponseFormat{
			Type: options.ResponseFormat.Type, Schema: append(json.RawMessage(nil), options.ResponseFormat.Schema...),
			Name: clonePointer(options.ResponseFormat.Name), Description: clonePointer(options.ResponseFormat.Description),
		}
	}
	return legacy, nil
}

func (legacy legacyCallOptions) toProvider() (provider.CallOptions, error) {
	options := provider.CallOptions{
		Temperature: clonePointer(legacy.Temperature), TopP: clonePointer(legacy.TopP),
		PresencePenalty: clonePointer(legacy.PresencePenalty), FrequencyPenalty: clonePointer(legacy.FrequencyPenalty),
		StopSequences: cloneSlice(legacy.StopSequences), Reasoning: clonePointer(legacy.Reasoning),
		IncludeRawChunks: clonePointer(legacy.IncludeRawChunks), Headers: cloneStringMap(legacy.Headers),
		ProviderOptions: legacyProviderOptionsToProvider(legacy.ProviderOptions),
	}
	var err error
	options.MaxOutputTokens, err = providerNumberPointerFromLegacy(legacy.MaxOutputTokens)
	if err != nil {
		return provider.CallOptions{}, fmt.Errorf("maxOutputTokens: %w", err)
	}
	options.TopK, err = providerNumberPointerFromLegacy(legacy.TopK)
	if err != nil {
		return provider.CallOptions{}, fmt.Errorf("topK: %w", err)
	}
	options.Seed, err = providerNumberPointerFromLegacy(legacy.Seed)
	if err != nil {
		return provider.CallOptions{}, fmt.Errorf("seed: %w", err)
	}
	if legacy.Prompt != nil {
		options.Prompt = make([]provider.Message, len(legacy.Prompt))
	}
	for index, message := range legacy.Prompt {
		mapped, err := message.toProvider()
		if err != nil {
			return provider.CallOptions{}, fmt.Errorf("prompt %d: %w", index, err)
		}
		options.Prompt[index] = mapped
	}
	if legacy.Tools != nil {
		options.Tools = make([]provider.Tool, len(legacy.Tools))
	}
	for index, tool := range legacy.Tools {
		options.Tools[index] = tool.toProvider()
	}
	if legacy.ToolChoice != nil {
		options.ToolChoice = &provider.ToolChoice{Type: legacy.ToolChoice.Type, ToolName: legacy.ToolChoice.ToolName}
	}
	if legacy.ResponseFormat != nil {
		options.ResponseFormat = &provider.ResponseFormat{
			Type: legacy.ResponseFormat.Type, Schema: append(json.RawMessage(nil), legacy.ResponseFormat.Schema...),
			Name: clonePointer(legacy.ResponseFormat.Name), Description: clonePointer(legacy.ResponseFormat.Description),
		}
	}
	return options, nil
}

func legacyMessageFromProvider(message provider.Message) (legacyMessage, error) {
	legacy := legacyMessage{Role: message.Role}
	var err error
	legacy.ProviderOptions, err = legacyProviderOptionsFromProvider(message.ProviderOptions)
	if err != nil {
		return legacyMessage{}, err
	}
	if message.Content != nil {
		legacy.Content = make([]legacyContentPart, len(message.Content))
	}
	for index, part := range message.Content {
		mapped, err := legacyContentPartFromProvider(part)
		if err != nil {
			return legacyMessage{}, fmt.Errorf("content %d: %w", index, err)
		}
		legacy.Content[index] = mapped
	}
	return legacy, nil
}

func (message legacyMessage) toProvider() (provider.Message, error) {
	mapped := provider.Message{Role: message.Role, ProviderOptions: legacyProviderOptionsToProvider(message.ProviderOptions)}
	if message.Content != nil {
		mapped.Content = make([]provider.ContentPart, len(message.Content))
	}
	for index, part := range message.Content {
		value, err := part.toProvider()
		if err != nil {
			return provider.Message{}, fmt.Errorf("content %d: %w", index, err)
		}
		mapped.Content[index] = value
	}
	return mapped, nil
}

func legacyContentPartFromProvider(part provider.ContentPart) (legacyContentPart, error) {
	legacy := legacyContentPart{
		Type: part.Type, Text: part.Text, MediaType: part.MediaType, Kind: part.Kind,
		SourceType: part.SourceType, ID: part.ID, URL: part.URL, Title: part.Title,
		ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: append(json.RawMessage(nil), part.Input...),
		ProviderExecuted: clonePointer(part.ProviderExecuted), ApprovalID: part.ApprovalID, Signature: part.Signature,
		IsAutomatic: part.IsAutomatic, Approved: clonePointer(part.Approved), Reason: clonePointer(part.Reason),
	}
	var err error
	legacy.ProviderOptions, err = legacyProviderOptionsFromProvider(part.ProviderOptions)
	if err != nil {
		return legacyContentPart{}, err
	}
	if part.Type == provider.ContentPartTypeFile {
		if part.FilePartFilename != nil && part.Filename != "" {
			return legacyContentPart{}, errors.New("file part has both request and response filenames")
		}
		if part.FilePartFilename != nil {
			legacy.Filename = clonePointer(part.FilePartFilename)
		} else if part.Filename != "" {
			legacy.Filename = &part.Filename
		}
	} else {
		if part.FilePartFilename != nil {
			return legacyContentPart{}, fmt.Errorf("request file filename is invalid for content type %q", part.Type)
		}
		if part.Filename != "" {
			legacy.Filename = &part.Filename
		}
	}
	if part.Data != nil {
		data, err := legacyDataContentFromProvider(*part.Data)
		if err != nil {
			return legacyContentPart{}, err
		}
		legacy.Data = &data
	}
	if part.Output != nil {
		output, err := legacyToolResultOutputFromProvider(*part.Output)
		if err != nil {
			return legacyContentPart{}, err
		}
		legacy.Output = &output
	}
	return legacy, nil
}

func (legacy legacyContentPart) toProvider() (provider.ContentPart, error) {
	part := provider.ContentPart{
		Type: legacy.Type, Text: legacy.Text, MediaType: legacy.MediaType, Kind: legacy.Kind,
		SourceType: legacy.SourceType, ID: legacy.ID, URL: legacy.URL, Title: legacy.Title,
		ToolCallID: legacy.ToolCallID, ToolName: legacy.ToolName, Input: append(json.RawMessage(nil), legacy.Input...),
		ProviderExecuted: clonePointer(legacy.ProviderExecuted), ApprovalID: legacy.ApprovalID, Signature: legacy.Signature,
		IsAutomatic: legacy.IsAutomatic, Approved: clonePointer(legacy.Approved), Reason: clonePointer(legacy.Reason),
		ProviderOptions: legacyProviderOptionsToProvider(legacy.ProviderOptions),
	}
	if legacy.Filename != nil {
		if legacy.Type == provider.ContentPartTypeFile {
			part.FilePartFilename = clonePointer(legacy.Filename)
		} else {
			part.Filename = *legacy.Filename
		}
	}
	if legacy.Data != nil {
		data, err := legacy.Data.toProvider()
		if err != nil {
			return provider.ContentPart{}, err
		}
		part.Data = &data
	}
	if legacy.Output != nil {
		output, err := legacy.Output.toProvider()
		if err != nil {
			return provider.ContentPart{}, err
		}
		part.Output = &output
	}
	return part, nil
}

func legacyToolFromProvider(tool provider.Tool) (legacyTool, error) {
	options, err := legacyProviderOptionsFromProvider(tool.ProviderOptions)
	if err != nil {
		return legacyTool{}, err
	}
	return legacyTool{
		Type: tool.Type, Name: tool.Name, Description: clonePointer(tool.Description),
		InputSchema: append(json.RawMessage(nil), tool.InputSchema...), InputExamples: cloneSlice(tool.InputExamples),
		Strict: clonePointer(tool.Strict), ID: tool.ID, Args: cloneRawMap(tool.Args), ProviderOptions: options,
	}, nil
}

func (tool legacyTool) toProvider() provider.Tool {
	return provider.Tool{
		Type: tool.Type, Name: tool.Name, Description: clonePointer(tool.Description),
		InputSchema: append(json.RawMessage(nil), tool.InputSchema...), InputExamples: cloneSlice(tool.InputExamples),
		Strict: clonePointer(tool.Strict), ID: tool.ID, Args: cloneRawMap(tool.Args),
		ProviderOptions: legacyProviderOptionsToProvider(tool.ProviderOptions),
	}
}

func legacyProviderOptionsFromProvider(options provider.ProviderOptions) (map[string]json.RawMessage, error) {
	if options == nil {
		return nil, nil
	}
	mapped := make(map[string]json.RawMessage, len(options))
	for key, option := range options {
		if raw, ok := option.(provider.RawProviderOption); ok {
			if len(raw.Raw) == 0 {
				mapped[key] = json.RawMessage("null")
				continue
			}
			if !json.Valid(raw.Raw) {
				return nil, fmt.Errorf("provider option %q contains invalid JSON", key)
			}
			mapped[key] = append(json.RawMessage(nil), raw.Raw...)
			continue
		}
		encoded, err := json.Marshal(provider.ProviderOptions{key: option})
		if err != nil {
			return nil, fmt.Errorf("provider option %q: %w", key, err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &object); err != nil {
			return nil, fmt.Errorf("provider option %q: %w", key, err)
		}
		mapped[key] = append(json.RawMessage(nil), object[key]...)
	}
	return mapped, nil
}

func legacyProviderOptionsToProvider(options map[string]json.RawMessage) provider.ProviderOptions {
	if options == nil {
		return nil
	}
	mapped := make(provider.ProviderOptions, len(options))
	for key, value := range options {
		mapped[key] = provider.RawProviderOption{Key: key, Raw: append(json.RawMessage(nil), value...)}
	}
	return mapped
}

func cloneSlice[T any](value []T) []T {
	if value == nil {
		return nil
	}
	cloned := make([]T, len(value))
	copy(cloned, value)
	return cloned
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	copy := make(map[string]string, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}

func cloneRawMap(value map[string]json.RawMessage) map[string]json.RawMessage {
	if value == nil {
		return nil
	}
	copy := make(map[string]json.RawMessage, len(value))
	for key, item := range value {
		copy[key] = append(json.RawMessage(nil), item...)
	}
	return copy
}
