package providerwirev4

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type callOptionsDTO struct {
	Prompt           []json.RawMessage  `json:"prompt"`
	Tools            *[]toolDTO         `json:"tools,omitempty"`
	ToolChoice       *toolChoiceDTO     `json:"toolChoice,omitempty"`
	MaxOutputTokens  *int               `json:"maxOutputTokens,omitempty"`
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"topP,omitempty"`
	TopK             *int               `json:"topK,omitempty"`
	PresencePenalty  *float64           `json:"presencePenalty,omitempty"`
	FrequencyPenalty *float64           `json:"frequencyPenalty,omitempty"`
	StopSequences    *[]string          `json:"stopSequences,omitempty"`
	ResponseFormat   *responseFormatDTO `json:"responseFormat,omitempty"`
	Seed             *int               `json:"seed,omitempty"`
	Reasoning        *string            `json:"reasoning,omitempty"`
	IncludeRawChunks bool               `json:"includeRawChunks,omitempty"`
	Headers          map[string]string  `json:"headers,omitempty"`
	ProviderOptions  providerOptionsDTO `json:"providerOptions,omitempty"`
}

type toolChoiceDTO struct {
	Type     string `json:"type"`
	ToolName string `json:"toolName,omitempty"`
}

type responseFormatDTO struct {
	Type        string          `json:"type"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
}

// EncodeCallOptions encodes canonical LanguageModelV4 call options without
// invoking provider CallOptions or nested polymorphic JSON methods. An empty
// top-level providerOptions.gateway object is omitted; non-empty and nested
// gateway namespaces are reserved for gateway policy and are unsupported.
func EncodeCallOptions(options provider.CallOptions) ([]byte, error) {
	dto, err := encodeCallOptions(options)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: encoding call options: %w", err)
	}
	return data, nil
}

func decodeCallOptionsJSON(data []byte) (provider.CallOptions, error) {
	object, err := decodeObject(data, "call options")
	if err != nil {
		return provider.CallOptions{}, err
	}
	if _, err := requireField(object, "prompt", "call options"); err != nil {
		return provider.CallOptions{}, err
	}
	if _, exists := object["abortSignal"]; exists {
		return provider.CallOptions{}, errors.New("providerwirev4: abortSignal is transport-private and is not supported")
	}
	if err := rejectUnknownFields(object, "call options", "prompt", "tools", "toolChoice", "maxOutputTokens", "temperature", "topP", "topK", "presencePenalty", "frequencyPenalty", "stopSequences", "responseFormat", "seed", "reasoning", "includeRawChunks", "headers", "providerOptions"); err != nil {
		return provider.CallOptions{}, err
	}
	if err := rejectNullFields(object, "call options", "tools", "toolChoice", "maxOutputTokens", "temperature", "topP", "topK", "presencePenalty", "frequencyPenalty", "stopSequences", "responseFormat", "seed", "reasoning", "includeRawChunks", "headers", "providerOptions"); err != nil {
		return provider.CallOptions{}, err
	}
	if raw, exists := object["stopSequences"]; exists {
		if err := validateStringArray(raw, "stopSequences"); err != nil {
			return provider.CallOptions{}, err
		}
	}
	if raw, exists := object["headers"]; exists {
		if err := validateStringMap(raw, "headers"); err != nil {
			return provider.CallOptions{}, err
		}
	}
	for field, context := range map[string]string{"toolChoice": "tool choice", "responseFormat": "response format"} {
		raw, exists := object[field]
		if !exists {
			continue
		}
		nested, err := decodeObject(raw, context)
		if err != nil {
			return provider.CallOptions{}, err
		}
		variant, err := decodeRequiredString(nested, "type", context)
		if err != nil {
			return provider.CallOptions{}, err
		}
		knownFields := []string{"type", "toolName"}
		if field == "responseFormat" {
			knownFields = []string{"type", "schema", "name", "description"}
		}
		if err := rejectUnknownFields(nested, context, knownFields...); err != nil {
			return provider.CallOptions{}, err
		}
		fields := []string{"type"}
		if field == "toolChoice" && provider.ToolChoiceType(variant) == provider.ToolChoiceTool {
			fields = append(fields, "toolName")
		}
		if field == "responseFormat" && provider.ResponseFormatType(variant) == provider.ResponseFormatJSON {
			fields = append(fields, "schema", "name", "description")
		}
		if err := rejectNullFields(nested, context, fields...); err != nil {
			return provider.CallOptions{}, err
		}
		selected := make(map[string]json.RawMessage, len(fields))
		for _, selectedField := range fields {
			if value, ok := nested[selectedField]; ok {
				selected[selectedField] = value
			}
		}
		object[field], err = json.Marshal(selected)
		if err != nil {
			return provider.CallOptions{}, err
		}
	}
	data, err = json.Marshal(object)
	if err != nil {
		return provider.CallOptions{}, err
	}
	var dto callOptionsDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return provider.CallOptions{}, fmt.Errorf("providerwirev4: decoding call options: %w", err)
	}
	return decodeCallOptions(dto)
}

