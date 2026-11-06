package openaicompatible

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// ChatPrediction is the native static predicted-output configuration. Content is
// a JSON string or a nonempty array of text parts, validated before transport.
type ChatPrediction struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content"`
}

// ChatWebSearchOptions controls native Chat search models.
type ChatWebSearchOptions struct {
	SearchContextSize string              `json:"search_context_size,omitempty"`
	UserLocation      *ChatSearchLocation `json:"user_location,omitempty"`
}

// ChatSearchLocation is the native approximate location union.
type ChatSearchLocation struct {
	Type        string                   `json:"type"`
	Approximate *ChatApproximateLocation `json:"approximate"`
}

// ChatApproximateLocation contains the user's optional search location hints.
type ChatApproximateLocation struct {
	City     string `json:"city,omitempty"`
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// Validate checks the complete static prediction union.
func (p *ChatPrediction) Validate() error {
	if p == nil {
		return nil
	}
	if p.Type != "content" || !utf8.Valid(p.Content) {
		return fmt.Errorf("openai: invalid prediction")
	}
	var text string
	if json.Unmarshal(p.Content, &text) == nil && !bytes.Equal(bytes.TrimSpace(p.Content), []byte("null")) {
		return nil
	}
	var parts []struct {
		Type       string               `json:"type"`
		Text       *string              `json:"text"`
		Breakpoint *ChatCacheBreakpoint `json:"prompt_cache_breakpoint,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(p.Content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&parts) != nil || decoder.Decode(new(any)) != io.EOF || len(parts) == 0 {
		return fmt.Errorf("openai: invalid prediction content")
	}
	for _, part := range parts {
		if part.Type != "text" || part.Text == nil || part.Breakpoint.Validate() != nil {
			return fmt.Errorf("openai: invalid prediction text part")
		}
	}
	return nil
}

// Validate checks native search controls without imposing model-specific limits.
func (w *ChatWebSearchOptions) Validate() error {
	if w == nil {
		return nil
	}
	switch w.SearchContextSize {
	case "", "low", "medium", "high":
	default:
		return fmt.Errorf("openai: invalid search context size")
	}
	if w.UserLocation != nil {
		l := w.UserLocation
		if l.Type != "approximate" || l.Approximate == nil {
			return fmt.Errorf("openai: invalid search location")
		}
		for _, value := range []string{l.Approximate.City, l.Approximate.Country, l.Approximate.Region, l.Approximate.Timezone} {
			if !utf8.ValidString(value) {
				return fmt.Errorf("openai: invalid search location text")
			}
		}
	}
	return nil
}

// ChatCacheBreakpoint marks an exact reusable prefix boundary.
type ChatCacheBreakpoint struct {
	Mode string `json:"mode"`
	TTL  string `json:"ttl,omitempty"`
}

// Validate enforces the native cache breakpoint contract.
func (b *ChatCacheBreakpoint) Validate() error {
	if b == nil {
		return nil
	}
	if b.Mode != "explicit" || (b.TTL != "" && b.TTL != "30m" && b.TTL != "12h") {
		return fmt.Errorf("openai: invalid cache breakpoint")
	}
	return nil
}
