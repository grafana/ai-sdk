package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/grafana/ai-sdk/schema"
)

func init() {
	registerScenario("gateway-provider-metadata", handleGatewayProviderMetadata)
}

func handleGatewayProviderMetadata(w http.ResponseWriter, r *http.Request) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"type":"stream-start","warnings":[]}`,
			`{"type":"text-start","id":"msg_1","providerMetadata":{"openai":{"itemId":"msg_1"}}}`,
			`{"type":"text-delta","id":"msg_1","delta":"hello"}`,
			`{"type":"text-end","id":"msg_1","providerMetadata":{"openai":{"itemId":"msg_1"}}}`,
			`{"type":"tool-call","toolCallId":"call","toolName":"weather","input":"{}","providerMetadata":{"anthropic":{"caller":{"type":"direct"}}}}`,
			`{"type":"tool-result","toolCallId":"call","toolName":"weather","result":{"conditions":"sunny"},"providerMetadata":{"anthropic":{"caller":{"type":"direct"}}}}`,
			`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"stop"}}`,
		} {
			_, _ = w.Write([]byte("data: " + event + "\n\n"))
		}
	}))
	defer gateway.Close()
	client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "test", BaseURL: gateway.URL})
	if err != nil {
		http.Error(w, "invalid test gateway", http.StatusInternalServerError)
		return
	}
	model, err := client.LanguageModel("public")
	if err != nil {
		http.Error(w, "invalid test model", http.StatusInternalServerError)
		return
	}
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object"}`))
	if err != nil {
		http.Error(w, "invalid test schema", http.StatusInternalServerError)
		return
	}
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("hello")),
		aisdk.WithTools(aisdk.ToolSet{"weather": {InputSchema: inputSchema}}),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
