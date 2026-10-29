package v4

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeToolHistory_InvalidRequestDocument(t *testing.T) {
	expected := httptest.NewRecorder()
	newTestHandler(t).writeFailure(expected, invalidMappingFailure())
	const prompt = `"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"f","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"call","toolName":"other","output":{"type":"text","value":"result"}}]}]`
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		name string
		body string
		ctx  context.Context
	}{
		{name: "alone", body: `{` + prompt + `}`, ctx: t.Context()},
		{name: "beside a protected header", body: `{` + prompt + `,"headers":{"Authorization":"Bearer caller"}}`, ctx: t.Context()},
		{name: "beside gateway controls", body: `{` + prompt + `,"providerOptions":{"gateway":{"byok":{}}}}`, ctx: t.Context()},
		{name: "with a canceled context", body: `{` + prompt + `}`, ctx: ended},
	} {
		for _, streaming := range []string{"false", "true"} {
			t.Run(tc.name+" streaming="+streaming, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				request := validRequest(tc.body).WithContext(tc.ctx)
				request.Header.Set(HeaderStreaming, streaming)
				response := harness.serve(request)
				require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				assert.Equal(t, expected.Header(), response.Header())
				assert.Equal(t, expected.Body.Bytes(), response.Body.Bytes())
				assert.Zero(t, harness.resolver.callCount())
				assert.Zero(t, harness.model.callCount())
			})
		}
	}
}

func unresolvedProviderCalls(prompt []provider.Message) (map[string]string, *requestFailure) {
	history, err := execution.RebuildToolHistory(prompt)
	if err != nil {
		return nil, invalidMappingFailure()
	}
	return map[string]string(history), nil
}
