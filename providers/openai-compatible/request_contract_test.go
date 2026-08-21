package openaicompatible

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compatibleStringPointer(value string) *string { return &value }

func compatibleBoolPointer(value bool) *bool { return &value }

func compatibleIntegerPointer(value int64) *provider.LanguageModelNumber {
	number := provider.LanguageModelNumberFromInt64(value)
	return &number
}

func compatibleFloatPointer(t *testing.T, value float64) *provider.LanguageModelNumber {
	t.Helper()
	number, err := provider.LanguageModelNumberFromFloat64(value)
	require.NoError(t, err)
	return &number
}

func TestBuildRequest_ExactRequestNumbers(t *testing.T) {
	model := &model{modelID: "test", providerName: "test"}
	request, warnings, err := model.buildRequest(provider.CallOptions{
		MaxOutputTokens: compatibleFloatPointer(t, 100.5),
		Seed:            compatibleFloatPointer(t, 4.5),
		TopK:            compatibleFloatPointer(t, 2.5),
	}, false)
	require.NoError(t, err)
	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"max_tokens":100.5`)
	assert.Contains(t, string(encoded), `"seed":4.5`)
	assert.NotContains(t, string(encoded), `"top_k"`)
	assert.True(t, compatibleWarningHasFeature(warnings, "topK"))

	largeRequest, _, err := model.buildRequest(provider.CallOptions{
		MaxOutputTokens: compatibleIntegerPointer(9007199254740993),
	}, false)
	require.NoError(t, err)
	encoded, err = json.Marshal(largeRequest)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"max_tokens":9007199254740993`)
}

func TestBuildRequest_EmptyFileDataArms(t *testing.T) {
	model := &model{modelID: "test", providerName: "test"}
	t.Run("empty image data is serialized", func(t *testing.T) {
		data := provider.BytesDataContent(nil)
		request, _, err := model.buildRequest(provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", data))},
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"url":"data:image/png;base64,"`)
	})

	t.Run("empty PDF data is serialized", func(t *testing.T) {
		data := provider.BytesDataContent(nil)
		request, _, err := model.buildRequest(provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/pdf", data))},
		}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"file_data":"data:application/pdf;base64,"`)
	})

	t.Run("empty text arm is explicitly unsupported", func(t *testing.T) {
		data := provider.TextDataContent("")
		_, _, err := model.buildRequest(provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", data))},
		}, false)
		require.ErrorContains(t, err, "text file parts are not supported")
	})
}

func TestBuildRequest_RejectsInvalidRequestArms(t *testing.T) {
	model := &model{modelID: "test", providerName: "test"}
	data := provider.TextDataContent("value")
	invalidNumber := provider.LanguageModelNumber{}
	invalid := []provider.CallOptions{
		{Prompt: []provider.Message{{Role: provider.Role("unsupported")}}},
		{TopK: &invalidNumber},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{
			Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain", Filename: "response.txt",
		})}},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "value", ToolName: "inactive"})}},
	}
	for _, options := range invalid {
		_, _, err := model.buildRequest(options, false)
		require.ErrorContains(t, err, "invalid request")
	}
}

func compatibleWarningHasFeature(warnings []provider.Warning, feature string) bool {
	for _, warning := range warnings {
		if warning.Feature == feature {
			return true
		}
	}
	return false
}
