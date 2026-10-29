package agentobservability

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordingMiddleware_ExplicitConversationInIsolatedContext(t *testing.T) {
	for _, tc := range []struct {
		name      string
		streaming bool
		fail      bool
	}{{name: "generate"}, {name: "stream", streaming: true}, {name: "generate_error", fail: true}, {name: "stream_error", streaming: true, fail: true}} {
		t.Run(tc.name, func(t *testing.T) {
			env := testkit.NewEnv(t)
			ctx := agento11y.WithConversationID(context.Background(), "untrusted-ambient")
			ctx = agento11y.WithAgentName(ctx, "untrusted-agent")
			model := &mockLanguageModel{provider_: "openai", modelID: "model",
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					if tc.fail {
						return nil, errors.New("provider unavailable")
					}
					return &provider.GenerateResult{}, nil
				},
				doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					if tc.fail {
						return nil, errors.New("provider unavailable")
					}
					ch := make(chan provider.StreamPart)
					close(ch)
					return &provider.StreamResult{Stream: ch}, nil
				},
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
				ContextSource:  ContextProvidedOnly,
				ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
				ContextProvider: func(context.Context) ContextInfo {
					return ContextInfo{ConversationID: "explicit-conversation", AgentName: "explicit-agent"}
				},
			})}})
			if tc.streaming {
				result, err := wrapped.DoStream(ctx, provider.CallOptions{})
				if tc.fail {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					for range result.Stream {
					}
				}
			} else {
				_, err := wrapped.DoGenerate(ctx, provider.CallOptions{})
				if tc.fail {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			env.Shutdown(t)
			g := env.SingleGenerationJSON(t)
			assert.Equal(t, "explicit-conversation", testkit.StringValue(t, g, "conversation_id"))
			assert.Equal(t, "explicit-agent", testkit.StringValue(t, g, "agent_name"))
			spans := env.Spans.Ended()
			require.Len(t, spans, 1)
			assert.Equal(t, "explicit-conversation", spanAttributes(spans[0])["gen_ai.conversation.id"])
		})
	}
}
