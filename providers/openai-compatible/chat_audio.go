package openaicompatible

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"unicode/utf8"
)

// ChatCompletionAudio represents complete unary audio or one streaming delta.
// Delta fields are optional and must not be filled with invented values.
type ChatCompletionAudio struct {
	ID         *string `json:"id,omitempty"`
	ExpiresAt  *int64  `json:"expires_at,omitempty"`
	Data       *string `json:"data,omitempty"`
	Transcript *string `json:"transcript,omitempty"`
}

// Validate checks a single native audio value or delta.
func (a ChatCompletionAudio) Validate() error {
	invalid := errors.New("openai: invalid Chat audio output")
	if a.ID != nil && (*a.ID == "" || !utf8.ValidString(*a.ID)) {
		return invalid
	}
	if a.ExpiresAt != nil && *a.ExpiresAt < 0 {
		return invalid
	}
	if a.Transcript != nil && !utf8.ValidString(*a.Transcript) {
		return invalid
	}
	if a.Data != nil {
		// Native audio deltas contain independently encoded byte chunks.
		if _, err := base64.StdEncoding.Strict().DecodeString(*a.Data); err != nil {
			return invalid
		}
	}
	return nil
}

// ValidateComplete additionally requires every field in a unary audio response.
func (a ChatCompletionAudio) ValidateComplete() error {
	if a.ID == nil || a.ExpiresAt == nil || a.Data == nil || a.Transcript == nil {
		return errors.New("openai: incomplete Chat audio output")
	}
	return a.Validate()
}

// ChatCompletionAudioOptions requests a native output format and voice.
type ChatCompletionAudioOptions struct {
	Format string              `json:"format"`
	Voice  ChatCompletionVoice `json:"voice"`
}

// ChatCompletionVoice is either a provider voice name or a custom voice ID.
// Exactly one field must be set. Custom voice availability is provider-owned.
type ChatCompletionVoice struct {
	Name string
	ID   string
}

func (v ChatCompletionVoice) validate() error {
	if (v.Name == "") == (v.ID == "") || !utf8.ValidString(v.Name) || !utf8.ValidString(v.ID) {
		return errors.New("openai: invalid Chat audio voice")
	}
	return nil
}

// MarshalJSON emits the native string-or-ID voice union.
func (v ChatCompletionVoice) MarshalJSON() ([]byte, error) {
	if err := v.validate(); err != nil {
		return nil, err
	}
	if v.Name != "" {
		return json.Marshal(v.Name)
	}
	return json.Marshal(struct {
		ID string `json:"id"`
	}{v.ID})
}

// UnmarshalJSON decodes the native string-or-ID voice union.
func (v *ChatCompletionVoice) UnmarshalJSON(data []byte) error {
	var decoded ChatCompletionVoice
	if json.Unmarshal(data, &decoded.Name) != nil {
		var reference struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &reference); err != nil {
			return err
		}
		decoded.ID = reference.ID
	}
	if err := decoded.validate(); err != nil {
		return err
	}
	*v = decoded
	return nil
}

func validateChatAudioOptions(options OpenAIOptions) error {
	invalid := errors.New("openai: invalid Chat audio options")
	if options.Modalities != nil && len(options.Modalities) == 0 {
		return invalid
	}
	seen := map[string]bool{}
	for _, modality := range options.Modalities {
		if (modality != "text" && modality != "audio") || seen[modality] {
			return invalid
		}
		seen[modality] = true
	}
	if seen["audio"] && options.Audio == nil {
		return invalid
	}
	if options.Audio != nil {
		switch options.Audio.Format {
		case "wav", "aac", "mp3", "flac", "opus", "pcm16":
		default:
			return invalid
		}
		return options.Audio.Voice.validate()
	}
	return nil
}

// ChatCompletionAudioReference refers to a prior native assistant audio response.
type ChatCompletionAudioReference struct {
	ID string `json:"id"`
}

// ChatCompletionMessageOptions carries an assistant audio history reference.
// Use it on Message.ProviderOptions, not CallOptions.ProviderOptions.
type ChatCompletionMessageOptions struct {
	ChatTextParts []ChatCacheTextPart           `json:"chatTextParts,omitempty"`
	Breakpoint    *ChatCacheBreakpoint          `json:"prompt_cache_breakpoint,omitempty"`
	Audio         *ChatCompletionAudioReference `json:"audio,omitempty"`
}

// ProviderKey returns the common compatible message metadata namespace.
func (ChatCompletionMessageOptions) ProviderKey() string { return "openaiCompatible" }

func validateAudioHistory(extra map[string]any) error {
	audio, ok := extra["audio"]
	if !ok || audio == nil {
		return nil
	}
	raw, err := json.Marshal(audio)
	if err != nil {
		return err
	}
	var ref ChatCompletionAudioReference
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&ref) != nil || ref.ID == "" || !utf8.ValidString(ref.ID) {
		return errors.New("openai: invalid assistant audio reference")
	}
	return nil
}
