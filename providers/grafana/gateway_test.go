package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ provider.ProviderOption = GatewayOptions{}

func gatewayOptionsJSON(t *testing.T, option provider.ProviderOption) string {
	t.Helper()
	body, err := encodeRequest(provider.CallOptions{Prompt: []provider.Message{}, ProviderOptions: provider.ProviderOptions{option.ProviderKey(): option}})
	require.NoError(t, err)
	var request struct {
		ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	return string(request.ProviderOptions["gateway"])
}

func TestGatewayOptions_Encoding(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options GatewayOptions
		want    string
	}{
		{name: "unset", want: `{}`},
		{name: "nil only is absent", options: GatewayOptions{Models: []string{"b"}}, want: `{"models":["b"]}`},
		{name: "empty only is kept", options: GatewayOptions{Only: []string{}}, want: `{"only":[]}`},
		{name: "routing controls keep order", options: GatewayOptions{Models: []string{"b", "a"}, Order: []string{"openai", "anthropic"}, Only: []string{"anthropic", "openai"}}, want: `{"models":["b","a"],"order":["openai","anthropic"],"only":["anthropic","openai"]}`},
		{
			name: "request accounts and timeouts",
			options: GatewayOptions{
				Models: []string{"openai/y"},
				BYOK: map[string][]BYOKAccount{
					"anthropic": {{APIKey: "dummy-a1"}, {APIKey: "dummy-a2", BaseURL: "https://approved.example/anthropic"}},
					"openai":    {{APIKey: "dummy-o1", Organization: "org", Project: "proj"}},
				},
				ProviderTimeouts: &ProviderTimeouts{BYOK: map[string]int{"anthropic": 2000}},
			},
			want: `{"models":["openai/y"],"byok":{"anthropic":[{"apiKey":"dummy-a1"},{"apiKey":"dummy-a2","baseURL":"https://approved.example/anthropic"}],"openai":[{"apiKey":"dummy-o1","organization":"org","project":"proj"}]},"providerTimeouts":{"byok":{"anthropic":2000}}}`,
		},
		{name: "values are not validated", options: GatewayOptions{Order: []string{""}, ProviderTimeouts: &ProviderTimeouts{BYOK: map[string]int{"vertex": 0}}}, want: `{"order":[""],"providerTimeouts":{"byok":{"vertex":0}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typed := gatewayOptionsJSON(t, tc.options)
			assert.JSONEq(t, tc.want, typed)
			raw := gatewayOptionsJSON(t, provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(tc.want)})
			assert.JSONEq(t, raw, typed)
		})
	}
}

func TestGatewayOptions_OneGatewayRequest(t *testing.T) {
	calls := 0
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"gateway":{"models":["b","c","d"]}`)
		assert.Equal(t, "a", r.Header.Get("Ai-Language-Model-Id"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"providerOptions.gateway.providerTimeouts.byok must map anthropic or openai to whole milliseconds from 1 to 600000","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	})
	model, err := p.LanguageModel("a")
	require.NoError(t, err)
	_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}, ProviderOptions: provider.ProviderOptions{"gateway": GatewayOptions{Models: []string{"b", "c", "d"}}}})
	var gateway *GatewayError
	require.ErrorAs(t, err, &gateway)
	assert.Equal(t, GatewayInvalidRequest, gateway.Category)
	assert.False(t, gateway.IsRetryable)
	assert.Equal(t, 1, calls)
}

const gatewayMetadataFixture = `{"execution":{"requestedModelId":"alias","canonicalModelId":"public","attempts":[` +
	`{"provider":"openai","modelId":"a1","providerInstance":"one","outcome":"failed","error":{"message":"busy","type":"overloaded_error","code":900719925474099312345,"statusCode":503}},` +
	`{"provider":"anthropic","modelId":"b1","providerInstance":"two","outcome":"selected"}]},` +
	`"routing":{"originalModelId":"alias","canonicalSlug":"public","resolvedProvider":"openai","finalProvider":"anthropic","modelAttemptCount":2,"totalProviderAttemptCount":2,"modelAttempts":[` +
	`{"modelId":"alias","canonicalSlug":"public","success":false,"providerAttemptCount":1,"providerAttempts":[{"provider":"openai","success":false,"statusCode":503}]},` +
	`{"modelId":"backup","canonicalSlug":"backup","success":true,"providerAttemptCount":1,"providerAttempts":[{"provider":"anthropic","success":true}]}]},` +
	`"nativeMetadata":{"cost":"0.1"},"future":true}`

func wantGatewayMetadata() *GatewayMetadata {
	return &GatewayMetadata{
		Execution: &ExecutionOverview{RequestedModelID: "alias", CanonicalModelID: "public", Attempts: []ExecutionAttempt{
			{Provider: "openai", ModelID: "a1", ProviderInstance: "one", Outcome: AttemptFailed, Error: &AttemptFailure{Message: "busy", Type: "overloaded_error", Code: json.RawMessage(`900719925474099312345`), StatusCode: 503}},
			{Provider: "anthropic", ModelID: "b1", ProviderInstance: "two", Outcome: AttemptSelected},
		}},
		Routing: &Routing{
			OriginalModelID: "alias", CanonicalSlug: "public", ResolvedProvider: "openai", FinalProvider: "anthropic", ModelAttemptCount: 2, TotalProviderAttemptCount: 2,
			ModelAttempts: []RoutingModelAttempt{
				{ModelID: "alias", CanonicalSlug: "public", ProviderAttemptCount: 1, ProviderAttempts: []RoutingProviderAttempt{{Provider: "openai", StatusCode: 503}}},
				{ModelID: "backup", CanonicalSlug: "backup", Success: true, ProviderAttemptCount: 1, ProviderAttempts: []RoutingProviderAttempt{{Provider: "anthropic", Success: true}}},
			},
		},
	}
}

