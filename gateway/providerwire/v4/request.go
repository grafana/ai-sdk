package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
)

type requestEnvelope struct {
	modelID   string
	streaming bool
}

func validateEnvelope(request *http.Request) (requestEnvelope, failure.Failure, bool) {
	if request.Method != http.MethodPost {
		return requestEnvelope{}, invalidRequest("method must be POST"), false
	}
	modelID, ok := exactHeader(request.Header, HeaderModelID)
	if !ok || modelID == "" {
		return requestEnvelope{}, invalidRequest("invalid model header"), false
	}
	version, ok := exactHeader(request.Header, HeaderSpecVersion)
	if !ok || version != SpecVersionV4 {
		return requestEnvelope{}, invalidRequest("invalid specification version header"), false
	}
	streamingValue, ok := exactHeader(request.Header, HeaderStreaming)
	if !ok || (streamingValue != "true" && streamingValue != "false") {
		return requestEnvelope{}, invalidRequest("invalid streaming header"), false
	}
	contentType, ok := exactHeader(request.Header, "Content-Type")
	if !ok || contentType != MIMEJSON {
		return requestEnvelope{}, invalidRequest("invalid content type"), false
	}
	return requestEnvelope{modelID: modelID, streaming: streamingValue == "true"}, failure.Failure{}, true
}

func exactHeader(header http.Header, name string) (string, bool) {
	var values []string
	for key, keyValues := range header {
		if strings.EqualFold(key, name) {
			values = append(values, keyValues...)
		}
	}
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func (h *Handler) readAndMapRequest(ctx context.Context, body io.ReadCloser) (provider.CallOptions, failure.Failure, bool) {
	defer func() { _ = body.Close() }()
	readLimit := h.maxRequestBodyBytes
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(body, readLimit))
	if value, ok := contextFailure(ctx); ok {
		return provider.CallOptions{}, value, false
	}
	if err != nil {
		return provider.CallOptions{}, invalidRequest("unable to read request body"), false
	}
	if int64(len(data)) > h.maxRequestBodyBytes {
		return provider.CallOptions{}, invalidRequest("request body is too large"), false
	}
	if err := validateStrictJSON(data); err != nil {
		return provider.CallOptions{}, invalidRequest("request body is not strict JSON"), false
	}
	if err := h.schemas.request.Validate(json.RawMessage(data)); err != nil {
		return provider.CallOptions{}, invalidRequest("request body does not match ProviderWire V4"), false
	}
	options, err := mapRequest(data)
	if err != nil {
		return provider.CallOptions{}, invalidRequest(err.Error()), false
	}
	return options, failure.Failure{}, true
}

func validateStrictJSON(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("providerwire v4: request body is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("providerwire v4: request body contains trailing data")
		}
		return fmt.Errorf("providerwire v4: request body contains trailing data: %w", err)
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("providerwire v4: decoding JSON token: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		names := map[string]struct{}{}
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("providerwire v4: decoding object member: %w", err)
			}
			name, ok := nameToken.(string)
			if !ok {
				return errors.New("providerwire v4: object member name is not a string")
			}
			if _, exists := names[name]; exists {
				return errors.New("providerwire v4: duplicate object member")
			}
			names[name] = struct{}{}
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("providerwire v4: invalid object ending")
		}
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("providerwire v4: invalid array ending")
		}
	default:
		return errors.New("providerwire v4: unexpected JSON delimiter")
	}
	return nil
}

type requestDTO struct {
	Prompt           json.RawMessage `json:"prompt"`
	MaxOutputTokens  json.RawMessage `json:"maxOutputTokens"`
	Temperature      json.RawMessage `json:"temperature"`
	StopSequences    json.RawMessage `json:"stopSequences"`
	TopP             json.RawMessage `json:"topP"`
	TopK             json.RawMessage `json:"topK"`
	PresencePenalty  json.RawMessage `json:"presencePenalty"`
	FrequencyPenalty json.RawMessage `json:"frequencyPenalty"`
	ResponseFormat   json.RawMessage `json:"responseFormat"`
	Seed             json.RawMessage `json:"seed"`
	Tools            json.RawMessage `json:"tools"`
	ToolChoice       json.RawMessage `json:"toolChoice"`
	IncludeRawChunks json.RawMessage `json:"includeRawChunks"`
	Headers          json.RawMessage `json:"headers"`
	Reasoning        json.RawMessage `json:"reasoning"`
	ProviderOptions  json.RawMessage `json:"providerOptions"`
}

