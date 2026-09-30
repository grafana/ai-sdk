package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasoningHighLevelTwoRequestReplay(t *testing.T) {
	for _, tc := range []struct{ name, start, end, want string }{
		{"end signature", ``, `,"providerMetadata":{"anthropic":{"signature":"final"}}`, `{"anthropic":{"signature":"final"}}`},
		{"omission preserves", `,"providerMetadata":{"openai":{"itemId":"r","reasoningEncryptedContent":null}}`, ``, `{"openai":{"itemId":"r","reasoningEncryptedContent":null}}`},
		{"end replaces", `,"providerMetadata":{"openai":{"itemId":"old","reasoningEncryptedContent":null}}`, `,"providerMetadata":{"openai":{"itemId":"new","reasoningEncryptedContent":"final"}}`, `{"openai":{"itemId":"new","reasoningEncryptedContent":"final"}}`},
		{"empty clears", `,"providerMetadata":{"anthropic":{"signature":"old"}}`, `,"providerMetadata":{}`, ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var replay provider.CallOptions
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 2 {
					require.NoError(t, json.NewDecoder(r.Body).Decode(&replay))
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, event := range []string{
					`{"type":"stream-start","warnings":[]}`,
					`{"type":"reasoning-start","id":"raw"` + tc.start + `}`,
					`{"type":"reasoning-delta","id":"raw","delta":" private  "}`,
					`{"type":"reasoning-end","id":"raw"` + tc.end + `}`,
					finishEvent, "[DONE]",
				} {
					_, _ = fmt.Fprint(w, sseFrame(event))
				}
			}, nil)
			model, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			// Use default retries: valid paid reasoning-only output must succeed
			// after exactly one request, not be retried as an adaptation error.
			first, err := aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.NewUserMessage(provider.TextPart("think"))))
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Len(t, first.Reasoning, 1)
			_, err = aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(first.Response.Messages...))
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.Len(t, replay.Prompt, 1)
			require.Len(t, replay.Prompt[0].Content, 1)
			part := replay.Prompt[0].Content[0]
			assert.Equal(t, " private  ", part.Text)
			if tc.want == "" {
				assert.Empty(t, part.ProviderOptions)
			} else {
				raw, err := json.Marshal(part.ProviderOptions)
				require.NoError(t, err)
				assert.JSONEq(t, tc.want, string(raw))
			}
		})
	}
}

func TestReasoningDecode(t *testing.T) {
	result, err := decodeGenerate([]byte(`{"content":[{"type":"reasoning","text":"","providerMetadata":{"openai":{"itemId":"r","reasoningEncryptedContent":null}}},{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""},"providerMetadata":{}}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{"reasoning":0}}}`))
	require.NoError(t, err)
	require.Len(t, result.Content, 2)
	assert.Equal(t, provider.ContentReasoning, result.Content[0].Type)
	assert.JSONEq(t, `{"itemId":"r","reasoningEncryptedContent":null}`, string(result.Content[0].ProviderMetadata["openai"]))
	assert.True(t, result.Content[1].Data.IsData())
	require.NotNil(t, result.Content[1].ProviderMetadata)
	for _, event := range []string{
		`{"type":"reasoning-start","id":"1","providerMetadata":{}}`,
		`{"type":"reasoning-delta","id":"1","delta":""}`,
		`{"type":"reasoning-end","id":"1","providerMetadata":{"anthropic":{"signature":"final"}}}`,
		`{"type":"reasoning-file","mediaType":"image/png","data":{"type":"url","url":"https://example.test/file"}}`,
	} {
		_, err := decodeStreamPart([]byte(event))
		require.NoError(t, err, event)
	}
}
