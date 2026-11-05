package openaicompatible

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNativeResponseMetadataMap(t *testing.T) {
	for _, value := range []string{`{}`, `{"tag":"native"}`} {
		chat, err := decodeChatCompletionMetadata([]byte(`{"metadata":`+value+`}`), false)
		require.NoError(t, err)
		assert.False(t, chat.Empty())
		raw, err := json.Marshal(chat)
		require.NoError(t, err)
		assert.JSONEq(t, `{"metadata":`+value+`}`, string(raw))
		global, err := globalChatMetadata([]byte(`{"metadata":` + value + `}`))
		require.NoError(t, err)
		assert.Equal(t, chat.Metadata, global.Metadata)
	}
	for _, value := range []string{`{"tag":1}`, `{"tag":"` + strings.Repeat("x", 513) + `"}`, `{"` + strings.Repeat("x", 65) + `":"v"}`} {
		_, err := decodeChatCompletionMetadata([]byte(`{"metadata":`+value+`}`), false)
		require.Error(t, err)
	}
	m := map[string]string{}
	for i := 0; i < 17; i++ {
		m[string(rune('a'+i))] = "v"
	}
	require.Error(t, (ChatCompletionMetadata{Metadata: m}).Validate())
}
