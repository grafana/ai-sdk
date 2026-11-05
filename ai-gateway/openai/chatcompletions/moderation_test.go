package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerationProjection(t *testing.T) {
	const moderation = `{"input":{"type":"moderation_results","model":"moderator","results":[]},"output":{"type":"error","code":"unavailable","message":"moderator unavailable"}}`
	metadata := provider.ProviderMetadata{"openaiCompatible": json.RawMessage(`{"chatCompletion":{"moderation":` + moderation + `}}`)}
	for _, count := range []int{1, 2} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("n%d/stream%t", count, streaming), func(t *testing.T) {
				f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					r := textResult()
					if count == 2 {
						r = nativeChoiceResult()
					}
					r.ProviderMetadata = metadata
					return r, nil
				}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := textParts()
					if count == 2 {
						parts = nativeChoiceParts()
					} else {
						parts = append(parts[:1], append([]provider.StreamPart{{Type: provider.PartCustom, Kind: "openai-compatible.chat-completion", ProviderMetadata: metadata}}, parts[1:]...)...)
					}
					parts[len(parts)-1].ProviderMetadata = metadata
					return partsStream(parts...), nil
				}}
				h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
				body := fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":"hello"}],"n":%d,"stream":%t}`, count, streaming)
				req := httptest.NewRequest("POST", Path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				out := httptest.NewRecorder()
				h.ServeHTTP(out, req)
				require.Equal(t, 200, out.Code, out.Body.String())
				require.NotContains(t, out.Body.String(), `"error":{"message":"bad gateway"`)
				found := false
				verify := func(data string) {
					var c map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(data), &c))
					if m, ok := c["moderation"]; ok {
						assert.JSONEq(t, moderation, string(m))
						found = true
					}
				}
				if streaming {
					require.Contains(t, out.Body.String(), "[DONE]")
					for _, line := range strings.Split(out.Body.String(), "\n") {
						if strings.HasPrefix(line, "data: {") {
							verify(strings.TrimPrefix(line, "data: "))
						}
					}
				} else {
					verify(out.Body.String())
				}
				assert.True(t, found, "moderation must appear in the public response")
			})
		}
	}
}
