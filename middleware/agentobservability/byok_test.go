package agentobservability

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordingMiddleware_BYOKRequestMetadataIsNotExported(t *testing.T) {
	const gateway = `{"byok":{"openai":[{"apiKey":"dummy-credential","future":"dummy-unfamiliar"}]}}`
	body := json.RawMessage(`{"providerOptions":{"gateway":` + gateway + `}}`)
	for _, operation := range []string{"generate", "stream", "error"} {
		t.Run(operation, func(t *testing.T) {
			env := testkit.NewEnv(t)
			complete := make(chan struct{})
			model := &mockLanguageModel{provider_: "grafana", modelID: "openai/native"}
			model.doGenerate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				if operation == "error" {
					return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "safe public failure", StatusCode: 400, RequestBodyValues: body})
				}
				return &provider.GenerateResult{Request: &provider.RequestMetadata{Body: body}, Response: &provider.GenerateResponse{Body: body}, Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "gateway.byok application-output"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
			}
			model.doStream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				parts := make(chan provider.StreamPart, 4)
				parts <- provider.StreamPart{Type: provider.PartStreamStart}
				parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}
				parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "gateway.byok application-output"}
				parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
				close(parts)
				return &provider.StreamResult{Stream: parts, Request: &provider.RequestMetadata{Body: body}}, nil
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{ClientResolver: func(context.Context) *agento11y.Client { return env.Client }, OnRecordComplete: func() { close(complete) }})}})
			options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("gateway.byok application-input")}, ProviderOptions: provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(gateway)}}}
			if operation == "stream" {
				result, err := wrapped.DoStream(context.Background(), options)
				require.NoError(t, err)
				for range result.Stream {
				}
				assert.Equal(t, body, result.Request.Body)
			} else {
				result, err := wrapped.DoGenerate(context.Background(), options)
				if operation == "error" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, body, result.Request.Body)
				}
			}
			select {
			case <-complete:
			case <-time.After(time.Second):
				require.FailNow(t, "recording did not finish")
			}
			env.Shutdown(t)
			generation, err := json.Marshal(env.SingleGenerationJSON(t))
			require.NoError(t, err)
			assert.NotContains(t, string(generation), "dummy-credential")
			assert.NotContains(t, string(generation), "dummy-unfamiliar")
			if operation != "error" {
				assert.Contains(t, string(generation), "gateway.byok application-input")
				assert.Contains(t, string(generation), "gateway.byok application-output")
			}
			original, err := json.Marshal(options.ProviderOptions)
			require.NoError(t, err)
			assert.Contains(t, string(original), "dummy-unfamiliar")
		})
	}
}
