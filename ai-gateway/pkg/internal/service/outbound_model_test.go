package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicModel_HardenedResponseBoundaries(t *testing.T) {
	validUnary := `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"}],"model":"backend-private","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`

	t.Run("unary exact limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, validUnary)
		}))
		defer server.Close()
		model := hardenedTestModel(t, server, int64(len(validUnary)), time.Second, "anthropic")
		result, err := model.DoGenerate(context.Background(), modelTestOptions())
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{name: "unary success over limit", status: http.StatusOK, contentType: "application/json", body: validUnary},
		{name: "unary error over limit", status: http.StatusBadGateway, contentType: "application/json", body: strings.Repeat("x", 512)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			model := hardenedTestModel(t, server, 128, time.Second, "anthropic")
			_, err := model.DoGenerate(context.Background(), modelTestOptions())
			require.Error(t, err)
			assert.True(t, errors.Is(err, outbound.ErrResponseTooLarge) || strings.Contains(err.Error(), "response exceeds byte limit"))
		})
	}
}

func TestAnthropicModel_HardenedTimeoutAndUnboundedStream(t *testing.T) {
	t.Run("response-header timeout", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			select {
			case <-request.Context().Done():
			case <-release:
			}
		}))
		model := hardenedTestModel(t, server, 1024, 50*time.Millisecond, "anthropic")
		_, err := model.DoGenerate(context.Background(), modelTestOptions())
		require.Error(t, err)
		close(release)
		server.Close()
	})

	t.Run("stream above the response bound", func(t *testing.T) {
		initial := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"backend-private\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"
		delta := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + strings.Repeat("x", 256) + "\"}}\n\n"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, initial)
			_, _ = fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
			_, _ = fmt.Fprint(w, delta)
		}))
		defer server.Close()
		model := hardenedTestModel(t, server, int64(len(initial)+32), time.Second, "anthropic")
		result, err := model.DoStream(context.Background(), modelTestOptions())
		require.NoError(t, err)
		var text string
		for part := range result.Stream {
			if part.Type == provider.PartError && part.APICallError != nil {
				assert.NotErrorIs(t, part.APICallError, outbound.ErrResponseTooLarge)
			}
			if part.Type == provider.PartTextDelta {
				text += part.Delta
			}
		}
		assert.Equal(t, strings.Repeat("x", 256), text)
	})
}

func hardenedTestModel(t *testing.T, server *httptest.Server, limit int64, headerTimeout time.Duration, providerType string) provider.LanguageModel {
	t.Helper()
	client, err := outbound.NewModelClient(headerTimeout, limit)
	require.NoError(t, err)
	file := config.File{
		Models: map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "provider", Model: "backend-private"}}},
	}
	modelCatalog, err := BuildCatalog(file, map[string]config.ResolvedProvider{
		"provider": {Type: providerType, APIKey: "explicit-key", BaseURL: server.URL},
	}, client, identityModelFactory)
	require.NoError(t, err)
	resolved, err := modelCatalog.ResolveModel(context.Background(), "public")
	require.NoError(t, err)
	return resolved.Model
}

func modelTestOptions() provider.CallOptions {
	maxTokens := 64
	return provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: &maxTokens}
}

func TestOpenAICompatibleModel_UsesHardenedClient(t *testing.T) {
	body := `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"backend-private","choices":[{"index":0,"message":{"role":"assistant","content":"` + strings.Repeat("x", 512) + `"},"finish_reason":"stop"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()
	_, err := hardenedTestModel(t, server, 128, time.Second, "openai-compatible").DoGenerate(context.Background(), modelTestOptions())
	require.Error(t, err)
	assert.ErrorIs(t, err, outbound.ErrResponseTooLarge)
}

func TestOpenAIModel_UsesHardenedClient(t *testing.T) {
	t.Run("unary response bound", func(t *testing.T) {
		body := `{"id":"resp_test","object":"response","created_at":1,"status":"completed","model":"backend-private","output":[{"id":"msg_test","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","annotations":[],"text":"` + strings.Repeat("x", 512) + `"}]}]}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, body)
		}))
		defer server.Close()
		_, err := hardenedTestModel(t, server, 128, time.Second, "openai").DoGenerate(context.Background(), modelTestOptions())
		assert.ErrorIs(t, err, outbound.ErrResponseTooLarge)
	})

	t.Run("stream above the response bound", func(t *testing.T) {
		initial := "event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_test\",\"object\":\"response\",\"created_at\":1,\"status\":\"in_progress\",\"model\":\"backend-private\",\"output\":[]}}\n\n"
		delta := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_test\",\"output_index\":0,\"content_index\":0,\"delta\":\"" + strings.Repeat("x", 256) + "\"}\n\n"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, initial)
			_, _ = fmt.Fprint(w, delta)
		}))
		defer server.Close()
		result, err := hardenedTestModel(t, server, int64(len(initial)+32), time.Second, "openai").DoStream(context.Background(), modelTestOptions())
		require.NoError(t, err)
		var text string
		for part := range result.Stream {
			if part.Type == provider.PartError && part.APICallError != nil {
				assert.NotErrorIs(t, part.APICallError, outbound.ErrResponseTooLarge)
			}
			if part.Type == provider.PartTextDelta {
				text += part.Delta
			}
		}
		assert.Equal(t, strings.Repeat("x", 256), text)
	})

	t.Run("redirect rejection", func(t *testing.T) {
		redirected := 0
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected++ }))
		defer target.Close()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", target.URL+"/responses")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer server.Close()
		_, err := hardenedTestModel(t, server, 1024, time.Second, "openai").DoGenerate(context.Background(), modelTestOptions())
		require.Error(t, err)
		assert.Zero(t, redirected)
	})
}
