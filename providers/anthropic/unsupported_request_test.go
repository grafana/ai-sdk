package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRejectUnsupportedBeforeNativeIO(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	model := New("test", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL)))
	seed := 1
	zero := 0.0
	cases := []provider.CallOptions{{Seed: &seed}, {PresencePenalty: &zero}, {FrequencyPenalty: &zero}}
	cases = append(cases, provider.CallOptions{Temperature: &zero, TopP: &zero})
	for _, options := range cases {
		options.RejectUnsupported = true
		options.Prompt = []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: "hello"}}}}
		_, err := model.DoGenerate(t.Context(), options)
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
		_, err = model.DoStream(t.Context(), options)
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	}
	require.Zero(t, calls)
}
