package openaicompatible

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestRejectUnsupportedBeforeNativeIO(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	model := New("test", WithBaseURL(server.URL))
	seed := 1
	options := provider.CallOptions{RejectUnsupported: true, TopK: &seed, Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: "hello"}}}}}
	_, err := model.DoGenerate(t.Context(), options)
	require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	_, err = model.DoStream(t.Context(), options)
	require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	require.Zero(t, calls)
}
