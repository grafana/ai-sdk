package bedrock

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bedrockStringPointer(value string) *string { return &value }

func bedrockOptionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func bedrockIntegerPointer(value int64) *provider.LanguageModelNumber {
	number := provider.LanguageModelNumberFromInt64(value)
	return &number
}

func bedrockIntegerValue(t *testing.T, number *provider.LanguageModelNumber) int64 {
	t.Helper()
	require.NotNil(t, number)
	value, ok := number.Int64()
	require.True(t, ok)
	return value
}

func bedrockFloatPointer(t *testing.T, value float64) *provider.LanguageModelNumber {
	t.Helper()
	number, err := provider.LanguageModelNumberFromFloat64(value)
	require.NoError(t, err)
	return &number
}

func TestBuildRequest_ExactRequestNumbers(t *testing.T) {
	t.Run("fractions serialize exactly", func(t *testing.T) {
		request, _, _, err := buildRequest("meta.llama", provider.CallOptions{
			MaxOutputTokens: bedrockFloatPointer(t, 100.5),
			TopK:            bedrockFloatPointer(t, 4.5),
		})
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"maxTokens":100.5`)
		assert.Contains(t, string(encoded), `"topK":4.5`)
	})

	t.Run("large historical integer serializes exactly", func(t *testing.T) {
		request, _, _, err := buildRequest("meta.llama", provider.CallOptions{
			MaxOutputTokens: bedrockIntegerPointer(9007199254740993),
		})
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"maxTokens":9007199254740993`)
	})

	t.Run("thinking adds budget without truncating fraction", func(t *testing.T) {
		request, warnings, _, err := buildRequest("anthropic.claude-sonnet-4-5", provider.CallOptions{
			MaxOutputTokens: bedrockFloatPointer(t, 100.5),
			ProviderOptions: provider.ProviderOptions{
				"bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"reasoningConfig":{"type":"enabled","budgetTokens":1000}}`)},
			},
		})
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"maxTokens":1100.5`)
		assert.Empty(t, warnings)
	})

	t.Run("thinking removes topK", func(t *testing.T) {
		request, warnings, _, err := buildRequest("anthropic.claude-sonnet-4-5", provider.CallOptions{
			TopK: bedrockFloatPointer(t, 4.5),
			ProviderOptions: provider.ProviderOptions{
				"bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"reasoningConfig":{"type":"enabled","budgetTokens":1000}}`)},
			},
		})
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), `"topK"`)
		assert.True(t, bedrockWarningHasFeature(warnings, "topK"))
	})
}

func TestBuildRequest_EmptyFileDataArms(t *testing.T) {
	t.Run("empty image data is serialized", func(t *testing.T) {
		data := provider.BytesDataContent(nil)
		request, _, _, err := buildRequest("meta.llama", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", data))},
		})
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"bytes":""`)
	})

	t.Run("empty text arm is serialized", func(t *testing.T) {
		data := provider.TextDataContent("")
		request, _, _, err := buildRequest("meta.llama", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", data))},
		})
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"bytes":""`)
	})
}

func TestBuildRequest_ThinkingMaxRejectsOverflow(t *testing.T) {
	_, _, _, err := buildRequest("anthropic.claude-sonnet-4-5", provider.CallOptions{
		MaxOutputTokens: bedrockIntegerPointer(math.MaxInt64),
		ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
			ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 1},
		}),
	})
	require.ErrorContains(t, err, "bedrock: max output tokens overflow")
}

func TestAddBedrockRequestNumber_InvalidPreservesProviderContext(t *testing.T) {
	_, err := addBedrockRequestNumber(provider.LanguageModelNumber{}, 1)
	require.ErrorContains(t, err, "bedrock: invalid language model number")
}

func TestBuildRequest_RejectsInvalidRequestArms(t *testing.T) {
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
		_, _, _, err := buildRequest("meta.llama", options)
		require.ErrorContains(t, err, "invalid request")
	}
}

func bedrockWarningHasFeature(warnings []provider.Warning, feature string) bool {
	for _, warning := range warnings {
		if warning.Feature == feature {
			return true
		}
	}
	return false
}
