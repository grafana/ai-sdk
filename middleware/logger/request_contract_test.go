package logger

import (
	"log/slog"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestSummaryAttrs_ExactNumbersAndPresence(t *testing.T) {
	large := provider.LanguageModelNumberFromInt64(9007199254740993)
	fraction, err := provider.LanguageModelNumberFromFloat64(1.5)
	require.NoError(t, err)
	explicitFalse := false
	attrs := requestSummaryAttrs(provider.CallOptions{
		MaxOutputTokens:  &large,
		TopK:             &fraction,
		IncludeRawChunks: &explicitFalse,
	}, CaptureOptions{})
	values := map[string]slog.Value{}
	for _, attr := range attrs {
		values[attr.Key] = attr.Value
	}
	assert.Equal(t, slog.KindInt64, values["ai_sdk.request.max_output_tokens"].Kind())
	assert.Equal(t, int64(9007199254740993), values["ai_sdk.request.max_output_tokens"].Int64())
	assert.Equal(t, slog.KindFloat64, values["ai_sdk.request.top_k"].Kind())
	assert.Equal(t, 1.5, values["ai_sdk.request.top_k"].Float64())
	assert.Equal(t, false, values["ai_sdk.request.include_raw_chunks"].Bool())

	absent := requestSummaryAttrs(provider.CallOptions{}, CaptureOptions{})
	for _, attr := range absent {
		assert.NotEqual(t, "ai_sdk.request.include_raw_chunks", attr.Key)
	}
}

func TestSanitizeContentPart_ClearsBothFilenameDirections(t *testing.T) {
	requestFilename := "request-secret.txt"
	data := provider.TextDataContent("secret")
	sanitized := sanitizeContentPart(provider.ContentPart{
		Type: provider.ContentPartTypeFile, Data: &data,
		FilePartFilename: &requestFilename, Filename: "response-secret.txt",
	}, CaptureOptions{})
	assert.Nil(t, sanitized.Data)
	assert.Nil(t, sanitized.FilePartFilename)
	assert.Empty(t, sanitized.Filename)
}
