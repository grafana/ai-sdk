package anthropic

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requestStringPointer(value string) *string { return &value }

func requestBoolPointer(value bool) *bool { return &value }

func requestIntegerPointer(value int64) *provider.LanguageModelNumber {
	number := provider.LanguageModelNumberFromInt64(value)
	return &number
}

func requestFloatPointer(t *testing.T, value float64) *provider.LanguageModelNumber {
	t.Helper()
	number, err := provider.LanguageModelNumberFromFloat64(value)
	require.NoError(t, err)
	return &number
}

func TestBuildParams_ExactRequestNumbers(t *testing.T) {
	t.Run("supported fractions reach final Anthropic request", func(t *testing.T) {
		params, _, warnings, _, err := buildParams("unknown-model", provider.CallOptions{
			MaxOutputTokens: requestFloatPointer(t, 1024.5),
			TopK:            requestFloatPointer(t, 4.5),
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		body := string(encoded)
		assert.JSONEq(t, `{"model":"unknown-model","max_tokens":1024.5,"top_k":4.5}`, body)
		assert.Equal(t, 1, strings.Count(body, `"max_tokens"`))
		assert.Equal(t, 1, strings.Count(body, `"top_k"`))
		assert.Empty(t, warnings)
	})

	t.Run("thinking removes fractional topK override", func(t *testing.T) {
		params, _, warnings, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			TopK: requestFloatPointer(t, 4.5),
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":1024}}`)},
			},
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), `"top_k"`)
		assert.True(t, warningHasFeature(warnings, "topK"))
		assert.False(t, warningHasFeature(warnings, "not-present"))
	})

	t.Run("sampling-rejecting model removes fractional topK override", func(t *testing.T) {
		params, _, warnings, _, err := buildParams("claude-opus-4-7", provider.CallOptions{TopK: requestFloatPointer(t, 4.5)}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), `"top_k"`)
		assert.True(t, warningHasFeature(warnings, "topK"))
	})

	t.Run("Vertex shares fractional override behavior", func(t *testing.T) {
		params, _, _, _, err := buildParamsWithCapabilities("unknown-model", provider.CallOptions{
			MaxOutputTokens: requestFloatPointer(t, 100.5),
			TopK:            requestFloatPointer(t, 2.5),
		}, false, vertexProviderCapabilities)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		body := string(encoded)
		assert.Contains(t, body, `"max_tokens":100.5`)
		assert.Contains(t, body, `"top_k":2.5`)
		assert.Equal(t, 1, strings.Count(body, `"max_tokens"`))
		assert.Equal(t, 1, strings.Count(body, `"top_k"`))
	})

	t.Run("Vertex thinking removes fractional topK override", func(t *testing.T) {
		params, _, warnings, _, err := buildParamsWithCapabilities("claude-sonnet-4-6", provider.CallOptions{
			TopK: requestFloatPointer(t, 2.5),
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":1024}}`)},
			},
		}, false, vertexProviderCapabilities)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), `"top_k"`)
		assert.True(t, warningHasFeature(warnings, "topK"))
	})

	t.Run("Vertex sampling-rejecting model removes fractional topK override", func(t *testing.T) {
		params, _, warnings, _, err := buildParamsWithCapabilities("claude-opus-4-7", provider.CallOptions{
			TopK: requestFloatPointer(t, 2.5),
		}, false, vertexProviderCapabilities)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), `"top_k"`)
		assert.True(t, warningHasFeature(warnings, "topK"))
	})

	t.Run("fractional max output preserves thinking arithmetic", func(t *testing.T) {
		params, _, _, _, err := buildParams("unknown-model", provider.CallOptions{
			MaxOutputTokens: requestFloatPointer(t, 100.5),
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":1000}}`)},
			},
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		body := string(encoded)
		assert.Contains(t, body, `"max_tokens":1100.5`)
		assert.Equal(t, 1, strings.Count(body, `"max_tokens"`))
	})

	t.Run("large historical integer remains exact on unknown model", func(t *testing.T) {
		params, _, _, _, err := buildParams("unknown-model", provider.CallOptions{
			MaxOutputTokens: requestIntegerPointer(9007199254740993),
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"max_tokens":9007199254740993`)
	})

	t.Run("thinking overflow preserves provider context", func(t *testing.T) {
		_, _, _, _, err := buildParams("unknown-model", provider.CallOptions{
			MaxOutputTokens: requestIntegerPointer(math.MaxInt64),
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":1}}`)},
			},
		}, false)
		require.ErrorContains(t, err, "anthropic: max output tokens overflow")
	})

	t.Run("invalid number preserves provider context", func(t *testing.T) {
		_, err := addRequestNumber(provider.LanguageModelNumber{}, 1)
		require.ErrorContains(t, err, "anthropic: invalid language model number")
	})
}

func TestBuildParams_EmptyFileDataAndFilenamePresence(t *testing.T) {
	t.Run("empty image data is serialized", func(t *testing.T) {
		data := provider.BytesDataContent(nil)
		params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", data))},
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"type":"base64"`)
		assert.Contains(t, string(encoded), `"data":""`)
	})

	t.Run("empty text arm is serialized", func(t *testing.T) {
		data := provider.TextDataContent("")
		params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/octet-stream", data))},
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"type":"text"`)
		assert.Contains(t, string(encoded), `"data":""`)
	})

	t.Run("filename preserves absent empty and non-empty", func(t *testing.T) {
		data := provider.TextDataContent("value")
		parts := []provider.ContentPart{
			provider.FilePart("text/plain", data),
			provider.FilePartWithFilename("text/plain", data, ""),
			provider.FilePartWithFilename("text/plain", data, "report.txt"),
		}
		params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(parts...)},
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(encoded, &body))
		messages := body["messages"].([]any)
		content := messages[0].(map[string]any)["content"].([]any)
		assert.NotContains(t, content[0].(map[string]any), "title")
		assert.Equal(t, "", content[1].(map[string]any)["title"])
		assert.Equal(t, "report.txt", content[2].(map[string]any)["title"])
	})
}

func TestBuildParams_RejectsInvalidRequestArms(t *testing.T) {
	data := provider.TextDataContent("value")
	invalidNumber := provider.LanguageModelNumber{}
	invalid := []provider.CallOptions{
		{Prompt: []provider.Message{{Role: provider.Role("unsupported")}}},
		{Seed: &invalidNumber},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{
			Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain", Filename: "response.txt",
		})}},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "value", ToolName: "inactive"})}},
	}
	for _, options := range invalid {
		_, _, _, _, err := buildParams("claude-sonnet-4-6", options, false)
		require.ErrorContains(t, err, "invalid request")
	}
}

func warningHasFeature(warnings []provider.Warning, feature string) bool {
	for _, warning := range warnings {
		if warning.Feature == feature {
			return true
		}
	}
	return false
}
