package chatcompletions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_OfficialSDKUnary(t *testing.T) {
	server := httptest.NewServer(testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return textResult(), nil
	}}))
	defer server.Close()
	client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
	params := openaisdk.ChatCompletionNewParams{Model: "alias", Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("hello")}}
	result, err := client.Chat.Completions.New(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, "canonical", result.Model)
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "hello", result.Choices[0].Message.Content)
	assert.Equal(t, int64(5), result.Usage.TotalTokens)
	assert.True(t, strings.HasPrefix(result.ID, "chatcmpl-"))
	params.Model = "unknown"
	_, err = client.Chat.Completions.New(context.Background(), params)
	var api *openaisdk.Error
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 404, api.StatusCode)
	assert.NotContains(t, err.Error(), "private-provider")
}

func TestHandler_RequestBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, media, body string
		status                          int
	}{
		{"wrong method", "GET", Path, "application/json", basic, 405},
		{"query", "POST", Path + "?x=1", "application/json", basic, 404},
		{"media", "POST", Path, "text/plain", basic, 415},
		{"invalid utf8", "POST", Path, "application/json", string([]byte{255}), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				t.Error("unexpected generation")
				return nil, nil
			}})
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			assert.Equal(t, tc.status, w.Code)
		})
	}
}

func TestHandler_HostOwnsLimits(t *testing.T) {
	t.Run("large body and output without imposed deadline", func(t *testing.T) {
		large := strings.Repeat("a", 2<<20)
		h := testHandler(t, &fakeModel{generate: func(ctx context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
			_, ok := ctx.Deadline()
			assert.False(t, ok)
			require.Len(t, o.Prompt, 1)
			assert.Equal(t, large, o.Prompt[0].Content[0].Text)
			result := textResult()
			result.Content[0].Text = large
			return result, nil
		}})
		req := httptest.NewRequest("POST", Path, strings.NewReader(strings.Replace(basic, "hello", large, 1)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), large)
	})
	t.Run("host deadline", func(t *testing.T) {
		stopped := make(chan struct{})
		h := testHandler(t, &fakeModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			defer close(stopped)
			<-ctx.Done()
			return nil, ctx.Err()
		}})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		req := httptest.NewRequest("POST", Path, strings.NewReader(basic)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, 504, w.Code)
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("generation did not stop")
		}
	})
}

func TestHandler_SafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *provider.GenerateResult
		err    error
		status int
	}{
		{"internal", nil, errors.New("secret"), 500},
		{"upstream", nil, &provider.APICallError{StatusCode: 403, Message: "secret"}, 424},
		{"rate limit", nil, &provider.APICallError{StatusCode: 429, Message: "secret"}, 429},
		{"nil response", nil, nil, 500},
		{"invalid response", &provider.GenerateResult{}, nil, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return tc.result, tc.err
			}})
			req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(basic))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			assert.Equal(t, tc.status, w.Code)
			assert.NotContains(t, w.Body.String(), "secret")
		})
	}
}

func TestHandler_RuntimePanicAndHostErrors(t *testing.T) {
	h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { panic("private secret") }})
	req := httptest.NewRequest("POST", Path, strings.NewReader(basic))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	assert.Equal(t, 500, response.Code)
	assert.NotContains(t, response.Body.String(), "secret")
	var _ host.ErrorWriter = NewHostErrorWriter()
	for _, tc := range []struct {
		category host.ErrorCategory
		status   int
	}{{host.ErrorPermission, 403}, {host.ErrorBYOKDiscovery, 400}, {host.ErrorCategory(255), 500}} {
		response := httptest.NewRecorder()
		NewHostErrorWriter().WriteHostError(response, tc.category)
		assert.Equal(t, tc.status, response.Code)
	}
}

func TestHandler_PerAttemptTranslation(t *testing.T) {
	calls := 0
	primary := execution.Translate(&fakeModel{generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
		calls++
		require.Contains(t, o.ProviderOptions, "openai")
		assert.NotContains(t, o.ProviderOptions, "anthropic")
		return nil, &provider.APICallError{StatusCode: 503, IsRetryable: true}
	}}, execution.Target{Type: execution.TargetOpenAI})
	secondary := execution.Translate(&fakeModel{generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
		calls++
		require.Contains(t, o.ProviderOptions, "anthropic")
		assert.NotContains(t, o.ProviderOptions, "openai")
		return textResult(), nil
	}}, execution.Target{Type: execution.TargetAnthropic})
	model, err := fallback.New(primary, secondary)
	require.NoError(t, err)
	catalog, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "alias"}, Model: model.WithDecider(execution.FallbackDecider)}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(catalog), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", Path, strings.NewReader(strings.TrimSuffix(basic, "}")+`,"parallel_tool_calls":false}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	assert.Equal(t, 200, response.Code)
	assert.Equal(t, 2, calls)
}
