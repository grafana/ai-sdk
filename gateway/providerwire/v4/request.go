package providerwirev4

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type wireRequest struct {
	Prompt           []wireMessage              `json:"prompt"`
	MaxOutputTokens  *json.Number               `json:"maxOutputTokens"`
	Temperature      *json.Number               `json:"temperature"`
	StopSequences    []string                   `json:"stopSequences"`
	TopP             *json.Number               `json:"topP"`
	TopK             *json.Number               `json:"topK"`
	PresencePenalty  *json.Number               `json:"presencePenalty"`
	FrequencyPenalty *json.Number               `json:"frequencyPenalty"`
	ResponseFormat   json.RawMessage            `json:"responseFormat"`
	Seed             *json.Number               `json:"seed"`
	Tools            []json.RawMessage          `json:"tools"`
	ToolChoice       json.RawMessage            `json:"toolChoice"`
	IncludeRawChunks *bool                      `json:"includeRawChunks"`
	Headers          map[string]string          `json:"headers"`
	Reasoning        *provider.ReasoningEffort  `json:"reasoning"`
	ProviderOptions  map[string]json.RawMessage `json:"providerOptions"`
}

type wireMessage struct {
	Role            provider.Role              `json:"role"`
	Content         json.RawMessage            `json:"content"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
}

type wirePart struct {
	Type             string                     `json:"type"`
	Text             string                     `json:"text"`
	Data             json.RawMessage            `json:"data"`
	MediaType        string                     `json:"mediaType"`
	Filename         string                     `json:"filename"`
	Kind             string                     `json:"kind"`
	ToolCallID       string                     `json:"toolCallId"`
	ToolName         string                     `json:"toolName"`
	Input            json.RawMessage            `json:"input"`
	Output           json.RawMessage            `json:"output"`
	ProviderExecuted bool                       `json:"providerExecuted"`
	ApprovalID       string                     `json:"approvalId"`
	Approved         *bool                      `json:"approved"`
	Reason           string                     `json:"reason"`
	ProviderOptions  map[string]json.RawMessage `json:"providerOptions"`
}

type wireFileData struct {
	Type      string          `json:"type"`
	Data      string          `json:"data"`
	URL       string          `json:"url"`
	Reference json.RawMessage `json:"reference"`
	Text      string          `json:"text"`
}

type requestAdapter struct {
	maxInlineBytes int64
	inlineBytes    int64
}

func decodeWireRequest(raw []byte) (wireRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var request wireRequest
	if err := decoder.Decode(&request); err != nil {
		return wireRequest{}, fmt.Errorf("decoding request: %w", err)
	}
	return request, nil
}

func (a *requestAdapter) adapt(request wireRequest) (provider.CallOptions, error) {
	if err := applyRequestPolicy(&request); err != nil {
		return provider.CallOptions{}, err
	}
	return a.adaptPolicyChecked(request)
}

func (a *requestAdapter) adaptPolicyChecked(request wireRequest) (provider.CallOptions, error) {
	prompt := make([]provider.Message, len(request.Prompt))
	for i, message := range request.Prompt {
		adapted, err := a.adaptMessage(message)
		if err != nil {
			return provider.CallOptions{}, fmt.Errorf("prompt/%d: %w", i, err)
		}
		prompt[i] = adapted
	}
	tools, err := a.adaptTools(request.Tools)
	if err != nil {
		return provider.CallOptions{}, err
	}
	toolChoice, err := adaptToolChoice(request.ToolChoice)
	if err != nil {
		return provider.CallOptions{}, err
	}
	responseFormat, err := adaptResponseFormat(request.ResponseFormat)
	if err != nil {
		return provider.CallOptions{}, err
	}
	providerOptions, err := adaptProviderOptions(request.ProviderOptions, true)
	if err != nil {
		return provider.CallOptions{}, err
	}
	maxOutputTokens, err := adaptInteger(request.MaxOutputTokens, "maxOutputTokens")
	if err != nil {
		return provider.CallOptions{}, err
	}
	topK, err := adaptInteger(request.TopK, "topK")
	if err != nil {
		return provider.CallOptions{}, err
	}
	seed, err := adaptInteger(request.Seed, "seed")
	if err != nil {
		return provider.CallOptions{}, err
	}
	temperature, err := adaptFloat(request.Temperature, "temperature")
	if err != nil {
		return provider.CallOptions{}, err
	}
	topP, err := adaptFloat(request.TopP, "topP")
	if err != nil {
		return provider.CallOptions{}, err
	}
	presencePenalty, err := adaptFloat(request.PresencePenalty, "presencePenalty")
	if err != nil {
		return provider.CallOptions{}, err
	}
	frequencyPenalty, err := adaptFloat(request.FrequencyPenalty, "frequencyPenalty")
	if err != nil {
		return provider.CallOptions{}, err
	}
	return provider.CallOptions{
		Prompt:           prompt,
		Tools:            tools,
		ToolChoice:       toolChoice,
		MaxOutputTokens:  maxOutputTokens,
		Temperature:      temperature,
		TopP:             topP,
		TopK:             topK,
		PresencePenalty:  presencePenalty,
		FrequencyPenalty: frequencyPenalty,
		StopSequences:    request.StopSequences,
		ResponseFormat:   responseFormat,
		Seed:             seed,
		Reasoning:        request.Reasoning,
		ProviderOptions:  providerOptions,
	}, nil
}

func applyRequestPolicy(request *wireRequest) error {
	if request.IncludeRawChunks != nil && *request.IncludeRawChunks {
		return fmt.Errorf("includeRawChunks is not supported")
	}
	if len(request.Headers) > 0 {
		if len(request.Headers) != 1 || request.Headers["user-agent"] != "ai/7.0.65" {
			return fmt.Errorf("body headers are not supported")
		}
	}
	if err := validateRequestProviderOptions(request); err != nil {
		return err
	}
	request.Headers = nil
	delete(request.ProviderOptions, "gateway")
	return nil
}

func validateRequestProviderOptions(request *wireRequest) error {
	if err := validateProviderOptionMap(request.ProviderOptions, true); err != nil {
		return err
	}
	for i, message := range request.Prompt {
		if err := validateProviderOptionMap(message.ProviderOptions, false); err != nil {
			return fmt.Errorf("prompt/%d: %w", i, err)
		}
		if message.Role == provider.RoleSystem {
			continue
		}
		var parts []json.RawMessage
		if err := json.Unmarshal(message.Content, &parts); err != nil {
			return fmt.Errorf("prompt/%d: decoding content for policy: %w", i, err)
		}
		for j, raw := range parts {
			if err := validatePartProviderOptions(raw); err != nil {
				return fmt.Errorf("prompt/%d/content/%d: %w", i, j, err)
			}
		}
	}
	for i, raw := range request.Tools {
		var tool struct {
			ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			return fmt.Errorf("tools/%d: decoding provider options for policy: %w", i, err)
		}
		if err := validateProviderOptionMap(tool.ProviderOptions, false); err != nil {
			return fmt.Errorf("tools/%d: %w", i, err)
		}
	}
	return nil
}

func validatePartProviderOptions(raw json.RawMessage) error {
	var part wirePart
	if err := json.Unmarshal(raw, &part); err != nil {
		return fmt.Errorf("decoding content part for policy: %w", err)
	}
	if err := validateProviderOptionMap(part.ProviderOptions, false); err != nil {
		return err
	}
	if part.Type != "tool-result" {
		return nil
	}
	var output struct {
		Type            provider.ToolResultOutputType `json:"type"`
		Value           json.RawMessage               `json:"value"`
		ProviderOptions map[string]json.RawMessage    `json:"providerOptions"`
	}
	if err := json.Unmarshal(part.Output, &output); err != nil {
		return fmt.Errorf("decoding tool result output for policy: %w", err)
	}
	if err := validateProviderOptionMap(output.ProviderOptions, false); err != nil {
		return err
	}
	if output.Type != provider.ToolOutputContent {
		return nil
	}
	var content []json.RawMessage
	if err := json.Unmarshal(output.Value, &content); err != nil {
		return fmt.Errorf("decoding tool result content for policy: %w", err)
	}
	for i, nested := range content {
		if err := validatePartProviderOptions(nested); err != nil {
			return fmt.Errorf("value/%d: %w", i, err)
		}
	}
	return nil
}

func validateProviderOptionMap(raw map[string]json.RawMessage, topLevel bool) error {
	for key, value := range raw {
		if key == "gateway" {
			if topLevel {
				var controls map[string]json.RawMessage
				if err := json.Unmarshal(value, &controls); err != nil {
					return fmt.Errorf("decoding gateway controls: %w", err)
				}
				if len(controls) == 0 {
					continue
				}
				return fmt.Errorf("gateway controls are not supported")
			}
			return fmt.Errorf("reserved gateway provider option is not supported")
		}
		if containsReservedGateway(value) {
			return fmt.Errorf("provider option %q contains reserved gateway member", key)
		}
	}
	return nil
}

func (a *requestAdapter) adaptMessage(message wireMessage) (provider.Message, error) {
	providerOptions, err := adaptProviderOptions(message.ProviderOptions, false)
	if err != nil {
		return provider.Message{}, err
	}
	if message.Role == provider.RoleSystem {
		var text string
		if err := json.Unmarshal(message.Content, &text); err != nil {
			return provider.Message{}, fmt.Errorf("decoding system content: %w", err)
		}
		return provider.Message{Role: message.Role, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: text}}, ProviderOptions: providerOptions}, nil
	}
	var rawParts []json.RawMessage
	if err := json.Unmarshal(message.Content, &rawParts); err != nil {
		return provider.Message{}, fmt.Errorf("decoding message content: %w", err)
	}
	parts := make([]provider.ContentPart, len(rawParts))
	for i, raw := range rawParts {
		part, err := a.adaptPart(raw)
		if err != nil {
			return provider.Message{}, fmt.Errorf("content/%d: %w", i, err)
		}
		parts[i] = part
	}
	return provider.Message{Role: message.Role, Content: parts, ProviderOptions: providerOptions}, nil
}

func (a *requestAdapter) adaptPart(raw json.RawMessage) (provider.ContentPart, error) {
	var part wirePart
	if err := json.Unmarshal(raw, &part); err != nil {
		return provider.ContentPart{}, fmt.Errorf("decoding content part: %w", err)
	}
	options, err := adaptProviderOptions(part.ProviderOptions, false)
	if err != nil {
		return provider.ContentPart{}, err
	}
	result := provider.ContentPart{ProviderOptions: options}
	switch part.Type {
	case "text":
		result.Type, result.Text = provider.ContentPartTypeText, part.Text
	case "reasoning":
		result.Type, result.Text = provider.ContentPartTypeReasoning, part.Text
	case "custom":
		result.Type, result.Kind = provider.ContentPartTypeCustom, part.Kind
	case "file", "reasoning-file":
		data, err := a.adaptFileData(part.Data)
		if err != nil {
			return provider.ContentPart{}, err
		}
		result.Type = provider.ContentPartTypeFile
		if part.Type == "reasoning-file" {
			result.Type = provider.ContentPartTypeReasoningFile
		}
		result.Data, result.MediaType, result.Filename = &data, part.MediaType, part.Filename
	case "tool-call":
		result.Type = provider.ContentPartTypeToolCall
		result.ToolCallID, result.ToolName = part.ToolCallID, part.ToolName
		result.Input = cloneRaw(part.Input)
		result.ProviderExecuted = part.ProviderExecuted
	case "tool-result":
		output, err := a.adaptToolResultOutput(part.Output)
		if err != nil {
			return provider.ContentPart{}, err
		}
		result.Type = provider.ContentPartTypeToolResult
		result.ToolCallID, result.ToolName, result.Output = part.ToolCallID, part.ToolName, output
	case "tool-approval-response":
		result.Type = provider.ContentPartTypeToolApprovalResponse
		result.ApprovalID, result.Approved, result.Reason = part.ApprovalID, part.Approved, part.Reason
	default:
		return provider.ContentPart{}, fmt.Errorf("unsupported content part type %q", part.Type)
	}
	return result, nil
}

func (a *requestAdapter) adaptFileData(raw json.RawMessage) (provider.DataContent, error) {
	var data wireFileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return provider.DataContent{}, fmt.Errorf("decoding file data: %w", err)
	}
	if data.Type == "data" {
		remaining := a.maxInlineBytes - a.inlineBytes
		decoded, err := decodeInlineFileData(data.Data, remaining)
		if err != nil {
			return provider.DataContent{}, err
		}
		a.inlineBytes += int64(len(decoded))
		return provider.DataContent{Bytes: decoded}, nil
	}
	var result provider.DataContent
	if err := json.Unmarshal(raw, &result); err != nil {
		return provider.DataContent{}, fmt.Errorf("adapting tagged file data: %w", err)
	}
	return result, nil
}

func decodeInlineFileData(encoded string, remaining int64) ([]byte, error) {
	if remaining < 0 {
		return nil, fmt.Errorf("decoded inline file data exceeds configured limit")
	}
	probeLimit := remaining
	if probeLimit < math.MaxInt64 {
		probeLimit++
	}
	decodedSize, err := io.CopyN(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded)), probeLimit)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("decoding inline file data: %w", err)
	}
	if decodedSize > remaining {
		return nil, fmt.Errorf("decoded inline file data exceeds configured limit")
	}
	decoded := make([]byte, int(decodedSize))
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	if _, err := io.ReadFull(decoder, decoded); err != nil && err != io.EOF {
		return nil, fmt.Errorf("decoding inline file data: %w", err)
	}
	var trailing [1]byte
	if count, err := decoder.Read(trailing[:]); count != 0 || err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decoded inline file data exceeds configured limit")
		}
		return nil, fmt.Errorf("decoding inline file data: %w", err)
	}
	return decoded, nil
}

func (a *requestAdapter) adaptToolResultOutput(raw json.RawMessage) (*provider.ToolResultOutput, error) {
	var output struct {
		Type            provider.ToolResultOutputType `json:"type"`
		Value           json.RawMessage               `json:"value"`
		Reason          string                        `json:"reason"`
		ProviderOptions map[string]json.RawMessage    `json:"providerOptions"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("decoding tool result output: %w", err)
	}
	options, err := adaptProviderOptions(output.ProviderOptions, false)
	if err != nil {
		return nil, err
	}
	result := &provider.ToolResultOutput{Type: output.Type, Reason: output.Reason, ProviderOptions: options}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		if err := json.Unmarshal(output.Value, &result.Text); err != nil {
			return nil, fmt.Errorf("decoding tool result text: %w", err)
		}
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		result.JSON = cloneRaw(output.Value)
	case provider.ToolOutputExecutionDenied:
	case provider.ToolOutputContent:
		var rawContent []json.RawMessage
		if err := json.Unmarshal(output.Value, &rawContent); err != nil {
			return nil, fmt.Errorf("decoding tool result content: %w", err)
		}
		result.Content = make([]provider.ToolResultContentValue, len(rawContent))
		for i, rawPart := range rawContent {
			part, err := a.adaptToolResultContent(rawPart)
			if err != nil {
				return nil, fmt.Errorf("tool result content/%d: %w", i, err)
			}
			result.Content[i] = part
		}
	default:
		return nil, fmt.Errorf("unsupported tool result output type %q", output.Type)
	}
	return result, nil
}

