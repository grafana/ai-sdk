package openaicompatible

import (
	"encoding/json"
	"errors"
	"math"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

// ChatCompletionMetadata retains Chat fields without a LanguageModelV4 equivalent.
// Unary results carry complete values. Custom streaming parts carry deltas; the
// finish part carries final usage. It is opt-in and never contains raw response data.
type ChatCompletionMetadata struct {
	Refusal  *string                 `json:"refusal,omitempty"`
	Logprobs *ChatCompletionLogprobs `json:"logprobs,omitempty"`
	Usage    *ChatCompletionUsage    `json:"usage,omitempty"`
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

// ChatCompletionUsage preserves reported counts, including absent versus zero.
type ChatCompletionUsage struct {
	PromptTokens            *int                               `json:"prompt_tokens,omitempty"`
	CompletionTokens        *int                               `json:"completion_tokens,omitempty"`
	TotalTokens             *int                               `json:"total_tokens,omitempty"`
	PromptTokensDetails     *ChatCompletionPromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *ChatCompletionOutputTokensDetails `json:"completion_tokens_details,omitempty"`
}

// ChatCompletionPromptTokensDetails describes native input accounting.
type ChatCompletionPromptTokensDetails struct {
	CachedTokens *int `json:"cached_tokens,omitempty"`
	AudioTokens  *int `json:"audio_tokens,omitempty"`
}

// ChatCompletionOutputTokensDetails describes native output accounting.
type ChatCompletionOutputTokensDetails struct {
	ReasoningTokens          *int `json:"reasoning_tokens,omitempty"`
	AudioTokens              *int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens *int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens *int `json:"rejected_prediction_tokens,omitempty"`
}

func decodeChatCompletionMetadata(data []byte, streaming bool) (ChatCompletionMetadata, error) {
	var envelope struct {
		Choices []struct {
			Message struct {
				Refusal *string `json:"refusal"`
			} `json:"message"`
			Delta struct {
				Refusal *string `json:"refusal"`
			} `json:"delta"`
			Logprobs *ChatCompletionLogprobs `json:"logprobs"`
		} `json:"choices"`
		Usage *ChatCompletionUsage `json:"usage"`
	}
	var result ChatCompletionMetadata
	if !utf8.Valid(data) {
		return result, errors.New("openai: invalid Chat metadata encoding")
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return result, err
	}
	result.Usage = envelope.Usage
	if len(envelope.Choices) > 0 {
		choice := envelope.Choices[0]
		result.Refusal = choice.Message.Refusal
		if streaming {
			result.Refusal = choice.Delta.Refusal
		}
		result.Logprobs = choice.Logprobs
	}
	if err := result.Validate(); err != nil {
		return ChatCompletionMetadata{}, err
	}
	return result, nil
}

// Validate checks the typed metadata before a codec publishes it.
func (m ChatCompletionMetadata) Validate() error {
	invalid := errors.New("openai: invalid Chat completion metadata")
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
	if u := m.Usage; u != nil {
		counts := []*int{u.PromptTokens, u.CompletionTokens, u.TotalTokens}
		if d := u.PromptTokensDetails; d != nil {
			counts = append(counts, d.CachedTokens, d.AudioTokens)
		}
		if d := u.CompletionTokensDetails; d != nil {
			counts = append(counts, d.ReasoningTokens, d.AudioTokens, d.AcceptedPredictionTokens, d.RejectedPredictionTokens)
		}
		for _, count := range counts {
			if count != nil && *count < 0 {
				return invalid
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

func withChatCompletionMetadata(metadata provider.ProviderMetadata, chat ChatCompletionMetadata) provider.ProviderMetadata {
	if chat.Refusal == nil && chat.Logprobs == nil && chat.Usage == nil {
		return metadata
	}
	if metadata == nil {
		metadata = provider.ProviderMetadata{}
	}
	var namespace map[string]json.RawMessage
	_ = json.Unmarshal(metadata["openaiCompatible"], &namespace)
	if namespace == nil {
		namespace = map[string]json.RawMessage{}
	}
	namespace["chatCompletion"], _ = json.Marshal(chat)
	metadata["openaiCompatible"], _ = json.Marshal(namespace)
	return metadata
}