func encodeCallOptions(options provider.CallOptions) (callOptionsDTO, error) {
	prompt := make([]json.RawMessage, len(options.Prompt))
	for i, message := range options.Prompt {
		encoded, err := encodeMessage(message)
		if err != nil {
			return callOptionsDTO{}, fmt.Errorf("providerwirev4: encoding prompt message %d: %w", i, err)
		}
		prompt[i] = encoded
	}
	var tools *[]toolDTO
	if options.Tools != nil {
		values := make([]toolDTO, len(options.Tools))
		for i, tool := range options.Tools {
			encoded, err := encodeTool(tool)
			if err != nil {
				return callOptionsDTO{}, fmt.Errorf("providerwirev4: encoding tool %d: %w", i, err)
			}
			values[i] = encoded
		}
		tools = &values
	}
	var stopSequences *[]string
	if options.StopSequences != nil {
		values := append([]string{}, options.StopSequences...)
		stopSequences = &values
	}
	providerOptions, err := encodeProviderOptions(options.ProviderOptions)
	if err != nil {
		return callOptionsDTO{}, err
	}
	providerOptions, err = validateAndRemoveGatewayOptions(providerOptions)
	if err != nil {
		return callOptionsDTO{}, err
	}

	dto := callOptionsDTO{
		Prompt: prompt, Tools: tools,
		MaxOutputTokens: options.MaxOutputTokens, Temperature: options.Temperature,
		TopP: options.TopP, TopK: options.TopK, PresencePenalty: options.PresencePenalty,
		FrequencyPenalty: options.FrequencyPenalty, StopSequences: stopSequences,
		Seed: options.Seed, IncludeRawChunks: options.IncludeRawChunks,
		Headers: options.Headers, ProviderOptions: providerOptions,
	}
	if options.ToolChoice != nil {
		dto.ToolChoice = &toolChoiceDTO{Type: string(options.ToolChoice.Type)}
		if options.ToolChoice.Type == provider.ToolChoiceTool {
			dto.ToolChoice.ToolName = options.ToolChoice.ToolName
		}
		if err := validateToolChoice(*dto.ToolChoice); err != nil {
			return callOptionsDTO{}, err
		}
	}
	if options.ResponseFormat != nil {
		dto.ResponseFormat = &responseFormatDTO{Type: string(options.ResponseFormat.Type)}
		if options.ResponseFormat.Type == provider.ResponseFormatJSON {
			if len(options.ResponseFormat.Schema) > 0 {
				if err := validateJSONObject(options.ResponseFormat.Schema, "response format schema"); err != nil {
					return callOptionsDTO{}, err
				}
			}
			dto.ResponseFormat.Schema = append(json.RawMessage(nil), options.ResponseFormat.Schema...)
			dto.ResponseFormat.Name = options.ResponseFormat.Name
			dto.ResponseFormat.Description = options.ResponseFormat.Description
		}
		if err := validateResponseFormat(*dto.ResponseFormat); err != nil {
			return callOptionsDTO{}, err
		}
	}
	if options.Reasoning != nil {
		value := string(*options.Reasoning)
		dto.Reasoning = &value
		if err := validateReasoning(value); err != nil {
			return callOptionsDTO{}, err
		}
	}
	return dto, nil
}

