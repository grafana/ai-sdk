package openaicompatible

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestChatExtendedUsagePresence(t *testing.T) {
	for _, details := range []string{`{}`, `{"prompt_tokens_details":{"cache_write_tokens":0,"image_tokens":0,"text_tokens":0},"completion_tokens_details":{"text_tokens":0}}`} {
		chat, err := decodeChatCompletionMetadata([]byte(`{"usage":`+details+`}`), false)
		require.NoError(t, err)
		raw, err := json.Marshal(chat.Usage)
		require.NoError(t, err)
		assert.JSONEq(t, details, string(raw))
	}
	for _, field := range []string{"cache_write_tokens", "image_tokens", "text_tokens"} {
		_, err := decodeChatCompletionMetadata([]byte(`{"usage":{"prompt_tokens_details":{"`+field+`":-1}}}`), false)
		require.Error(t, err)
	}
	_, err := decodeChatCompletionMetadata([]byte(`{"usage":{"completion_tokens_details":{"text_tokens":-1}}}`), true)
	require.Error(t, err)
	raw := `{"usage":{"prompt_tokens_details":{"cache_write_tokens":0}}}`
	chat, err := decodeChatCompletionMetadata([]byte(raw), true)
	require.NoError(t, err)
	encoded, err := json.Marshal(chat)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(encoded), `"cache_write_tokens":0`))
}
