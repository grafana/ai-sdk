package openaicompatible

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageDetailOption(t *testing.T) {
	for _, detail := range []ImageDetail{ImageDetailAuto, ImageDetailLow, ImageDetailHigh, ImageDetailOriginal} {
		t.Run(string(detail), func(t *testing.T) {
			part := provider.FilePart("image/png", provider.URLDataContent("https://example.test/image.png"))
			part.ProviderOptions = provider.BuildProviderOptions(ImageOptions{Detail: detail})
			before := part.ProviderOptions["openaiCompatible"]
			for _, streaming := range []bool{false, true} {
				m := &model{modelID: "vision", providerName: defaultProviderName}
				body, _, err := m.buildRequest(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(part)}}, streaming)
				require.NoError(t, err)
				messages := body["messages"].([]chatMessage)
				parts := messages[0].Content.([]chatContentPart)
				assert.Equal(t, detail, parts[0].ImageURL.Detail)
				assert.Equal(t, "https://example.test/image.png", parts[0].ImageURL.URL)
				assert.NotContains(t, parts[0].ExtraFields, "imageDetail")
			}
			assert.Equal(t, before, part.ProviderOptions["openaiCompatible"])
		})
	}
	t.Run("invalid detail", func(t *testing.T) {
		part := provider.FilePart("image/png", provider.URLDataContent("https://example.test/i"))
		part.ProviderOptions = provider.BuildProviderOptions(ImageOptions{Detail: "invented"})
		_, _, err := (&model{modelID: "vision", providerName: defaultProviderName}).buildRequest(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(part)}}, false)
		require.Error(t, err)
	})
}
