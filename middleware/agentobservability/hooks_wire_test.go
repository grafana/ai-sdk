package agentobservability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestHooksMiddleware_WireResponses(t *testing.T) {
	signed := provider.ReasoningPart("thinking")
	signed.ProviderOptions = provider.ProviderOptions{
		"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"signature":"signature-private"}`)},
	}
	serverCall := provider.ToolCallPart("call-1", "lookup", json.RawMessage(`{"n":1}`))
	serverCall.ProviderExecuted = true
	serverCall.ProviderOptions = provider.ProviderOptions{
		"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"type":"server_tool_use"}`)},
	}
	for _, tc := range []struct {
		name    string
		body    string
		prompt  []provider.Message
		want    []provider.Message
		wantErr error
	}{
		{name: "empty transform is no-op", body: `{"action":"allow","transformed_input":{}}`, want: []provider.Message{provider.UserText("original")}},
		{name: "empty collections are no-op", body: `{"action":"allow","transformed_input":{"messages":[],"tools":[]}}`, want: []provider.Message{provider.UserText("original")}},
		{name: "tools-only transform fails closed", body: `{"action":"allow","transformed_input":{"tools":[{"name":"lookup"}]}}`, wantErr: ErrHookTransformFailed},
		{name: "unknown role becomes user", body: `{"action":"allow","transformed_input":{"messages":[{"role":"unknown","parts":[{"kind":"text","text":"filtered"}]}]}}`, want: []provider.Message{provider.UserText("filtered")}},
		{name: "unknown kind with text becomes text", body: `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"unknown","text":"filtered"}]}]}}`, want: []provider.Message{provider.UserText("filtered")}},
		{name: "unsupported parts are dropped", body: `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"unknown"},{"kind":"text","text":""},{"kind":"text","text":"filtered"}]}]}}`, want: []provider.Message{provider.UserText("filtered")}},
		{name: "part metadata is discarded", body: `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"text","text":"filtered","metadata":{"provider_type":"unknown"}}]}]}}`, want: []provider.Message{provider.UserText("filtered")}},
		{name: "empty normalized message fails closed", body: `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"unknown"}]}]}}`, wantErr: ErrHookTransformFailed},
		{name: "preview-only transform fails closed", body: `{"action":"allow","transformed_input":{"conversation_preview":"filtered"}}`, wantErr: ErrHookTransformFailed},
		{name: "decode error fails closed", body: `{"action":"allow","transformed_input":{"messages":[{"role":{}}]}}`, wantErr: agento11y.ErrHookTransportFailed},
		{name: "deny wins over transform", body: `{"action":"deny","reason":"blocked","transformed_input":{"messages":[{"role":"user","parts":[]}]}}`, wantErr: ErrHookDenied},
		{name: "deny survives invalid tool schema encoding", body: `{"action":"deny","transformed_input":{"tools":[{"name":"lookup","input_schema_json":false}]}}`, wantErr: ErrHookDenied},
		{name: "base64 tool input", body: `{"action":"allow","transformed_input":{"messages":[{"role":2,"parts":[{"kind":"tool_call","tool_call":{"id":"call-1","name":"lookup","input_json":"eyJuIjo5MDA3MTk5MjU0NzQwOTkzfQ=="}}]}]}}`, want: []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart("call-1", "lookup", json.RawMessage(`{"n":9007199254740993}`)))}},
		{name: "base64 tool result", body: `{"action":"allow","transformed_input":{"messages":[{"role":"tool","parts":[{"kind":"tool_result","tool_result":{"tool_call_id":"call-1","name":"lookup","content_json":"eyJuIjoxfQ=="}}]}]}}`, want: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call-1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"n":1}`)}))}},
		{name: "signed reasoning retained without omitted parts", body: `{"action":"allow","transformed_input":{"messages":[{"role":"assistant","parts":[{"kind":"thinking","thinking":"thinking"}]}]}}`, prompt: []provider.Message{provider.NewAssistantMessage(signed, provider.TextPart("omitted"))}, want: []provider.Message{provider.NewAssistantMessage(signed)}},
		{name: "changed signed reasoning fails closed", body: `{"action":"allow","transformed_input":{"messages":[{"role":"assistant","parts":[{"kind":"thinking","thinking":"changed"}]}]}}`, prompt: []provider.Message{provider.NewAssistantMessage(signed)}, wantErr: ErrHookTransformFailed},
		{name: "provider tool discriminator loss fails closed", body: `{"action":"allow","transformed_input":{"messages":[{"role":"assistant","parts":[{"kind":"tool_call","metadata":{"provider_type":"server_tool_use"},"tool_call":{"id":"call-1","name":"lookup","input_json":"eyJuIjoxfQ=="}}]}]}}`, prompt: []provider.Message{provider.NewAssistantMessage(serverCall)}, wantErr: ErrHookTransformFailed},
		{name: "undisclosed media fails closed", body: `{"action":"allow","transformed_input":{"messages":[{"role":"user","parts":[{"kind":"text","text":"filtered"}]}]}}`, prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("original"), provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/private.png"}))}, wantErr: ErrHookTransformFailed},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/generate", true: "/stream"}[streaming], func(t *testing.T) {
				h := newHooksWireTestServer(t, json.RawMessage(tc.body))
				client := h.clientWithHooksEnabled()
				t.Cleanup(func() { require.NoError(t, client.Shutdown(context.Background())) })
				model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
				wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
					ClientResolver: func(context.Context) *agento11y.Client { return client },
				})}})
				prompt := tc.prompt
				if prompt == nil {
					prompt = []provider.Message{provider.UserText("original")}
				}
				params := provider.CallOptions{Prompt: prompt}
				var err error
				if streaming {
					var result *provider.StreamResult
					result, err = wrapped.DoStream(context.Background(), params)
					if err == nil {
						for range result.Stream {
						}
					}
				} else {
					_, err = wrapped.DoGenerate(context.Background(), params)
				}
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
					assert.Zero(t, model.generateHit+model.streamHit)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, 1, model.generateHit+model.streamHit)
				assert.Equal(t, tc.want, model.lastParams.Prompt)
				assert.Equal(t, prompt, params.Prompt)
			})
		}
	}
}

func TestHooksMiddleware_WireRequest(t *testing.T) {
	type capturedRequest struct {
		body   []byte
		header http.Header
	}
	requests := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "/api/v1/hooks:evaluate", r.URL.Path)
		requests <- capturedRequest{body: body, header: r.Header.Clone()}
		_, err = io.WriteString(w, `{"action":"allow"}`)
		assert.NoError(t, err)
	}))
	defer server.Close()
	client := agento11y.NewClient(agento11y.Config{
		API:              agento11y.APIConfig{Endpoint: server.URL},
		GenerationExport: agento11y.GenerationExportConfig{Protocol: agento11y.GenerationExportProtocolNone},
		Hooks:            agento11y.HooksConfig{Enabled: true, FailOpen: agento11y.BoolPtr(false), Timeout: 2 * time.Second},
	})
	defer func() { require.NoError(t, client.Shutdown(context.Background())) }()
	parent := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	ctx := trace.ContextWithSpanContext(context.Background(), parent)
	ctx = agento11y.WithConversationID(ctx, "conversation-1")
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
		ClientResolver: func(context.Context) *agento11y.Client { return client },
	})}})
	schema := json.RawMessage(`{"type":"object","properties":{"n":{"const":9007199254740993}}}`)
	params := provider.CallOptions{
		Prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("hello"), provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/media-private"}))},
		Tools:  []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: schema}},
	}
	_, err := wrapped.DoGenerate(ctx, params)
	require.NoError(t, err)
	request := <-requests
	assert.Equal(t, "2000", request.header.Get("X-Agento11y-Hook-Timeout-Ms"))
	assert.Empty(t, request.header.Get("X-Sigil-Hook-Timeout-Ms"))
	assert.NotContains(t, string(request.body), "media-private")
	assert.NotContains(t, string(request.body), `"input_schema":`)
	var decoded struct {
		Context agento11y.HookContext `json:"context"`
		Input   struct {
			Tools []struct {
				InputSchemaJSON []byte `json:"input_schema_json"`
			} `json:"tools"`
		} `json:"input"`
	}
	require.NoError(t, json.Unmarshal(request.body, &decoded))
	assert.Equal(t, "conversation-1", decoded.Context.ConversationID)
	assert.Equal(t, parent.TraceID().String(), decoded.Context.TraceID)
	assert.NotEmpty(t, decoded.Context.SpanID)
	require.Len(t, decoded.Input.Tools, 1)
	assert.JSONEq(t, string(schema), string(decoded.Input.Tools[0].InputSchemaJSON))
	assert.Contains(t, string(decoded.Input.Tools[0].InputSchemaJSON), `"const":9007199254740993`)
	assert.Equal(t, params, model.lastParams)
}
