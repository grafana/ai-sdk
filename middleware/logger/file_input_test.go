package logger

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeMessages_FileInputs(t *testing.T) {
	secret := "private-file-name"
	file := provider.FilePart("application/pdf", provider.TextDataContent("private-file-text"))
	file.Filename = &secret
	file.ProviderOptions = provider.ProviderOptions{"provider": provider.RawProviderOption{Raw: json.RawMessage(`{"private":"option"}`)}}
	result := provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{
		Type: provider.ToolOutputContent,
		Content: []provider.ToolResultContentValue{
			{Type: provider.ToolContentText, Text: "visible text"},
			{Type: provider.ToolContentFile, Data: &provider.DataContent{URL: "https://example.test/private"}, MediaType: "application/pdf", Filename: &secret},
		},
	})
	original := []provider.Message{provider.NewUserMessage(file), provider.NewToolMessage(result)}

	redacted := sanitizeMessages(original, CaptureOptions{ToolOutputs: true})
	assert.Nil(t, redacted[0].Content[0].Data)
	assert.Nil(t, redacted[0].Content[0].Filename)
	require.NotNil(t, redacted[1].Content[0].Output)
	require.Len(t, redacted[1].Content[0].Output.Content, 1)
	assert.Equal(t, "visible text", redacted[1].Content[0].Output.Content[0].Text)
	assert.NotNil(t, original[0].Content[0].Data)
	assert.Equal(t, &secret, original[0].Content[0].Filename)
	require.Len(t, original[1].Content[0].Output.Content, 2)

	encoded, err := json.Marshal(redacted)
	require.NoError(t, err)
	for _, private := range []string{"private-file-name", "private-file-text", "https://example.test/private", "option"} {
		assert.NotContains(t, string(encoded), private)
	}

	captured := sanitizeMessages(original, CaptureOptions{Files: true, ToolOutputs: true})
	assert.Equal(t, &secret, captured[0].Content[0].Filename)
	require.Len(t, captured[1].Content[0].Output.Content, 2)
}
