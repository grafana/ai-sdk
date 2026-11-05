package openaicompatible

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/grafana/ai-sdk/provider"
)

func choiceEnvelope(body []byte, choice json.RawMessage) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	delete(fields, "usage")
	fields["choices"] = append(append([]byte("["), choice...), ']')
	return json.Marshal(fields)
}

func parseGenerateChoices(body []byte, headers http.Header, providerName, metadataKey string, generateID func() string, preserveChat bool, count int) (*provider.GenerateResult, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var raw struct {
		Choices []json.RawMessage `json:"choices"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if count > 128 || len(parsed.Choices) != count {
		return nil, fmt.Errorf("openai: response choice count differs from request")
	}
	result := &provider.GenerateResult{Choices: make([]provider.GenerateChoice, 0, count), Usage: convertUsage(parsed.Usage), ProviderMetadata: responseProviderMetadata(metadataKey, parsed.Usage)}
	seen := make([]bool, count)
	for i, choice := range parsed.Choices {
		if choice == nil || choice.Index == nil || *choice.Index < 0 || *choice.Index >= count || seen[*choice.Index] {
			return nil, fmt.Errorf("openai: invalid response choice index")
		}
		seen[*choice.Index] = true
		data, err := choiceEnvelope(body, raw.Choices[i])
		if err != nil {
			return nil, err
		}
		single, err := parseGenerateResponse(data, headers, providerName, metadataKey, generateID, preserveChat)
		if err != nil {
			return nil, err
		}
		result.Choices = append(result.Choices, provider.GenerateChoice{Index: *choice.Index, Content: single.Content, FinishReason: single.FinishReason, ProviderMetadata: single.ProviderMetadata})
	}
	if preserveChat {
		metadata, err := globalChatMetadata(body)
		if err != nil {
			return nil, err
		}
		result.ProviderMetadata = withChatCompletionMetadata(result.ProviderMetadata, metadata)
	}
	result.Response = &provider.GenerateResponse{ResponseMetadata: responseMetadata(parsed.ID, parsed.Model, providerName, parsed.Created), Headers: flattenHeaders(headers), Body: json.RawMessage(append([]byte(nil), body...))}
	return result, nil
}

func (m *model) runChoicesStream(ctx context.Context, endpoint string, requestBody []byte, body io.Reader, headers http.Header, warnings []provider.Warning, includeRaw bool, metadataKey string, out chan<- provider.StreamPart, preserveChat bool, count int) {
	send := func(part provider.StreamPart) bool { return sendStreamPart(ctx, out, part) }
	fail := func(err error) {
		send(provider.StreamPart{Type: provider.PartError, APICallError: streamDecodeError(endpoint, err)})
	}
	if count < 2 || count > 128 {
		fail(errors.New("invalid choice count"))
		return
	}
	states := make([]*streamState, count)
	finished := make([]bool, count)
	var usage *provider.Usage
	var rawUsage *openAIUsage
	var chatGlobal ChatCompletionMetadata
	firstChunk := true
	if !send(provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings}) {
		return
	}
	reader := newOpenAISSEReader(body)
	for {
		data, err := reader.Next()
		if errors.Is(err, io.EOF) || err == nil && string(data) == "[DONE]" {
			for _, done := range finished {
				if !done {
					fail(errors.New("response stream ended before all choices finished"))
					return
				}
			}
			if usage == nil {
				usage = &provider.Usage{}
			}
			metadata := responseProviderMetadata(metadataKey, rawUsage)
			if preserveChat {
				metadata = withChatCompletionMetadata(metadata, chatGlobal)
			}
			send(provider.StreamPart{Type: provider.PartChoicesFinish, Usage: usage, ProviderMetadata: metadata})
			return
		}
		if err != nil {
			fail(err)
			return
		}
		if includeRaw && !send(provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(append([]byte(nil), data...))}) {
			return
		}
		errorData, message, hasError, err := streamError(data)
		if err != nil {
			fail(err)
			return
		}
		if hasError {
			send(provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: message, URL: endpoint, RequestBodyValues: json.RawMessage(requestBody), ResponseHeaders: cloneHeaders(headers), ResponseBody: string(data), Data: errorData})})
			return
		}
		var parsed chatCompletionResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			fail(err)
			return
		}
		if err := validateStreamChunk(parsed, preserveChat); err != nil {
			fail(err)
			return
		}
		if firstChunk && (parsed.ID != "" || parsed.Model != "" || parsed.Created != nil) {
			firstChunk = false
			metadata := responseMetadata(parsed.ID, parsed.Model, m.providerName, parsed.Created)
			if !send(provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: metadata.ID, ModelID: metadata.ModelID, Provider: metadata.Provider, Timestamp: metadata.Timestamp, ResponseHeaders: flattenHeaders(headers)}) {
				return
			}
		}
		if parsed.Usage != nil {
			u := convertUsage(parsed.Usage)
			usage = &u
			rawUsage = parsed.Usage
		}
		var raw struct {
			Choices []json.RawMessage    `json:"choices"`
			Usage   *ChatCompletionUsage `json:"usage"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			fail(err)
			return
		}
		if preserveChat {
			metadata, err := globalChatMetadata(data)
			if err != nil {
				fail(err)
				return
			}
			if metadata.Moderation != nil {
				chatGlobal.Moderation = metadata.Moderation
			}
			if metadata.Usage != nil {
				chatGlobal.Usage = metadata.Usage
			}
			if metadata.ServiceTier != nil {
				chatGlobal.ServiceTier = metadata.ServiceTier
			}
			if metadata.SystemFingerprint != nil {
				chatGlobal.SystemFingerprint = metadata.SystemFingerprint
			}
		}
		seen := make(map[int]bool, len(parsed.Choices))
		for i, choice := range parsed.Choices {
			if choice.Index == nil || *choice.Index < 0 || *choice.Index >= count || seen[*choice.Index] || finished[*choice.Index] {
				fail(errors.New("invalid or repeated stream choice index"))
				return
			}
			index := *choice.Index
			seen[index] = true
			state := states[index]
			if state == nil {
				state = &streamState{ctx: ctx, out: out, choiceIndex: &index, endpoint: endpoint, responseHeaders: headers, providerName: m.providerName, metadataKey: metadataKey, toolCallsByID: map[string]*streamToolCall{}, toolCallsByIndex: map[int]*streamToolCall{}}
				states[index] = state
			}
			if preserveChat {
				single, err := choiceEnvelope(data, raw.Choices[i])
				if err != nil {
					fail(err)
					return
				}
				metadata, err := decodeChatCompletionMetadata(single, true)
				if err != nil {
					fail(err)
					return
				}
				if !metadata.Empty() {
					if !state.send(provider.StreamPart{Type: provider.PartCustom, Kind: "openai-compatible.chat-completion", ProviderMetadata: withChatCompletionMetadata(nil, metadata)}) {
						return
					}
				}
			}
			if !state.handleChoice(*choice) {
				return
			}
			if choice.FinishReason != nil {
				state.flush()
				finished[index] = true
			}
		}
	}
}

// Global metadata deliberately excludes alternative content and probabilities.
func globalChatMetadata(data []byte) (ChatCompletionMetadata, error) {
	var envelope struct {
		Moderation        *ChatModeration      `json:"moderation"`
		Usage             *ChatCompletionUsage `json:"usage"`
		ServiceTier       *string              `json:"service_tier"`
		SystemFingerprint *string              `json:"system_fingerprint"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ChatCompletionMetadata{}, err
	}
	metadata := ChatCompletionMetadata{Moderation: envelope.Moderation, Usage: envelope.Usage, ServiceTier: envelope.ServiceTier, SystemFingerprint: envelope.SystemFingerprint}
	return metadata, metadata.Validate()
}