func (a *requestAdapter) adaptToolResultContent(raw json.RawMessage) (provider.ToolResultContentValue, error) {
	var part wirePart
	if err := json.Unmarshal(raw, &part); err != nil {
		return provider.ToolResultContentValue{}, fmt.Errorf("decoding tool result content: %w", err)
	}
	options, err := adaptProviderOptions(part.ProviderOptions, false)
	if err != nil {
		return provider.ToolResultContentValue{}, err
	}
	result := provider.ToolResultContentValue{ProviderOptions: options, Text: part.Text, MediaType: part.MediaType, Filename: part.Filename}
	switch part.Type {
	case "text":
		result.Type = provider.ToolContentText
	case "custom":
		result.Type = provider.ToolContentCustom
	case "file":
		data, err := a.adaptFileData(part.Data)
		if err != nil {
			return provider.ToolResultContentValue{}, err
		}
		result.Type, result.Data = provider.ToolContentFile, &data
	default:
		return provider.ToolResultContentValue{}, fmt.Errorf("unsupported tool result content type %q", part.Type)
	}
	return result, nil
}

func (a *requestAdapter) adaptTools(rawTools []json.RawMessage) ([]provider.Tool, error) {
	if rawTools == nil {
		return nil, nil
	}
	tools := make([]provider.Tool, len(rawTools))
	for i, raw := range rawTools {
		var tool struct {
			Type            provider.ToolType          `json:"type"`
			Name            string                     `json:"name"`
			Description     string                     `json:"description"`
			InputSchema     json.RawMessage            `json:"inputSchema"`
			InputExamples   []provider.InputExample    `json:"inputExamples"`
			Strict          *bool                      `json:"strict"`
			ID              string                     `json:"id"`
			Args            map[string]json.RawMessage `json:"args"`
			ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			return nil, fmt.Errorf("tools/%d: %w", i, err)
		}
		options, err := adaptProviderOptions(tool.ProviderOptions, false)
		if err != nil {
			return nil, fmt.Errorf("tools/%d: %w", i, err)
		}
		tools[i] = provider.Tool{Type: tool.Type, Name: tool.Name, Description: tool.Description, InputSchema: cloneRaw(tool.InputSchema), InputExamples: tool.InputExamples, Strict: tool.Strict, ID: tool.ID, Args: cloneRawMap(tool.Args), ProviderOptions: options}
	}
	return tools, nil
}

