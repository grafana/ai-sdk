package openaicompatible

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/grafana/ai-sdk/provider"
	"strings"
	"unicode/utf8"
)

// ChatPromptCacheOptions configures native implicit or explicit prompt caching.
type ChatPromptCacheOptions struct {
	Mode string `json:"mode,omitempty"`
	TTL  string `json:"ttl,omitempty"`
}

// Validate checks the native request-level cache options. Breakpoint lifetimes
// have a separate contract and may additionally select twelve hours.
func (c *ChatPromptCacheOptions) Validate() error {
	if c == nil {
		return nil
	}
	if (c.Mode != "" && c.Mode != "implicit" && c.Mode != "explicit") || (c.TTL != "" && c.TTL != "30m") {
		return errors.New("openai: invalid prompt cache options")
	}
	return nil
}

// ChatCacheTextPart restores text boundaries that the common prompt represents
// as one string. The codec verifies its concatenation against that string.
type ChatCacheTextPart struct {
	Type       string               `json:"type"`
	Text       *string              `json:"text"`
	Breakpoint *ChatCacheBreakpoint `json:"prompt_cache_breakpoint,omitempty"`
}

func decodeCacheValue(value any, into any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(into)
}

func validateCachePartMetadata(meta map[string]any) error {
	if value, ok := meta["prompt_cache_breakpoint"]; ok {
		var breakpoint ChatCacheBreakpoint
		if decodeCacheValue(value, &breakpoint) != nil || breakpoint.Validate() != nil {
			return errors.New("openai: invalid cache breakpoint metadata")
		}
	}
	return nil
}

func restoreCacheTextParts(msg *chatMessage, options provider.ProviderOptions) error {
	meta, err := openAICompatibleMetadata(options)
	if err != nil {
		return err
	}
	value, ok := meta["chatTextParts"]
	if !ok {
		return nil
	}
	var parts []ChatCacheTextPart
	if decodeCacheValue(value, &parts) != nil || len(parts) == 0 {
		return errors.New("openai: invalid cache text boundaries")
	}
	var joined strings.Builder
	marked := false
	for _, p := range parts {
		if p.Type != "text" || p.Text == nil || !utf8.ValidString(*p.Text) || p.Breakpoint.Validate() != nil {
			return errors.New("openai: invalid cache text part")
		}
		joined.WriteString(*p.Text)
		marked = marked || p.Breakpoint != nil
	}
	text, ok := msg.Content.(string)
	if pointer, isPointer := msg.Content.(*string); isPointer && pointer == nil {
		text = ""
		ok = true
	}
	if !ok || text != joined.String() || !marked {
		return errors.New("openai: cache text boundaries changed message content")
	}
	msg.Content = parts
	delete(msg.ExtraFields, "chatTextParts")
	return nil
}

func hasCacheBreakpoint(options provider.ProviderOptions) bool {
	meta, err := openAICompatibleMetadata(options)
	return err == nil && meta["prompt_cache_breakpoint"] != nil
}
