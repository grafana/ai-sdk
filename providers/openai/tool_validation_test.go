package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_InvalidToolDoesNotInvokeProvider(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	model := NewResponses("key", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client())))
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "openai.web_search", Name: "search", Args: map[string]json.RawMessage{"limit": json.RawMessage(`{`)}}}}
	_, err := model.DoGenerate(context.Background(), options)
	require.Error(t, err)
	_, err = model.DoStream(context.Background(), options)
	require.Error(t, err)
	assert.Zero(t, requests)
}
