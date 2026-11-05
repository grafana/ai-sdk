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
	Moderation        *ChatModeration            `json:"moderation,omitempty"`
	ServiceTier       *string                    `json:"service_tier,omitempty"`
	SystemFingerprint *string                    `json:"system_fingerprint,omitempty"`
	Annotations       []ChatCompletionAnnotation `json:"annotations,omitempty"`
	Audio             *ChatCompletionAudio       `json:"audio,omitempty"`
	Refusal           *string                    `json:"refusal,omitempty"`
	Logprobs          *ChatCompletionLogprobs    `json:"logprobs,omitempty"`
	Usage             *ChatCompletionUsage       `json:"usage,omitempty"`
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
	CacheWriteTokens *int `json:"cache_write_tokens,omitempty"`
	ImageTokens      *int `json:"image_tokens,omitempty"`
	TextTokens       *int `json:"text_tokens,omitempty"`
	CachedTokens     *int `json:"cached_tokens,omitempty"`
	AudioTokens      *int `json:"audio_tokens,omitempty"`
}

// ChatCompletionOutputTokensDetails describes native output accounting.
type ChatCompletionOutputTokensDetails struct {
	TextTokens               *int `json:"text_tokens,omitempty"`
	ReasoningTokens          *int `json:"reasoning_tokens,omitempty"`
	AudioTokens              *int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens *int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens *int `json:"rejected_prediction_tokens,omitempty"`
}

func decodeChatCompletionMetadata(data []byte, streaming bool) (ChatCompletionMetadata, error) {
	var envelope struct {
		Moderation        *ChatModeration `json:"moderation"`
		ServiceTier       *string         `json:"service_tier"`
		SystemFingerprint *string         `json:"system_fingerprint"`
		Choices           []struct {
			Message struct {
				Annotations []ChatCompletionAnnotation `json:"annotations"`
				Refusal     *string                    `json:"refusal"`
				Audio       *ChatCompletionAudio       `json:"audio"`
			} `json:"message"`
			Delta struct {
				Annotations []ChatCompletionAnnotation `json:"annotations"`
				Refusal     *string                    `json:"refusal"`
				Audio       *ChatCompletionAudio       `json:"audio"`
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
	result.Moderation = envelope.Moderation
	result.Usage = envelope.Usage
	result.ServiceTier = envelope.ServiceTier
	result.SystemFingerprint = envelope.SystemFingerprint
	if len(envelope.Choices) > 0 {
		choice := envelope.Choices[0]
		result.Annotations = choice.Message.Annotations
		result.Audio = choice.Message.Audio
		result.Refusal = choice.Message.Refusal
		if streaming {
			result.Annotations = choice.Delta.Annotations
			result.Audio = choice.Delta.Audio
			result.Refusal = choice.Delta.Refusal
		}
		result.Logprobs = choice.Logprobs
	}
	if result.Audio != nil && !streaming {
		if err := result.Audio.ValidateComplete(); err != nil {
			return ChatCompletionMetadata{}, err
		}
	}
	if err := result.Validate(); err != nil {
		return ChatCompletionMetadata{}, err
	}
	return result, nil
}

// Validate checks the typed metadata before a codec publishes it.
func (m ChatCompletionMetadata) Validate() error {
	if err := m.Moderation.Validate(); err != nil {
		return err
	}
	invalid := errors.New("openai: invalid Chat completion metadata")
	for _, v := range []*string{m.ServiceTier, m.SystemFingerprint} {
		if v != nil && !utf8.ValidString(*v) {
			return invalid
		}
	}
	for _, a := range m.Annotations {
		if a.Validate() != nil {
			return invalid
		}
	}
	if m.Audio != nil {
		if err := m.Audio.Validate(); err != nil {
			return err
		}
	}
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
			counts = append(counts, d.CachedTokens, d.AudioTokens, d.CacheWriteTokens, d.ImageTokens, d.TextTokens)
		}
		if d := u.CompletionTokensDetails; d != nil {
			counts = append(counts, d.ReasoningTokens, d.AudioTokens, d.AcceptedPredictionTokens, d.RejectedPredictionTokens, d.TextTokens)
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
	if chat.Empty() {
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

// Empty reports whether there are any native fields to publish.
func (m ChatCompletionMetadata) Empty() bool {
	return m.Moderation == nil && m.Audio == nil && m.Refusal == nil && m.Logprobs == nil && m.Usage == nil && m.ServiceTier == nil && m.SystemFingerprint == nil && len(m.Annotations) == 0
}

// ChatCompletionAnnotation preserves the native URL citation shape.
type ChatCompletionAnnotation struct {
	Type        string                    `json:"type"`
	URLCitation ChatCompletionURLCitation `json:"url_citation"`
}

// ChatCompletionURLCitation describes a native text span and URL.
type ChatCompletionURLCitation struct {
	StartIndex *int   `json:"start_index"`
	EndIndex   *int   `json:"end_index"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}

// Validate checks the annotation before it crosses a typed output boundary.
func (a ChatCompletionAnnotation) Validate() error {
	c := a.URLCitation
	if a.Type != "url_citation" || c.StartIndex == nil || c.EndIndex == nil || *c.StartIndex < 0 || *c.EndIndex < *c.StartIndex || c.URL == "" || !utf8.ValidString(c.URL) || !utf8.ValidString(c.Title) {
		return errors.New("openai: invalid Chat annotation")
	}
	return nil
}
