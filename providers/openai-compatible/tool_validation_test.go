package openaicompatible

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_InvalidToolDoesNotInvokeProvider(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	model := New("test-model", WithBaseURL(server.URL), WithAPIKey("key"))
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "openai.web_search", Name: "search", ProviderOptions: provider.ProviderOptions{}}}}
	_, err := model.DoGenerate(context.Background(), options)
	require.Error(t, err)
	_, err = model.DoStream(context.Background(), options)
	require.Error(t, err)
	assert.Zero(t, requests)
}
