package main

import (
	"encoding/json"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionMetadata_History(t *testing.T) {
	server := chatCompletionMetadataServer()
	defer server.Close()
	model := openaicompatible.New("test", openaicompatible.WithBaseURL(server.URL), openaicompatible.WithChatCompletionMetadata())
	result := aisdk.StreamText(t.Context(), model, aisdk.WithModelMessages(provider.UserText("hi")))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, "hello world", result.Text())
	require.Len(t, result.Content(), 1)
	messages := result.ResponseMessages()
	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)
	assert.Equal(t, provider.ContentPartTypeText, messages[0].Content[0].Type)
	var namespace map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result.ProviderMetadata()["openai-compatible"], &namespace))
	var chat openaicompatible.ChatCompletionMetadata
	require.NoError(t, json.Unmarshal(namespace[openaicompatible.ChatCompletionMetadataKey], &chat))
	require.NotNil(t, chat.Refusal)
	assert.Equal(t, "Cannot help", *chat.Refusal)
	require.NotNil(t, chat.Logprobs)
	assert.Len(t, chat.Logprobs.Content, 2)
}
