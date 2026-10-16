package bedrock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customToolResultOptions() provider.CallOptions {
	return toolResultFileCallOptions(provider.ToolResultContentValue{
		Type: provider.ToolContentCustom,
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"type":"tool-reference","toolName":"search"}`)},
		},
	})
}

func newCountingModel(t *testing.T) (provider.LanguageModel, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	model := New(testAnthropicModel, WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
	return model, &requests
}

func callBoth(t *testing.T, model provider.LanguageModel, opts provider.CallOptions) map[string]error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, generateErr := model.DoGenerate(ctx, opts)
	_, streamErr := model.DoStream(ctx, opts)
	return map[string]error{"DoGenerate": generateErr, "DoStream": streamErr}
}

func TestUnsupportedFunctionality_CustomToolResultContent(t *testing.T) {
	t.Run("rejected before any HTTP request when tools are active", func(t *testing.T) {
		model, requests := newCountingModel(t)

		for name, err := range callBoth(t, model, customToolResultOptions()) {
			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality, name)
			assert.ErrorContains(t, err, "unsupported tool content part type: custom", name)
		}
		assert.Zero(t, requests.Load())
	})

	t.Run("filtered without rejection when no tools are active", func(t *testing.T) {
		opts := customToolResultOptions()
		opts.Tools = nil

		req, warnings, _ := mustBuildRequest(t, testAnthropicModel, opts)

		assert.Empty(t, req.Messages)
		assert.Contains(t, warningFeatures(warnings), "toolContent")
	})

	t.Run("filtered without rejection for toolChoice none", func(t *testing.T) {
		opts := customToolResultOptions()
		none := provider.ToolChoice{Type: provider.ToolChoiceNone}
		opts.ToolChoice = &none

		req, warnings, _ := mustBuildRequest(t, testAnthropicModel, opts)

		assert.Nil(t, req.ToolConfig)
		assert.Empty(t, req.Messages)
		assert.Contains(t, warningFeatures(warnings), "toolContent")
	})

	t.Run("existing file warnings are unchanged", func(t *testing.T) {
		url := provider.URLDataContent("https://example.com/a.png")

		req, warnings, _ := mustBuildRequest(t, testAnthropicModel, toolResultFileCallOptions(provider.ToolResultContentValue{
			Type: provider.ToolContentFile, Data: &url, MediaType: "image/png",
		}))

		require.Len(t, req.Messages, 1)
		assert.Equal(t, []string{"toolResultFileURL"}, warningFeatures(warnings))
	})
}

func TestUnsupportedFunctionality_ExistingConversionErrors(t *testing.T) {
	file := func(data provider.DataContent, mediaType string) provider.ContentPart {
		return provider.ContentPart{Type: provider.ContentPartTypeFile, Data: &data, MediaType: mediaType}
	}
	toolFile := func(data provider.DataContent, mediaType string) provider.CallOptions {
		return toolResultFileCallOptions(provider.ToolResultContentValue{Type: provider.ToolContentFile, Data: &data, MediaType: mediaType})
	}
	user := func(part provider.ContentPart) provider.CallOptions {
		return provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(part)}}
	}

	tests := []struct {
		name    string
		opts    provider.CallOptions
		message string
	}{
		{"provider reference", user(file(provider.ReferenceDataContent(json.RawMessage(`{"bedrock":"ref"}`)), "image/png")), "bedrock: file parts with provider references are not supported"},
		{"non-S3 URL", user(file(provider.URLDataContent("https://example.com/a.png"), "image/png")), "bedrock: file URL data is not supported"},
		{"S3 URL with unsupported top-level type", user(file(provider.URLDataContent("s3://bucket/a.txt"), "text/plain")), "bedrock: file URL data is only supported for images and videos"},
		{"S3 URL with unsupported image type", user(file(provider.URLDataContent("s3://bucket/a.avif"), "image/avif")), `bedrock: image media type "image/avif" is not supported`},
		{"S3 URL with unsupported video type", user(file(provider.URLDataContent("s3://bucket/a.avi"), "video/avi")), `bedrock: video media type "video/avi" is not supported`},
		{"inline unsupported image type", user(file(provider.Base64DataContent("AA=="), "image/avif")), `bedrock: image media type "image/avif" is not supported`},
		{"inline unsupported video type", user(file(provider.Base64DataContent("AA=="), "video/avi")), `bedrock: video media type "video/avi" is not supported`},
		{"inline unsupported file type", user(file(provider.Base64DataContent("AA=="), "application/x-unknown")), `bedrock: file media type "application/x-unknown" is not supported`},
		{"tool result unsupported image type", toolFile(provider.Base64DataContent("AA=="), "image/avif"), `bedrock: image media type "image/avif" is not supported`},
		{"tool result unsupported video type", toolFile(provider.Base64DataContent("AA=="), "video/avi"), `bedrock: video media type "video/avi" is not supported`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model, requests := newCountingModel(t)

			for name, err := range callBoth(t, model, tc.opts) {
				require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality, name)
				assert.EqualError(t, err, tc.message+": provider: unsupported functionality", name)
			}
			assert.Zero(t, requests.Load())
		})
	}
}

func TestBuildRequest_AssistantCustomPartsAreSkipped(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: []provider.Message{
		provider.UserText("hi"),
		provider.NewAssistantMessage(
			provider.TextPart("answer"),
			provider.ContentPart{Type: provider.ContentPartTypeCustom, Kind: "openai.compaction"},
		),
		provider.UserText("next"),
	}})

	require.Len(t, req.Messages, 3)
	require.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "answer", req.Messages[1].Content[0].Text)
	assert.Empty(t, warnings)
}
