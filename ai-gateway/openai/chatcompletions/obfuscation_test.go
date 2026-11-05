package chatcompletions

import (
	"context"
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestChunkObfuscation(t *testing.T) {
	c := chunk{Choices: []chunkChoice{{Delta: delta{Content: ptr("hello")}}}}
	for _, suffix := range []string{`,"stream":true`, `,"stream":true,"stream_options":{"include_obfuscation":true}`, `,"stream":true,"stream_options":{"include_obfuscation":false}`} {
		r := requestWith(t, suffix)
		a, err := r.marshalChunk(c)
		require.NoError(t, err)
		b, err := r.marshalChunk(c)
		require.NoError(t, err)
		var got map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(a, &got))
		if r.StreamOptions != nil && !*r.StreamOptions.IncludeObfuscation {
			assert.NotContains(t, got, "obfuscation")
			assert.Equal(t, a, b)
			continue
		}
		assert.Contains(t, got, "obfuscation")
		assert.NotEqual(t, a, b)
		assert.Equal(t, 0, len(a)%64)
		delete(got, "obfuscation")
		unpacked, err := json.Marshal(got)
		require.NoError(t, err)
		original, err := json.Marshal(c)
		require.NoError(t, err)
		assert.JSONEq(t, string(original), string(unpacked))
	}
}

func TestOfficialGoObfuscationControl(t *testing.T) {
	f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(textParts()...), nil
	}}
	server := httptest.NewServer(testHandler(t, f))
	defer server.Close()
	client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"))
	for _, enabled := range []bool{true, false} {
		params := openaisdk.ChatCompletionNewParams{Model: "alias", Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("hello")}, StreamOptions: openaisdk.ChatCompletionStreamOptionsParam{IncludeObfuscation: openaisdk.Bool(enabled), IncludeUsage: openaisdk.Bool(true)}}
		stream := client.Chat.Completions.NewStreaming(t.Context(), params)
		count := 0
		for stream.Next() {
			count++
			var chunk map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(stream.Current().RawJSON()), &chunk))
			_, present := chunk["obfuscation"]
			assert.Equal(t, enabled, present)
		}
		require.NoError(t, stream.Err())
		require.NoError(t, stream.Close())
		assert.Greater(t, count, 2)
	}
}
