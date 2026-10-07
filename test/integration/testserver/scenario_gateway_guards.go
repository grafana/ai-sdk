package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/grafana/ai-sdk/schema"
)

func init() {
	for _, mode := range []string{"allow", "deny", "transform"} {
		registerScenario("gateway-guards-"+mode, func(w http.ResponseWriter, r *http.Request) { handleGatewayGuards(w, r, mode) })
	}
}

func handleGatewayGuards(w http.ResponseWriter, r *http.Request, mode string) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if mode != "allow" {
			w.Header().Set("Content-Type", "application/json")
			if mode == "deny" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`)
			} else {
				w.WriteHeader(http.StatusFailedDependency)
				_, _ = io.WriteString(w, `{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`)
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range []string{
			`{"type":"stream-start","warnings":[]}`,
			`{"type":"text-start","id":"safe"}`,
			`{"type":"text-delta","id":"safe","delta":"sanitized answer"}`,
			`{"type":"text-end","id":"safe"}`,
			`{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`,
		} {
			_, _ = io.WriteString(w, "data: "+frame+"\n\n")
		}
	}))
	defer server.Close()
	client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "local-token", BaseURL: server.URL})
	if err != nil {
		http.Error(w, "client setup failed", http.StatusInternalServerError)
		return
	}
	model, err := client.LanguageModel("guarded-model")
	if err != nil {
		http.Error(w, "model setup failed", http.StatusInternalServerError)
		return
	}
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object"}`))
	if err != nil {
		http.Error(w, "tool setup failed", http.StatusInternalServerError)
		return
	}
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("prompt-canary")), aisdk.WithMaxRetries(0),
		aisdk.WithTools(aisdk.ToolSet{"lookup": {InputSchema: inputSchema, Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			return nil, errors.New("unexpected tool execution")
		}}}),
	)
	stream := result.ToUIMessageStream(aisdk.OnUIMessageStreamError(func(err error) string {
		var failure *grafana.GatewayError
		if errors.As(err, &failure) {
			switch failure.Category {
			case grafana.GatewayForbidden:
				return "forbidden"
			case grafana.GatewayFailedDependency:
				return "failed dependency"
			}
		}
		return "inference failed"
	}))
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil && r.Context().Err() == nil {
		return
	}
}
