package openaicompatible

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const chatMetadataPayload = `{"choices":[{"message":{"content":"hello","refusal":"Cannot help"},"delta":{"content":"hello","refusal":"Cannot help"},"finish_reason":"stop","logprobs":{"content":null,"refusal":[{"token":"Cannot","logprob":-0.1,"bytes":[67,97],"top_logprobs":[{"token":"No","logprob":-2,"bytes":null}]}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":0,"audio_tokens":2,"cache_write_tokens":4},"completion_tokens_details":{"reasoning_tokens":1,"audio_tokens":2,"accepted_prediction_tokens":0,"rejected_prediction_tokens":3}}}`

func TestChatCompletionMetadata_HTTP(t *testing.T) {
	for _, name := range []string{"openai-compatible", "custom.chat"} {
		for _, enabled := range []bool{false, true} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/enabled=%t/stream=%t", name, enabled, streaming), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body map[string]any
						assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
						assert.NotContains(t, body, "preserveChatCompletionMetadata")
						assert.Equal(t, true, body["logprobs"])
						if streaming {
							_, err := fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chatMetadataPayload)
							assert.NoError(t, err)
						} else {
							_, err := fmt.Fprint(w, chatMetadataPayload)
							assert.NoError(t, err)
						}
					}))
					defer server.Close()
					options := []Option{WithBaseURL(server.URL), WithProviderName(name)}
					if enabled {
						options = append(options, WithChatCompletionMetadata())
					}
					m := New("test", options...)
					key := "custom"
					if name == "openai-compatible" {
						key = "openaiCompatible"
					}
					call := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, ProviderOptions: provider.ProviderOptions{key: provider.RawProviderOption{Key: key, Raw: json.RawMessage(`{"logprobs":true,"top_logprobs":1}`)}}}
					var metadata provider.ProviderMetadata
					var usage provider.Usage
					if streaming {
						result, err := m.DoStream(t.Context(), call)
						require.NoError(t, err)
						for part := range result.Stream {
							require.NotEqual(t, provider.PartError, part.Type)
							assert.NotEqual(t, provider.PartCustom, part.Type)
							if part.Type == provider.PartFinish {
								metadata = part.ProviderMetadata
								require.NotNil(t, part.Usage)
								usage = *part.Usage
							}
						}
					} else {
						result, err := m.DoGenerate(t.Context(), call)
						require.NoError(t, err)
						metadata, usage = result.ProviderMetadata, result.Usage
						assert.Empty(t, result.Warnings)
					}
					require.Len(t, metadata, 1)
					var namespace map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(metadata[key], &namespace))
					assert.JSONEq(t, `0`, string(namespace["acceptedPredictionTokens"]))
					if enabled {
						var chat ChatCompletionMetadata
						require.NoError(t, json.Unmarshal(namespace[ChatCompletionMetadataKey], &chat))
						require.NotNil(t, chat.Refusal)
						assert.Equal(t, "Cannot help", *chat.Refusal)
						require.NotNil(t, chat.Logprobs)
						assert.Len(t, chat.Logprobs.Refusal, 1)
						assert.NotContains(t, string(namespace[ChatCompletionMetadataKey]), "usage")
					} else {
						assert.NotContains(t, namespace, ChatCompletionMetadataKey)
					}
					assert.Contains(t, string(usage.Raw), `"cache_write_tokens":4`)
					assert.Contains(t, string(usage.Raw), `"audio_tokens":2`)
				})
			}
		}
	}
}