func TestGatewayMetadataFrom(t *testing.T) {
	t.Run("execution and routing", func(t *testing.T) {
		metadata := provider.ProviderMetadata{"gateway": json.RawMessage(gatewayMetadataFixture), "anthropic": json.RawMessage(`{}`)}
		before := string(metadata["gateway"])
		got, err := GatewayMetadataFrom(metadata)
		require.NoError(t, err)
		assert.Equal(t, wantGatewayMetadata(), got)
		assert.Equal(t, before, string(metadata["gateway"]))
	})
	t.Run("string code", func(t *testing.T) {
		got, err := GatewayMetadataFrom(provider.ProviderMetadata{"gateway": json.RawMessage(`{"execution":{"requestedModelId":"a","attempts":[{"provider":"p","modelId":"m","outcome":"failed","error":{"code":"busy"}}]}}`)})
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage(`"busy"`), got.Execution.Attempts[0].Error.Code)
		assert.Nil(t, got.Routing)
	})
	for name, metadata := range map[string]provider.ProviderMetadata{
		"nil metadata":      nil,
		"no gateway":        {"anthropic": json.RawMessage(`{}`)},
		"native gateway":    {"gateway": json.RawMessage(`{"cost":"0.1"}`)},
		"null members":      {"gateway": json.RawMessage(`{"execution":null,"routing":null}`)},
		"only native value": {"gateway": json.RawMessage(`{"nativeMetadata":{"execution":{}}}`)},
	} {
		t.Run("absent/"+name, func(t *testing.T) {
			got, err := GatewayMetadataFrom(metadata)
			require.NoError(t, err)
			assert.Nil(t, got)
		})
	}
	for name, raw := range map[string]string{
		"routing not an object":   `{"execution":{"requestedModelId":"a","attempts":[]},"routing":42}`,
		"malformed model attempt": `{"routing":{"originalModelId":"a","modelAttempts":"x"}}`,
		"execution not an object": `{"execution":"x"}`,
		"namespace not an object": `[]`,
	} {
		t.Run("malformed/"+name, func(t *testing.T) {
			got, err := GatewayMetadataFrom(provider.ProviderMetadata{"gateway": json.RawMessage(raw)})
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestGatewayMetadataFromError(t *testing.T) {
	t.Run("unary error document", func(t *testing.T) {
		p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"service overloaded","type":"internal_server_error","param":null,"code":"overloaded"},"providerMetadata":{"gateway":`+gatewayMetadataFixture+`}}`)
		})
		model, err := p.LanguageModel("alias")
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.Error(t, err)
		got, err := GatewayMetadataFromError(err)
		require.NoError(t, err)
		assert.Equal(t, wantGatewayMetadata(), got)
	})
	t.Run("stream finish and committed error", func(t *testing.T) {
		model := streamFromBody(t, io.NopCloser(strings.NewReader(
			sseFrame(`{"type":"stream-start","warnings":[]}`)+
				sseFrame(`{"type":"error","error":{"message":"service overloaded","type":"internal_server_error","param":null,"code":"overloaded","statusCode":503,"retryable":true,"data":{"providerMetadata":{"gateway":`+gatewayMetadataFixture+`},"nativeError":{"message":"busy"}}}}`)+
				sseFrame(`{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":{"gateway":`+gatewayMetadataFixture+`}}`)+
				"data: [DONE]\n\n")))
		result, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		var fromError, fromFinish *GatewayMetadata
		for _, part := range collectParts(t, result) {
			switch part.Type {
			case provider.PartError:
				fromError, err = GatewayMetadataFromError(part.APICallError)
				require.NoError(t, err)
			case provider.PartFinish:
				fromFinish, err = GatewayMetadataFrom(part.ProviderMetadata)
				require.NoError(t, err)
			}
		}
		assert.Equal(t, wantGatewayMetadata(), fromError)
		assert.Equal(t, wantGatewayMetadata(), fromFinish)
	})
	for name, err := range map[string]error{
		"nil":                nil,
		"not an API error":   errors.New("plain"),
		"no data":            provider.NewAPICallError(provider.APICallErrorOptions{Message: "x"}),
		"data without":       provider.NewAPICallError(provider.APICallErrorOptions{Message: "x", Data: json.RawMessage(`{"error":{"message":"x"}}`)}),
		"data not an object": provider.NewAPICallError(provider.APICallErrorOptions{Message: "x", Data: json.RawMessage(`"x"`)}),
	} {
		t.Run("absent/"+name, func(t *testing.T) {
			got, readErr := GatewayMetadataFromError(err)
			require.NoError(t, readErr)
			assert.Nil(t, got)
		})
	}
}