func adaptToolChoice(raw json.RawMessage) (*provider.ToolChoice, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var choice provider.ToolChoice
	if err := json.Unmarshal(raw, &choice); err != nil {
		return nil, fmt.Errorf("decoding tool choice: %w", err)
	}
	return &choice, nil
}

func adaptResponseFormat(raw json.RawMessage) (*provider.ResponseFormat, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var format struct {
		Type        provider.ResponseFormatType `json:"type"`
		Schema      json.RawMessage             `json:"schema"`
		Name        string                      `json:"name"`
		Description string                      `json:"description"`
	}
	if err := json.Unmarshal(raw, &format); err != nil {
		return nil, fmt.Errorf("decoding response format: %w", err)
	}
	return &provider.ResponseFormat{Type: format.Type, Schema: cloneRaw(format.Schema), Name: format.Name, Description: format.Description}, nil
}

func adaptProviderOptions(raw map[string]json.RawMessage, topLevel bool) (provider.ProviderOptions, error) {
	if raw == nil {
		return nil, nil
	}
	result := make(provider.ProviderOptions, len(raw))
	for key, value := range raw {
		if key == "gateway" {
			if topLevel {
				var object map[string]json.RawMessage
				if err := json.Unmarshal(value, &object); err == nil && len(object) == 0 {
					continue
				}
			}
			return nil, fmt.Errorf("reserved gateway provider option is not supported")
		}
		if containsReservedGateway(value) {
			return nil, fmt.Errorf("provider option %q contains reserved gateway member", key)
		}
		result[key] = provider.RawProviderOption{Key: key, Raw: cloneRaw(value)}
	}
	return result, nil
}

