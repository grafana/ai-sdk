package output

import (
	"github.com/grafana/ai-sdk/provider"
)

// TextOutput implements aisdk.Output as a no-op pass-through for plain text.
// It signals that no structured output processing should occur.
type TextOutput struct{}

// Text creates a TextOutput that passes through the LLM response unchanged.
func Text() *TextOutput {
	return &TextOutput{}
}

func (o *TextOutput) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{
		Type: provider.ResponseFormatText,
	}
}

func (o *TextOutput) ParseComplete(text string) (any, error) {
	return text, nil
}

func (o *TextOutput) ParsePartial(text string) (any, bool) {
	return text, true
}
