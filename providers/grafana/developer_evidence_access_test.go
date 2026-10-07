package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const developerEvidenceMetadata = `{"future":{"value":[null,false,0,"",{},[]]},"gateway":{"routing":{"originalModelId":"assistant","canonicalSlug":"grafana/assistant","resolvedProvider":"openai","resolvedProviderApiModelId":"native-model"},"evidence":{"requestedModelId":"assistant","canonicalModelId":"grafana/assistant","selectedAttempt":2,"attempts":[{"index":1,"provider":"anthropic","modelId":"primary","selection":"failed","nativeError":{"statusCode":429,"isRetryable":true}},{"index":2,"provider":"openai","modelId":"native-model","selection":"selected","completion":"completed"}]},"nativeMetadata":{"routing":{"resolvedProvider":"untrusted-native-claim"}}}}`

func developerEvidenceUnary() string {
	return fmt.Sprintf(`{"content":[{"type":"text","text":"hello"}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"providerMetadata":%s,"request":{"body":{"input":"native"}},"response":{"id":"native-response","modelId":"native-model","headers":{"x-native":"native"},"body":{"output":"native"}}}`, developerEvidenceMetadata)
}

func developerEvidenceStream(withError bool) string {
	parts := []string{
		`{"type":"stream-start","warnings":[]}`,
		`{"type":"response-metadata","id":"native-response","modelId":"native-model","timestamp":"2026-10-01T00:00:00Z"}`,
		`{"type":"text-start","id":"text"}`,
		`{"type":"text-delta","id":"text","delta":"before"}`,
	}
	if withError {
		parts = append(parts, fmt.Sprintf(`{"type":"error","error":{"message":"native account rejected","type":"failed_dependency","code":"failed_dependency","param":null,"statusCode":424,"retryable":false,"data":{"nativeError":{"statusCode":401,"isRetryable":false},"providerMetadata":%s}}}`, developerEvidenceMetadata))
	}
	parts = append(parts, `{"type":"text-delta","id":"text","delta":"after"}`, `{"type":"text-end","id":"text"}`, fmt.Sprintf(`{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":%s}`, developerEvidenceMetadata))
	return "data: " + strings.Join(parts, "\n\ndata: ") + "\n\n"
}

func developerEvidenceModel(t *testing.T, status int, contentType, body string) provider.LanguageModel {
	t.Helper()
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/aisdk/language-model", r.URL.Path)
		assert.Equal(t, "access-token", r.Header.Get("X-Access-Token"))
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Gateway", "hop")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}, nil)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	return m
}

