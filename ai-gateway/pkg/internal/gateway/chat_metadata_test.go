package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedCatalog_ChatMetadata(t *testing.T) {
	for _, name := range []string{"", "custom.chat"} {
		for _, malformed := range []bool{false, true} {
			t.Run(fmt.Sprintf("provider=%s/malformed=%t", name, malformed), func(t *testing.T) {
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request map[string]any
					if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
						return
					}
					assert.NotContains(t, request, "preserveChatCompletionMetadata")
					metadata := `"refusal":"No"`
					if malformed {
						metadata = `"refusal":7`
					}
					usage := `{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cache_write_tokens":1,"audio_tokens":0},"completion_tokens_details":{"accepted_prediction_tokens":0}}`
					if request["stream"] == true {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, payload := range []string{
							`{"choices":[{"delta":{"role":"assistant"}}]}`,
							`{"choices":[{"delta":{"content":"hello",` + metadata + `},"logprobs":{"content":[{"token":"hello","logprob":-0.1}]}}]}`,
							`{"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`,
							`{"choices":[],"usage":` + usage + `}`,
							`[DONE]`,
						} {
							if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); !assert.NoError(t, err) {
								return
							}
						}
					} else {
						_, err := fmt.Fprintf(w, `{"choices":[{"message":{"content":"hello world",%s},"finish_reason":"stop","logprobs":{"content":[{"token":"hello","logprob":-0.1}]}}],"usage":%s}`, metadata, usage)
						assert.NoError(t, err)
					}
				}))
				defer backend.Close()
				factory := func(_ string, m provider.LanguageModel) (provider.LanguageModel, error) { return m, nil }
				catalog, err := service.BuildCatalog(config.File{Models: map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "native", Model: "test"}}}}, map[string]config.ResolvedProvider{"native": {Type: "openai-compatible", BaseURL: backend.URL, APIKey: "test-key", ProviderName: name}}, backend.Client(), factory)
				require.NoError(t, err)
				runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(catalog), Limits: execution.DefaultLimits})
				require.NoError(t, err)
				wire, err := providerv4.New(runtime)
				require.NoError(t, err)
				chat, err := chatcompletions.New(runtime)
				require.NoError(t, err)
				for _, streaming := range []bool{false, true} {
					for _, protocol := range []string{"chat", "wire"} {
						t.Run(fmt.Sprintf("%s/stream=%t", protocol, streaming), func(t *testing.T) {
							body := `{"model":"public","messages":[{"role":"user","content":"hi"}],"stream":` + strconv.FormatBool(streaming)
							if streaming {
								body += `,"stream_options":{"include_usage":true}`
							}
							body += `}`
							var handler http.Handler = chat
							if protocol == "wire" {
								handler = wire
								body = `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
							}
							path := chatcompletions.Path
							if protocol == "wire" {
								path = providerv4.LanguageModelPath
							}
							req := httptest.NewRequest("POST", path, strings.NewReader(body))
							req.Header.Set("Content-Type", "application/json")
							req.Header.Set(providerv4.HeaderSpecificationVersion, "4")
							req.Header.Set(providerv4.HeaderModelID, "public")
							req.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
							out := httptest.NewRecorder()
							handler.ServeHTTP(out, req)
							require.Equal(t, 200, out.Code, out.Body.String())
							result := out.Body.String()
							assert.Contains(t, result, "hello")
							assert.Contains(t, result, "world")
							assert.Contains(t, result, `"cache_write_tokens":1`)
							assert.Contains(t, result, `"accepted_prediction_tokens":0`)
							assert.NotContains(t, result, `"type":"custom"`)
							if !malformed {
								assert.Contains(t, result, `"refusal":"No"`)
								assert.Contains(t, result, `"token":"hello"`)
							}
							if protocol == "chat" {
								assert.NotContains(t, result, `"error":`)
								if streaming {
									assert.Contains(t, result, "[DONE]")
								}
							} else if streaming {
								assert.Contains(t, result, `"type":"finish"`)
							}
						})
					}
				}
			})
		}
	}
}
