package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertPrompt_UpstreamFiveZeroThirty(t *testing.T) {
	t.Run("converts user S3 image URL", func(t *testing.T) {
		prompt := []provider.Message{provider.NewUserMessage(
			provider.TextPart("Describe the image"),
			provider.FilePart("image/png", provider.DataContent{URL: "s3://my-test-bucket/path/to/image.png"}),
		)}
		converted, warnings, err := convertPrompt(prompt, false, false)
		require.NoError(t, err)
		assert.Empty(t, warnings)
		require.Len(t, converted.Messages, 1)
		require.Len(t, converted.Messages[0].Content, 2)
		assert.Equal(t, &imageBlock{
			Format: "png",
			Source: imageSource{S3Location: &s3LocationBlock{URI: "s3://my-test-bucket/path/to/image.png"}},
		}, converted.Messages[0].Content[1].Image)
	})

	t.Run("converts tool result S3 image URL", func(t *testing.T) {
		output := &provider.ToolResultOutput{
			Type: provider.ToolOutputContent,
			Content: []provider.ToolResultContentValue{{
				Type:      provider.ToolContentFileURL,
				URL:       "s3://my-test-bucket/path/to/image.png",
				MediaType: "image/png",
			}},
		}
		prompt := []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call-123", "image-generator", output))}
		converted, warnings, err := convertPrompt(prompt, false, true)
		require.NoError(t, err)
		assert.Empty(t, warnings)
		require.Len(t, converted.Messages, 1)
		result := converted.Messages[0].Content[0].ToolResult
		require.NotNil(t, result)
		require.Len(t, result.Content, 1)
		assert.Equal(t, &imageBlock{
			Format: "png",
			Source: imageSource{S3Location: &s3LocationBlock{URI: "s3://my-test-bucket/path/to/image.png"}},
		}, result.Content[0].Image)
	})

	t.Run("sanitizes assistant tool names", func(t *testing.T) {
		prompt := []provider.Message{provider.NewAssistantMessage(
			provider.ToolCallPart("call-1", "$READFILE", json.RawMessage(`{}`)),
			provider.ToolCallPart("call-2", "exchange_delivered_order_items<|channel|>", json.RawMessage(`{}`)),
			provider.ToolCallPart("call-3", "$", json.RawMessage(`{}`)),
		)}
		converted, _, err := convertPrompt(prompt, false, true)
		require.NoError(t, err)
		require.Len(t, converted.Messages, 1)
		require.Len(t, converted.Messages[0].Content, 3)
		assert.Equal(t, "READFILE", converted.Messages[0].Content[0].ToolUse.Name)
		assert.Equal(t, "exchange_delivered_order_itemschannel", converted.Messages[0].Content[1].ToolUse.Name)
		assert.Equal(t, "_", converted.Messages[0].Content[2].ToolUse.Name)
	})
}

func TestModelSupportedURLs_S3Media(t *testing.T) {
	model := New("test-model")
	for _, mediaType := range []string{"image/*", "video/*"} {
		patterns := model.SupportedURLs()[mediaType]
		require.Len(t, patterns, 1)
		assert.True(t, patterns[0].MatchString("s3://bucket/media"))
		assert.False(t, patterns[0].MatchString("https://example.com/media"))
	}
}