func TestDeveloperEvidenceAccess_CurrentBaseline(t *testing.T) {
	t.Run("UnaryTransportAndOpaqueMetadata", func(t *testing.T) {
		body := developerEvidenceUnary()
		model := developerEvidenceModel(t, 200, "application/json", body)
		result, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		assert.Equal(t, "hello", result.Content[0].Text)
		assertDecodedMetadata(t, developerEvidenceMetadata, result.ProviderMetadata)
		assert.Empty(t, result.Response.ModelID)
		assert.Equal(t, "hop", result.Response.Headers["X-Gateway"])
		assert.JSONEq(t, body, string(result.Response.Body))
		assert.JSONEq(t, `[]`, jsonMember(t, result.Request.Body, "prompt"))
		var raw struct {
			ProviderMetadata provider.ProviderMetadata `json:"providerMetadata"`
		}
		require.NoError(t, json.Unmarshal(result.Response.Body, &raw))
		assert.JSONEq(t, jsonMember(t, []byte(developerEvidenceMetadata), "gateway"), string(raw.ProviderMetadata["gateway"]))
	})

	t.Run("StreamSetupFinishAndCommittedErrorBoundary", func(t *testing.T) {
		for _, withError := range []bool{false, true} {
			t.Run(fmt.Sprint(withError), func(t *testing.T) {
				model := developerEvidenceModel(t, 200, "text/event-stream", developerEvidenceStream(withError))
				result, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
				require.NoError(t, err)
				assert.Equal(t, "hop", result.Response.Headers["X-Gateway"])
				assert.JSONEq(t, `[]`, jsonMember(t, result.Request.Body, "prompt"))
				parts := collectParts(t, result)
				assert.Equal(t, "native-model", parts[1].ModelID)
				last := parts[len(parts)-1]
				assert.Equal(t, provider.PartFinish, last.Type)
				assertDecodedMetadata(t, developerEvidenceMetadata, last.ProviderMetadata)
				if withError {
					require.Equal(t, provider.PartError, parts[4].Type)
					require.NotNil(t, parts[4].APICallError)
					assert.Equal(t, 424, parts[4].APICallError.StatusCode)
					assert.Contains(t, string(parts[4].APICallError.Data), "nativeError")
					assert.NotContains(t, parts[4].APICallError.ResponseBody, "nativeError")
					assert.Equal(t, "after", parts[5].Delta)
				}
			})
		}
	})

	t.Run("DirectSetupAndAllFailedEnvelopeRetention", func(t *testing.T) {
		for _, attempts := range []string{
			`[{"index":1,"provider":"anthropic","modelId":"primary","selection":"failed","nativeError":{"statusCode":401,"isRetryable":false}}]`,
			`[{"index":1,"provider":"anthropic","modelId":"primary","selection":"failed","nativeError":{"statusCode":429,"isRetryable":true}},{"index":2,"provider":"openai","modelId":"native-model","selection":"failed","nativeError":{"statusCode":401,"isRetryable":false}}]`,
		} {
			body := fmt.Sprintf(`{"error":{"message":"candidate unavailable","type":"failed_dependency","code":"failed_dependency","param":null},"providerMetadata":{"gateway":{"evidence":{"attempts":%s}}}}`, attempts)
			model := developerEvidenceModel(t, 424, "application/json", body)
			_, generateErr := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			_, streamErr := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			for _, err := range []error{generateErr, streamErr} {
				var gatewayErr *GatewayError
				var apiErr *provider.APICallError
				require.ErrorAs(t, err, &gatewayErr)
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, 424, gatewayErr.StatusCode)
				assert.False(t, gatewayErr.IsRetryable)
				assert.JSONEq(t, body, string(apiErr.Data))
				assert.Equal(t, body, apiErr.ResponseBody)
			}
		}
		model := developerEvidenceModel(t, 424, "application/json", `{"error":{"message":12}}`)
		_, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		var apiErr *provider.APICallError
		require.ErrorAs(t, err, &apiErr)
		assert.False(t, apiErr.IsRetryable)
	})

	t.Run("HighLevelNormalizationAndConsumerConfiguration", func(t *testing.T) {
		for _, capture := range []bool{false, true} {
			t.Run(fmt.Sprint(capture), func(t *testing.T) {
				for _, withError := range []bool{false, true} {
					t.Run(fmt.Sprint(withError), func(t *testing.T) {
						ctx, cancel := context.WithCancel(context.Background())
						defer cancel()
						var mu sync.Mutex
						var observed []provider.StreamPart
						var hop string
						model := middleware.WrapLanguageModel(developerEvidenceModel(t, 200, "text/event-stream", developerEvidenceStream(withError)), middleware.Middleware{
							WrapStream: func(ctx context.Context, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
								result, err := p.DoStream(ctx)
								if err != nil {
									return nil, err
								}
								mu.Lock()
								if capture {
									hop = result.Response.Headers["X-Gateway"]
								}
								mu.Unlock()
								return middleware.TransformStream(ctx, result, func(part provider.StreamPart, emit func(provider.StreamPart)) {
									mu.Lock()
									if capture {
										observed = append(observed, part)
									}
									mu.Unlock()
									emit(part)
								}, nil), nil
							},
						})
						result := aisdk.StreamText(ctx, model, aisdk.WithModelMessages(provider.UserText("hi")), aisdk.WithMaxRetries(0))
						for range result.FullStream() {
						}
						result.Wait()
						assertDecodedMetadata(t, developerEvidenceMetadata, result.ProviderMetadata())
						if withError {
							require.Error(t, result.Err())
							var apiErr *provider.APICallError
							require.ErrorAs(t, result.Err(), &apiErr)
							assert.Equal(t, 424, apiErr.StatusCode)
							assert.Contains(t, string(apiErr.Data), "nativeError")
							assert.Equal(t, "beforeafter", result.Text())
						} else {
							require.NoError(t, result.Err())
							assert.Equal(t, "beforeafter", result.Text())
							assert.Equal(t, "native-model", result.Response().ModelID)
						}
						mu.Lock()
						if capture {
							assert.Equal(t, "hop", hop)
							require.NotEmpty(t, observed)
							assertDecodedMetadata(t, developerEvidenceMetadata, observed[len(observed)-1].ProviderMetadata)
						} else {
							assert.Empty(t, hop)
							assert.Empty(t, observed)
						}
						mu.Unlock()
					})
				}
			})
		}
		model := developerEvidenceModel(t, 200, "text/event-stream", developerEvidenceStream(false))
		generated, err := aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("hi")), aisdk.WithMaxRetries(0))
		require.NoError(t, err)
		assert.Equal(t, "beforeafter", generated.Text)
		assertDecodedMetadata(t, developerEvidenceMetadata, generated.ProviderMetadata)
		failed := developerEvidenceModel(t, 200, "text/event-stream", developerEvidenceStream(true))
		generated, err = aisdk.GenerateText(context.Background(), failed, aisdk.WithModelMessages(provider.UserText("hi")), aisdk.WithMaxRetries(0))
		require.Error(t, err)
		assert.Nil(t, generated)
	})

	t.Run("ConsumerSetupFailureAccess", func(t *testing.T) {
		for _, capture := range []bool{false, true} {
			var captures atomic.Int32
			body := `{"error":{"message":"candidate unavailable","type":"failed_dependency","code":"failed_dependency","param":null},"providerMetadata":{"gateway":{"evidence":{"attempts":[]}}}}`
			model := middleware.WrapLanguageModel(developerEvidenceModel(t, 424, "application/json", body), middleware.Middleware{
				WrapStream: func(ctx context.Context, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
					result, err := p.DoStream(ctx)
					if capture && err != nil {
						var gatewayErr *GatewayError
						var apiErr *provider.APICallError
						assert.ErrorAs(t, err, &gatewayErr)
						assert.ErrorAs(t, err, &apiErr)
						if apiErr != nil {
							assert.JSONEq(t, body, string(apiErr.Data))
						}
						captures.Add(1)
					}
					return result, err
				},
			})
			_, err := aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("hi")), aisdk.WithMaxRetries(0))
			require.Error(t, err)
			if capture {
				assert.EqualValues(t, 1, captures.Load())
			} else {
				assert.Zero(t, captures.Load())
			}
		}
	})

	t.Run("DiscoveryConfiguredRetention", func(t *testing.T) {
		body := configuredDiscoveryFixture(t)
		var calls atomic.Int32
		p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/v1/aisdk/config", r.URL.Path)
			assert.Equal(t, "access-token", r.Header.Get("X-Access-Token"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}, nil)
		rows, err := p.ListModels(context.Background())
		require.NoError(t, err)
		require.Len(t, rows, 1)
		stock, err := json.Marshal(rows)
		require.NoError(t, err)
		assert.Contains(t, string(stock), "primary")
		assert.Equal(t, "public", rows[0].ID)
		assert.Equal(t, "native-primary", rows[0].Gateway.Primary.ProviderModelID)
		assert.JSONEq(t, body, `{"models":`+string(stock)+`}`)
		p.limits.DiscoveryBytes = int64(len(body) - 1)
		rows, err = p.ListModels(context.Background())
		require.Error(t, err)
		assert.Nil(t, rows)
		assert.EqualValues(t, 2, calls.Load())
	})
}
