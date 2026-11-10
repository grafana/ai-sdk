package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"testing"
)

func TestChoices_CapturePrivacyAndUsage(t *testing.T) {
	result := &provider.GenerateResult{Choices: []provider.GenerateChoice{{Index: 0, Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "visible-output"}, {Type: provider.ContentReasoning, Text: "secret-reasoning"}}, ProviderMetadata: provider.ProviderMetadata{"native": json.RawMessage(`{"secret":"secret-metadata"}`)}}}}
	for _, capture := range []CaptureOptions{{}, {Outputs: true, MaxJSONBytes: 16000, MaxStringLen: 4096}} {
		var output bytes.Buffer
		slog.New(slog.NewJSONHandler(&output, nil)).LogAttrs(context.Background(), slog.LevelInfo, "test", generateResultAttrs(result, capture)...)
		assert.NotContains(t, output.String(), "secret-reasoning")
		assert.NotContains(t, output.String(), "secret-metadata")
		if capture.Outputs {
			assert.Contains(t, output.String(), "visible-output")
			assert.Contains(t, output.String(), "ai_sdk.response.choices")
		} else {
			assert.NotContains(t, output.String(), "visible-output")
		}
	}
	zero, one := 0, 1
	input, output := 10, 30
	stop := provider.FinishReason{Unified: provider.FinishReasonStop}
	length := provider.FinishReason{Unified: provider.FinishReasonLength}
	var summary streamSummary
	summary.observe(provider.StreamPart{Type: provider.PartFinish, ChoiceIndex: &zero, FinishReason: &stop, Usage: &provider.Usage{}})
	summary.observe(provider.StreamPart{Type: provider.PartFinish, ChoiceIndex: &one, FinishReason: &length, Usage: &provider.Usage{}})
	summary.observe(provider.StreamPart{Type: provider.PartChoicesFinish, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &input}, OutputTokens: provider.OutputTokenUsage{Total: &output}}})
	usage, ok := summary.usage.Usage()
	assert.True(t, ok)
	assert.Equal(t, &input, usage.InputTokens.Total)
	assert.Equal(t, &output, usage.OutputTokens.Total)
	assert.Equal(t, provider.FinishReasonOther, summary.finish.Unified)
	attrs := streamPartCaptureAttrs(provider.StreamPart{Type: provider.PartTextDelta, ChoiceIndex: &zero, Delta: "private"}, CaptureOptions{})
	assert.Equal(t, []slog.Attr{slog.Int("ai_sdk.stream.choice_index", 0)}, attrs)
}
