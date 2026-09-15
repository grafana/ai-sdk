package agentobservability

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordingMiddleware_MetadataOnlyRetainsNonContentFields(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
			var sanitizerCalls atomic.Int32
			env := testkit.NewEnv(t, func(config *agento11y.Config) {
				config.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				config.GenerationSanitizer = func(g agento11y.Generation) agento11y.Generation {
					sanitizerCalls.Add(1)
					return g
				}
			})
			ctx := agento11y.WithUserID(context.Background(), "ambient-user")
			ctx = agento11y.WithAgentName(ctx, "ambient-agent")
			ctx = agento11y.WithConversationID(ctx, "ambient-conversation")
			ctx = agento11y.WithTag(ctx, "ambient-tag", "ambient-tag-value")
			ctx = WithGenerationID(ctx, "ambient-generation")
			ctx = WithParentGenerationIDs(ctx, "ambient-parent")
			finish := provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "provider-raw-finish"}
			model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "private-output"}}, FinishReason: finish}, nil
				},
				doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					ch := make(chan provider.StreamPart, 2)
					ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "private-output"}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish}
					close(ch)
					return &provider.StreamResult{Stream: ch}, nil
				},
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
				IdentitySource:  IdentityRequested,
				ClientResolver:  func(context.Context) *agento11y.Client { return env.Client },
				ContextProvider: func(context.Context) ContextInfo { return ContextInfo{Metadata: map[string]any{"approved": "trusted"}} },
			})}})
			params := provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("private-prompt")},
				ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"budgetTokens":123},"unknown":"private-option"}`)}},
			}
			if streaming {
				result, err := wrapped.DoStream(ctx, params)
				require.NoError(t, err)
				for range result.Stream {
				}
				awaitRecordedGeneration(t, env)
			} else {
				_, err := wrapped.DoGenerate(ctx, params)
				require.NoError(t, err)
			}
			require.NoError(t, env.Client.Shutdown(context.Background()))
			gen := env.SingleGenerationJSON(t)
			encoded, err := json.Marshal(gen)
			require.NoError(t, err)
			for _, retained := range []string{"ambient-user", "ambient-agent", "ambient-conversation", "ambient-tag-value", "ambient-generation", "ambient-parent", "provider-raw-finish", MetadataThinkingBudgetTokens} {
				assert.Contains(t, string(encoded), retained)
			}
			for _, omitted := range []string{"private-prompt", "private-output", "private-option"} {
				assert.NotContains(t, string(encoded), omitted)
			}
			assert.Zero(t, sanitizerCalls.Load())
			spans := env.Spans.Ended()
			require.Len(t, spans, 1)
			attrs := spanAttributes(spans[0])
			assert.Contains(t, attrs, "agento11y.tag.ambient-tag")
		})
	}
}

func TestRecordingMiddleware_ProvidedOnlyIsolatesAmbientContextButPreservesProviderContext(t *testing.T) {
	type providerContextKey struct{}
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
			env := testkit.NewEnv(t, func(config *agento11y.Config) {
				config.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
			})
			ctx := agento11y.WithConversationID(context.Background(), "ambient-conversation")
			ctx = agento11y.WithConversationTitle(ctx, "ambient-title")
			ctx = agento11y.WithUserID(ctx, "ambient-user")
			ctx = agento11y.WithAgentName(ctx, "ambient-agent")
			ctx = agento11y.WithAgentVersion(ctx, "ambient-version")
			ctx = agento11y.WithTag(ctx, "ambient-tag", "ambient-tag-value")
			ctx = agento11y.WithExperimentRunID(ctx, "ambient-experiment")
			ctx = WithGenerationID(ctx, "ambient-generation")
			ctx = WithParentGenerationIDs(ctx, "ambient-parent")
			ctx = context.WithValue(ctx, providerContextKey{}, "provider-context-value")

			providerSawContext := false
			finish := provider.FinishReason{Unified: provider.FinishReasonStop}
			model := &mockLanguageModel{
				provider_: "grafana",
				modelID:   "grafana/assistant",
				doGenerate: func(callCtx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
					providerSawContext = callCtx.Value(providerContextKey{}) == "provider-context-value"
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "private-output"}}, FinishReason: finish}, nil
				},
				doStream: func(callCtx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					providerSawContext = callCtx.Value(providerContextKey{}) == "provider-context-value"
					parts := make(chan provider.StreamPart, 2)
					parts <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "private-output"}
					parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
				IdentitySource: IdentityRequested,
				ContextSource:  ContextProvidedOnly,
				ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
				ContextProvider: func(context.Context) ContextInfo {
					return ContextInfo{Metadata: map[string]any{"approved": "trusted"}, Tags: map[string]string{"approved-tag": "trusted-tag"}}
				},
			})}})
			params := provider.CallOptions{Prompt: []provider.Message{provider.UserText("private-prompt")}}
			if streaming {
				result, err := wrapped.DoStream(ctx, params)
				require.NoError(t, err)
				for range result.Stream {
				}
				awaitRecordedGeneration(t, env)
			} else {
				_, err := wrapped.DoGenerate(ctx, params)
				require.NoError(t, err)
			}
			assert.True(t, providerSawContext)
			require.NoError(t, env.Client.Shutdown(context.Background()))
			generation := env.SingleGenerationJSON(t)
			encoded, err := json.Marshal(generation)
			require.NoError(t, err)
			serialized := string(encoded)
			for _, expected := range []string{"trusted", "approved", "trusted-tag", "grafana/assistant"} {
				assert.Contains(t, serialized, expected)
			}
			for _, omitted := range []string{
				"ambient-conversation", "ambient-title", "ambient-user", "ambient-agent", "ambient-version",
				"ambient-tag", "ambient-tag-value", "ambient-experiment", "ambient-generation", "ambient-parent",
				"provider-context-value", "private-prompt", "private-output",
			} {
				assert.NotContains(t, serialized, omitted)
			}
			spans := env.Spans.Ended()
			require.Len(t, spans, 1)
			spanJSON, err := json.Marshal(spanAttributes(spans[0]))
			require.NoError(t, err)
			for _, omitted := range []string{"ambient-user", "ambient-agent", "ambient-tag", "ambient-experiment", "provider-context-value"} {
				assert.NotContains(t, string(spanJSON), omitted)
			}
		})
	}
}
