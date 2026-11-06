package nativeoptions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/require"
)

func TestCompatibleImageDetailPreservesGuards(t *testing.T) {
	image := provider.FilePart("image/png", provider.URLDataContent("https://example.test/original.png"))
	image.ProviderOptions = provider.BuildProviderOptions(compatible.ImageOptions{Detail: compatible.ImageDetailHigh})
	require.NoError(t, Compatible(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(image)}}, "vendor"))
	for _, fields := range []string{
		`{"imageDetail":"high","image_url":{"url":"https://example.test/replaced.png"}}`,
		`{"imageDetail":"high","type":"text"}`,
		`{"imageDetail":"high","file":{"file_id":"replacement"}}`,
		`{"imageDetail":"high","input_audio":{"data":"replacement","format":"wav"}}`,
		`{"imageDetail":"high","text":"replacement"}`,
	} {
		image.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(fields)}}
		require.Error(t, Compatible(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(image)}}, "vendor"), fields)
	}
}