func mapRequest(data []byte) (provider.CallOptions, error) {
	var request requestDTO
	if err := json.Unmarshal(data, &request); err != nil {
		return provider.CallOptions{}, errors.New("request body cannot be decoded")
	}
	for _, unsupported := range []json.RawMessage{
		request.ResponseFormat,
		request.Tools,
		request.ToolChoice,
		request.IncludeRawChunks,
		request.Headers,
		request.ProviderOptions,
	} {
		if len(unsupported) != 0 {
			return provider.CallOptions{}, errors.New("request contains an unsupported Phase 3 field")
		}
	}
	var rawMessages []map[string]json.RawMessage
	if err := json.Unmarshal(request.Prompt, &rawMessages); err != nil {
		return provider.CallOptions{}, errors.New("prompt cannot be decoded")
	}
	options := provider.CallOptions{Prompt: make([]provider.Message, 0, len(rawMessages))}
	for _, rawMessage := range rawMessages {
		if _, present := rawMessage["providerOptions"]; present {
			return provider.CallOptions{}, errors.New("message provider options are unsupported in Phase 3")
		}
		var role provider.Role
		if err := json.Unmarshal(rawMessage["role"], &role); err != nil {
			return provider.CallOptions{}, errors.New("message role cannot be decoded")
		}
		switch role {
		case provider.RoleSystem:
			var text string
			if err := json.Unmarshal(rawMessage["content"], &text); err != nil {
				return provider.CallOptions{}, errors.New("system content cannot be decoded")
			}
			options.Prompt = append(options.Prompt, provider.NewSystemMessage(text))
		case provider.RoleUser, provider.RoleAssistant:
			parts, err := mapTextParts(rawMessage["content"])
			if err != nil {
				return provider.CallOptions{}, err
			}
			options.Prompt = append(options.Prompt, provider.Message{Role: role, Content: parts})
		default:
			return provider.CallOptions{}, errors.New("tool messages are unsupported in Phase 3")
		}
	}
	if err := mapSettings(request, &options); err != nil {
		return provider.CallOptions{}, err
	}
	return options, nil
}

func mapTextParts(raw json.RawMessage) ([]provider.ContentPart, error) {
	var rawParts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawParts); err != nil {
		return nil, errors.New("message content cannot be decoded")
	}
	parts := make([]provider.ContentPart, 0, len(rawParts))
	for _, rawPart := range rawParts {
		if _, present := rawPart["providerOptions"]; present {
			return nil, errors.New("part provider options are unsupported in Phase 3")
		}
		var partType provider.ContentPartType
		if err := json.Unmarshal(rawPart["type"], &partType); err != nil || partType != provider.ContentPartTypeText {
			return nil, errors.New("non-text content is unsupported in Phase 3")
		}
		var text string
		if err := json.Unmarshal(rawPart["text"], &text); err != nil {
			return nil, errors.New("text content cannot be decoded")
		}
		parts = append(parts, provider.TextPart(text))
	}
	return parts, nil
}

func mapSettings(request requestDTO, options *provider.CallOptions) error {
	if err := mapLanguageModelNumber("maxOutputTokens", request.MaxOutputTokens, &options.MaxOutputTokens); err != nil {
		return err
	}
	if err := mapLanguageModelNumber("topK", request.TopK, &options.TopK); err != nil {
		return err
	}
	if err := mapLanguageModelNumber("seed", request.Seed, &options.Seed); err != nil {
		return err
	}
	if err := mapFloat("temperature", request.Temperature, &options.Temperature); err != nil {
		return err
	}
	if err := mapFloat("topP", request.TopP, &options.TopP); err != nil {
		return err
	}
	if err := mapFloat("presencePenalty", request.PresencePenalty, &options.PresencePenalty); err != nil {
		return err
	}
	if err := mapFloat("frequencyPenalty", request.FrequencyPenalty, &options.FrequencyPenalty); err != nil {
		return err
	}
	if len(request.StopSequences) != 0 {
		if err := json.Unmarshal(request.StopSequences, &options.StopSequences); err != nil {
			return errors.New("stop sequences cannot be decoded")
		}
		if options.StopSequences == nil {
			options.StopSequences = []string{}
		}
	}
	if len(request.Reasoning) != 0 {
		var value provider.ReasoningEffort
		if err := json.Unmarshal(request.Reasoning, &value); err != nil {
			return errors.New("reasoning cannot be decoded")
		}
		options.Reasoning = &value
	}
	return nil
}

func mapLanguageModelNumber(name string, raw json.RawMessage, target **provider.LanguageModelNumber) error {
	if len(raw) == 0 {
		return nil
	}
	token := string(raw)
	rational, ok := new(big.Rat).SetString(token)
	if !ok {
		return fmt.Errorf("setting %q is not representable", name)
	}
	if rational.IsInt() {
		if !rational.Num().IsInt64() {
			return fmt.Errorf("setting %q is not representable", name)
		}
		value := provider.LanguageModelNumberFromInt64(rational.Num().Int64())
		*target = &value
		return nil
	}
	floating, err := strconv.ParseFloat(token, 64)
	if err != nil || math.IsNaN(floating) || math.IsInf(floating, 0) || floating == math.Trunc(floating) {
		return fmt.Errorf("setting %q is not representable", name)
	}
	value, err := provider.LanguageModelNumberFromFloat64(floating)
	if err != nil {
		return fmt.Errorf("setting %q is not representable", name)
	}
	*target = &value
	return nil
}

func mapFloat(name string, raw json.RawMessage, target **float64) error {
	if len(raw) == 0 {
		return nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("setting %q is not representable", name)
	}
	*target = &value
	return nil
}