func decodeCallOptions(dto callOptionsDTO) (provider.CallOptions, error) {
	prompt := make([]provider.Message, len(dto.Prompt))
	for i, message := range dto.Prompt {
		decoded, err := decodeMessage(message)
		if err != nil {
			return provider.CallOptions{}, fmt.Errorf("providerwirev4: decoding prompt message %d: %w", i, err)
		}
		prompt[i] = decoded
	}
	var tools []provider.Tool
	if dto.Tools != nil {
		tools = make([]provider.Tool, len(*dto.Tools))
		for i, tool := range *dto.Tools {
			decoded, err := decodeTool(tool)
			if err != nil {
				return provider.CallOptions{}, fmt.Errorf("providerwirev4: decoding tool %d: %w", i, err)
			}
			tools[i] = decoded
		}
	}
	var stopSequences []string
	if dto.StopSequences != nil {
		stopSequences = append([]string{}, (*dto.StopSequences)...)
	}
	encodedProviderOptions, err := validateAndRemoveGatewayOptions(dto.ProviderOptions)
	if err != nil {
		return provider.CallOptions{}, err
	}
	providerOptions, err := decodeProviderOptions(encodedProviderOptions)
	if err != nil {
		return provider.CallOptions{}, err
	}

	options := provider.CallOptions{
		Prompt: prompt, Tools: tools,
		MaxOutputTokens: dto.MaxOutputTokens, Temperature: dto.Temperature,
		TopP: dto.TopP, TopK: dto.TopK, PresencePenalty: dto.PresencePenalty,
		FrequencyPenalty: dto.FrequencyPenalty, StopSequences: stopSequences,
		Seed: dto.Seed, IncludeRawChunks: dto.IncludeRawChunks,
		Headers: dto.Headers, ProviderOptions: providerOptions,
	}
	if dto.ToolChoice != nil {
		if err := validateToolChoice(*dto.ToolChoice); err != nil {
			return provider.CallOptions{}, err
		}
		options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceType(dto.ToolChoice.Type), ToolName: dto.ToolChoice.ToolName}
	}
	if dto.ResponseFormat != nil {
		if err := validateResponseFormat(*dto.ResponseFormat); err != nil {
			return provider.CallOptions{}, err
		}
		if len(dto.ResponseFormat.Schema) > 0 {
			if err := validateJSONObject(dto.ResponseFormat.Schema, "response format schema"); err != nil {
				return provider.CallOptions{}, err
			}
		}
		options.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatType(dto.ResponseFormat.Type), Schema: append(json.RawMessage(nil), dto.ResponseFormat.Schema...), Name: dto.ResponseFormat.Name, Description: dto.ResponseFormat.Description}
	}
	if dto.Reasoning != nil {
		if err := validateReasoning(*dto.Reasoning); err != nil {
			return provider.CallOptions{}, err
		}
		value := provider.ReasoningEffort(*dto.Reasoning)
		options.Reasoning = &value
	}
	return options, nil
}

func validateToolChoice(choice toolChoiceDTO) error {
	switch provider.ToolChoiceType(choice.Type) {
	case provider.ToolChoiceAuto, provider.ToolChoiceNone, provider.ToolChoiceRequired:
	case provider.ToolChoiceTool:
		if choice.ToolName == "" {
			return errors.New("providerwirev4: tool choice toolName is required")
		}
	default:
		return fmt.Errorf("providerwirev4: unsupported tool choice %q", choice.Type)
	}
	return nil
}

func validateResponseFormat(format responseFormatDTO) error {
	switch provider.ResponseFormatType(format.Type) {
	case provider.ResponseFormatText:
	case provider.ResponseFormatJSON:
	case "":
		return errors.New("providerwirev4: response format type is required")
	default:
		return fmt.Errorf("providerwirev4: unsupported response format %q", format.Type)
	}
	return nil
}

func validateReasoning(value string) error {
	switch provider.ReasoningEffort(value) {
	case provider.ReasoningProviderDefault, provider.ReasoningNone, provider.ReasoningMinimal,
		provider.ReasoningLow, provider.ReasoningMedium, provider.ReasoningHigh, provider.ReasoningXHigh:
		return nil
	default:
		return fmt.Errorf("providerwirev4: unsupported reasoning effort %q", value)
	}
}
