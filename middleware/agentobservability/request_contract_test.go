package agentobservability

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func observabilityStringPointer(value string) *string { return &value }

func observabilityBoolPointer(value bool) *bool { return &value }

func observabilityIntegerPointer(value int) *provider.LanguageModelNumber {
	number := provider.LanguageModelNumberFromInt(value)
	return &number
}

func TestControlsFromCallOptions_ExactNumberHandling(t *testing.T) {
	large := provider.LanguageModelNumberFromInt64(9007199254740993)
	controls := controlsFromCallOptions(provider.CallOptions{MaxOutputTokens: &large})
	require.NotNil(t, controls.MaxTokens)
	assert.Equal(t, int64(9007199254740993), *controls.MaxTokens)

	fraction, err := provider.LanguageModelNumberFromFloat64(1.5)
	require.NoError(t, err)
	controls = controlsFromCallOptions(provider.CallOptions{MaxOutputTokens: &fraction})
	assert.Nil(t, controls.MaxTokens)
}

func TestContentFilePartToAgento11y_UsesRequestFilename(t *testing.T) {
	data := provider.BytesDataContent([]byte("not-an-image"))
	part := provider.ContentPart{
		Type: provider.ContentPartTypeFile, Data: &data, MediaType: "",
		FilePartFilename: observabilityStringPointer("image.png"), Filename: "ignored.webm",
	}
	mapped, ok := contentFilePartToAgento11y(part, "file")
	require.True(t, ok)
	assert.Equal(t, "image/png", mapped.Media.MIMEType)
	assert.Equal(t, "image.png", mapped.Media.Name)
}