func containsReservedGateway(raw json.RawMessage) bool {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return false
	}
	var visit func(any) bool
	visit = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			if _, ok := typed["gateway"]; ok {
				return true
			}
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

const (
	maxNumericLexemeBytes = 128
	maxNumericExponent    = 1024
)

func validateNumericWork(number string) error {
	if len(number) > maxNumericLexemeBytes {
		return fmt.Errorf("numeric value is too long")
	}
	index := strings.IndexAny(number, "eE")
	if index < 0 {
		return nil
	}
	exponent, err := strconv.ParseInt(number[index+1:], 10, 32)
	if err != nil || exponent < -maxNumericExponent || exponent > maxNumericExponent {
		return fmt.Errorf("numeric exponent is out of range")
	}
	return nil
}

func adaptInteger(number *json.Number, name string) (*int, error) {
	if number == nil {
		return nil, nil
	}
	if err := validateNumericWork(number.String()); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	rational, ok := new(big.Rat).SetString(number.String())
	if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return nil, fmt.Errorf("%s must be an integer within Go int range", name)
	}
	value := rational.Num().Int64()
	if strconv.IntSize == 32 && (value < math.MinInt32 || value > math.MaxInt32) {
		return nil, fmt.Errorf("%s must be an integer within Go int range", name)
	}
	result := int(value)
	return &result, nil
}

func adaptFloat(number *json.Number, name string) (*float64, error) {
	if number == nil {
		return nil, nil
	}
	raw := number.String()
	if err := validateNumericWork(raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fmt.Errorf("%s must be representable as a finite float64", name)
	}
	original, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("%s must be a valid JSON number", name)
	}
	canonical, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'g', -1, 64))
	if !ok || original.Cmp(canonical) != 0 {
		return nil, fmt.Errorf("%s must survive canonical float64 decimal round trip", name)
	}
	result := value
	return &result, nil
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func cloneRawMap(raw map[string]json.RawMessage) map[string]json.RawMessage {
	if raw == nil {
		return nil
	}
	result := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		result[key] = cloneRaw(value)
	}
	return result
}