func TestChatCompletionMetadata_ToleratesInvalidExtensions(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		warning       bool
	}{
		{"invalid refusal", `{"choices":[{"message":{"content":"ok","refusal":7},"finish_reason":"stop"}]}`, true},
		{"invalid probability", `{"choices":[{"message":{"content":"ok"},"logprobs":{"content":[{"token":"a","logprob":1}]},"finish_reason":"stop"}]}`, true},
		{"unused choice", `{"choices":[{"message":{"content":"ok","refusal":"no"},"finish_reason":"stop"},{"message":{"refusal":7},"logprobs":7}]}`, false},
		{"invalid text byte", "{\"choices\":[{\"message\":{\"content\":\"\xff\",\"refusal\":\"no\"},\"finish_reason\":\"stop\"}]}", false},
		{"empty metadata", `{"choices":[{"message":{"content":"ok"},"logprobs":{}}],"usage":{}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseGenerateResponse([]byte(tc.payload), nil, "custom.chat", "custom", func() string { return "id" }, true)
			require.NoError(t, err)
			require.NotEmpty(t, result.Content)
			if tc.warning {
				require.Len(t, result.Warnings, 1)
				assert.NotContains(t, string(result.ProviderMetadata["custom"]), ChatCompletionMetadataKey)
			} else {
				assert.Empty(t, result.Warnings)
			}
			if tc.name == "empty metadata" {
				assert.JSONEq(t, `{}`, string(result.ProviderMetadata["custom"]))
			}
		})
	}
}

func TestChatCompletionMetadata_MultiChunkStream(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprintf("malformed=%t", malformed), func(t *testing.T) {
			chunks := []string{`{"choices":[{"delta":{"role":"assistant"}}]}`, `{"choices":[{"delta":{"refusal":"Cannot "},"logprobs":{"refusal":[{"token":"Cannot","logprob":-0.1}]}}]}`}
			if malformed {
				chunks = append(chunks, `{"choices":[{"delta":{"refusal":7,"content":"still here"}}]}`)
			}
			chunks = append(chunks, `{"choices":[{"delta":{"refusal":"help"},"logprobs":{"refusal":[{"token":"help","logprob":-0.2}]},"finish_reason":"stop"}]}`, `{"choices":[],"usage":{"completion_tokens":2,"prompt_tokens_details":{"cache_write_tokens":3}}}`, "[DONE]")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := fmt.Fprint(w, "data: "+strings.Join(chunks, "\n\ndata: ")+"\n\n")
				assert.NoError(t, err)
			}))
			defer server.Close()
			m := New("test", WithBaseURL(server.URL), WithChatCompletionMetadata())
			result, err := m.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			require.NoError(t, err)
			errors, finishes := 0, 0
			text := ""
			for part := range result.Stream {
				assert.NotEqual(t, provider.PartCustom, part.Type)
				switch part.Type {
				case provider.PartError:
					errors++
					require.NotNil(t, part.APICallError)
					assert.False(t, part.APICallError.IsRetryable)
					assert.ErrorIs(t, part.APICallError, ErrInvalidChatCompletionMetadata)
				case provider.PartTextDelta:
					text += part.Delta
				case provider.PartFinish:
					finishes++
					require.NotNil(t, part.FinishReason)
					assert.Equal(t, provider.FinishReasonStop, part.FinishReason.Unified)
					var namespace map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(part.ProviderMetadata["openai-compatible"], &namespace))
					var chat ChatCompletionMetadata
					require.NoError(t, json.Unmarshal(namespace[ChatCompletionMetadataKey], &chat))
					require.NotNil(t, chat.Refusal)
					assert.Equal(t, "Cannot help", *chat.Refusal)
					require.NotNil(t, chat.Logprobs)
					assert.Len(t, chat.Logprobs.Refusal, 2)
					assert.Contains(t, string(part.Usage.Raw), `"cache_write_tokens":3`)
				}
			}
			assert.Equal(t, 1, finishes)
			if malformed {
				assert.Equal(t, 1, errors)
				assert.Equal(t, "still here", text)
			} else {
				assert.Zero(t, errors)
			}
		})
	}
}

func TestChatCompletionMetadata_Validate(t *testing.T) {
	token := "a"
	negative, positive, nan := -1.0, 1.0, math.NaN()
	for _, tc := range []struct {
		name        string
		probability ChatCompletionTokenProbability
	}{
		{"missing token", ChatCompletionTokenProbability{Logprob: &negative}},
		{"missing probability", ChatCompletionTokenProbability{Token: &token}},
		{"positive", ChatCompletionTokenProbability{Token: &token, Logprob: &positive}},
		{"NaN", ChatCompletionTokenProbability{Token: &token, Logprob: &nan}},
		{"invalid byte", ChatCompletionTokenProbability{Token: &token, Logprob: &negative, Bytes: []int{256}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chat := ChatCompletionMetadata{Logprobs: &ChatCompletionLogprobs{Content: []ChatCompletionTokenLogprob{{ChatCompletionTokenProbability: tc.probability}}}}
			assert.ErrorIs(t, chat.Validate(), ErrInvalidChatCompletionMetadata)
		})
	}
	t.Run("invalid namespace", func(t *testing.T) {
		metadata := provider.ProviderMetadata{"custom": json.RawMessage(`[]`)}
		_, err := withChatCompletionMetadata(metadata, "custom", ChatCompletionMetadata{Refusal: &token})
		require.Error(t, err)
		assert.JSONEq(t, `[]`, string(metadata["custom"]))
	})
}
