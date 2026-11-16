package openaicompatible

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

// ChatCompletionMetadata retains Chat fields without a LanguageModelV4 equivalent.
// Unary results and stream finish metadata carry complete values.
// Native token accounting remains in Usage.Raw.
type ChatCompletionMetadata struct {
	Refusal  *string                 `json:"refusal,omitempty"`
	Logprobs *ChatCompletionLogprobs `json:"logprobs,omitempty"`
}

// ChatCompletionLogprobs preserves both native probability channels.
type ChatCompletionLogprobs struct {
	Content []ChatCompletionTokenLogprob `json:"content"`
	Refusal []ChatCompletionTokenLogprob `json:"refusal"`
}

// ChatCompletionTokenProbability is a reported token and its log probability.
// Pointers distinguish missing required fields from valid empty tokens and zero.
type ChatCompletionTokenProbability struct {
	Token   *string  `json:"token"`
	Logprob *float64 `json:"logprob"`
	Bytes   []int    `json:"bytes"`
}

// ChatCompletionTokenLogprob adds the native alternatives for one output token.
type ChatCompletionTokenLogprob struct {
	ChatCompletionTokenProbability
	TopLogprobs []ChatCompletionTokenProbability `json:"top_logprobs"`
}

// ChatCompletionMetadataKey identifies Chat-only fields inside the resolved provider namespace.
const ChatCompletionMetadataKey = "chatCompletion"

// ErrInvalidChatCompletionMetadata identifies malformed Chat-only output.
var ErrInvalidChatCompletionMetadata = errors.New("openai-compatible: invalid chat completion metadata")

func decodeChatCompletionMetadata(data []byte, streaming bool) (ChatCompletionMetadata, error) {
	var envelope struct {
		Choices []json.RawMessage `json:"choices"`
	}
	var result ChatCompletionMetadata
	if err := json.Unmarshal(data, &envelope); err != nil {
		return result, fmt.Errorf("%w: decoding choices: %w", ErrInvalidChatCompletionMetadata, err)
	}
	if len(envelope.Choices) == 0 {
		return result, nil
	}
	var choice struct {
		Message  json.RawMessage         `json:"message"`
		Delta    json.RawMessage         `json:"delta"`
		Logprobs *ChatCompletionLogprobs `json:"logprobs"`
	}
	if err := json.Unmarshal(envelope.Choices[0], &choice); err != nil {
		return result, fmt.Errorf("%w: decoding first choice: %w", ErrInvalidChatCompletionMetadata, err)
	}
	message := choice.Message
	if streaming {
		message = choice.Delta
	}
	if len(message) > 0 {
		var fields struct {
			Refusal *string `json:"refusal"`
		}
		if err := json.Unmarshal(message, &fields); err != nil {
			return result, fmt.Errorf("%w: decoding refusal: %w", ErrInvalidChatCompletionMetadata, err)
		}
		result.Refusal = fields.Refusal
	}
	if choice.Logprobs != nil && (len(choice.Logprobs.Content) > 0 || len(choice.Logprobs.Refusal) > 0) {
		result.Logprobs = choice.Logprobs
	}
	if err := result.Validate(); err != nil {
		return ChatCompletionMetadata{}, err
	}
	return result, nil
}

// Validate checks the typed metadata before a codec publishes it.
func (m ChatCompletionMetadata) Validate() error {
	invalid := ErrInvalidChatCompletionMetadata
	if m.Refusal != nil && !utf8.ValidString(*m.Refusal) {
		return invalid
	}
	if m.Logprobs != nil {
		for _, channel := range [][]ChatCompletionTokenLogprob{m.Logprobs.Content, m.Logprobs.Refusal} {
			for _, entry := range channel {
				if !validTokenProbability(entry.ChatCompletionTokenProbability) {
					return invalid
				}
				for _, alternative := range entry.TopLogprobs {
					if !validTokenProbability(alternative) {
						return invalid
					}
				}
			}
		}
	}
	return nil
}

func validTokenProbability(p ChatCompletionTokenProbability) bool {
	if p.Token == nil || p.Logprob == nil || !utf8.ValidString(*p.Token) || math.IsNaN(*p.Logprob) || math.IsInf(*p.Logprob, 0) || *p.Logprob > 0 {
		return false
	}
	for _, b := range p.Bytes {
		if b < 0 || b > 255 {
			return false
		}
	}
	return true
}

func withChatCompletionMetadata(metadata provider.ProviderMetadata, metadataKey string, chat ChatCompletionMetadata) (provider.ProviderMetadata, error) {
	if chat.Refusal == nil && chat.Logprobs == nil {
		return metadata, nil
	}
	if err := chat.Validate(); err != nil {
		return metadata, fmt.Errorf("validating chat metadata: %w", err)
	}
	var namespace map[string]json.RawMessage
	if raw := metadata[metadataKey]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &namespace); err != nil {
			return metadata, fmt.Errorf("decoding provider metadata: %w", err)
		}
	}
	if namespace == nil {
		namespace = map[string]json.RawMessage{}
	}
	rawChat, err := json.Marshal(chat)
	if err != nil {
		return metadata, fmt.Errorf("encoding chat metadata: %w", err)
	}
	namespace[ChatCompletionMetadataKey] = rawChat
	rawNamespace, err := json.Marshal(namespace)
	if err != nil {
		return metadata, fmt.Errorf("encoding provider metadata: %w", err)
	}
	if metadata == nil {
		metadata = provider.ProviderMetadata{}
	}
	metadata[metadataKey] = rawNamespace
	return metadata, nil
}

func (m *ChatCompletionMetadata) append(delta ChatCompletionMetadata) {
	if delta.Refusal != nil {
		refusal := ""
		if m.Refusal != nil {
			refusal = *m.Refusal
		}
		refusal += *delta.Refusal
		m.Refusal = &refusal
	}
	if delta.Logprobs != nil {
		if m.Logprobs == nil {
			m.Logprobs = &ChatCompletionLogprobs{}
		}
		m.Logprobs.Content = append(m.Logprobs.Content, delta.Logprobs.Content...)
		m.Logprobs.Refusal = append(m.Logprobs.Refusal, delta.Logprobs.Refusal...)
	}
}

func chatMetadataError(endpoint string, err error) *provider.APICallError {
	retryable := false
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message: fmt.Sprintf("openai-compatible: decoding chat metadata: %v", err),
		URL:     endpoint, IsRetryable: &retryable, Cause: err,
	})
}
