package openaicompatible

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type chatResponseContent struct {
	text  string
	array []provider.GenerateContentPart
}

type chatResponsePartType string

const (
	chatResponsePartText     chatResponsePartType = "text"
	chatResponsePartThinking chatResponsePartType = "thinking"
)

func (c *chatResponseContent) UnmarshalJSON(data []byte) error {
	if data[0] != '[' {
		c.array = nil
		return json.Unmarshal(data, &c.text)
	}

	var blocks []struct {
		Type     *chatResponsePartType `json:"type"`
		Text     json.RawMessage       `json:"text"`
		Thinking json.RawMessage       `json:"thinking"`
	}
	if err := json.Unmarshal(data, &blocks); err != nil {
		return err
	}
	parts := make([]provider.GenerateContentPart, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == nil {
			return errors.New("openai: content array member is missing a string type")
		}
		switch *block.Type {
		case chatResponsePartText:
			var text string
			if json.Unmarshal(block.Text, &text) == nil && text != "" {
				parts = append(parts, provider.GenerateContentPart{Type: provider.ContentText, Text: text})
			}
		case chatResponsePartThinking:
			if text := thinkingText(block.Thinking); text != "" {
				parts = append(parts, provider.GenerateContentPart{Type: provider.ContentReasoning, Text: text})
			}
		}
	}
	*c = chatResponseContent{array: parts}
	return nil
}

func thinkingText(data json.RawMessage) string {
	var fragments []json.RawMessage
	if json.Unmarshal(data, &fragments) != nil {
		return ""
	}
	var text strings.Builder
	for _, fragment := range fragments {
		var chunk struct {
			Type chatResponsePartType `json:"type"`
			Text string               `json:"text"`
		}
		if json.Unmarshal(fragment, &chunk) == nil && chunk.Type == chatResponsePartText {
			text.WriteString(chunk.Text)
		}
	}
	return text.String()
}

type chatReasoningContent struct {
	text    string
	present bool
}

func (r *chatReasoningContent) UnmarshalJSON(data []byte) error {
	r.present = string(data) != "null"
	return json.Unmarshal(data, &r.text)
}

func (r chatReasoningContent) selectText(fallback string, arrayContent bool) string {
	if (arrayContent && r.present) || (!arrayContent && r.text != "") {
		return r.text
	}
	return fallback
}
