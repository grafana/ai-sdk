package agentobservability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hooksTestServer stands up an in-process HTTP server that plays the
// agento11y hooks evaluation endpoint. Returning a canned response lets us
// exercise the allow/deny/transform branches without a real Agent Observability deployment.
type hooksTestServer struct {
	srv      *httptest.Server
	hits     atomic.Int32
	response agento11y.HookEvaluateResponse
	// delay, when non-zero, sleeps before responding so MaxLatency tests can
	// observe a timeout cancellation.
	delay time.Duration
	// statusCode overrides the response code (defaults to 200).
	statusCode int
}

func newHooksTestServer(t *testing.T, resp agento11y.HookEvaluateResponse) *hooksTestServer {
	t.Helper()
	h := &hooksTestServer{response: resp, statusCode: http.StatusOK}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		if h.delay > 0 {
			select {
			case <-time.After(h.delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(h.statusCode)
		_ = json.NewEncoder(w).Encode(h.response)
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *hooksTestServer) clientWithHooksEnabled() *agento11y.Client {
	failOpen := false
	return agento11y.NewClient(agento11y.Config{
		API: agento11y.APIConfig{Endpoint: h.srv.URL},
		Hooks: agento11y.HooksConfig{
			Enabled:  true,
			Phases:   []agento11y.HookPhase{agento11y.HookPhasePreflight},
			Timeout:  30 * time.Second,
			FailOpen: &failOpen,
		},
	})
}

func TestHooksMiddleware_NilResolver_PassesThrough(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model:      model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{})},
	})
	result, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, model.generateHit)
}

func TestHooksMiddleware_EnabledFalse_PassesThrough(t *testing.T) {
	h := newHooksTestServer(t, agento11y.HookEvaluateResponse{Action: agento11y.HookActionDeny})
	defer h.srv.Close()
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
			Enabled:        func(ctx context.Context) bool { return false },
			ClientResolver: func(ctx context.Context) *agento11y.Client { return h.clientWithHooksEnabled() },
		})},
	})
	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(0), h.hits.Load(), "Enabled=false short-circuits before HTTP")
	assert.Equal(t, 1, model.generateHit, "inner model still called")
}

func TestHooksMiddleware_AllowPassesThrough(t *testing.T) {
	h := newHooksTestServer(t, agento11y.HookEvaluateResponse{Action: agento11y.HookActionAllow})
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	client := h.clientWithHooksEnabled()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
			ClientResolver: func(ctx context.Context) *agento11y.Client { return client },
		})},
	})
	result, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), h.hits.Load(), "hook server contacted exactly once")
	assert.Equal(t, 1, model.generateHit)
}

func TestHooksMiddleware_TransformFailureDoesNotInvokeModel(t *testing.T) {
	tests := []struct {
		name        string
		prompt      []provider.Message
		transformed agento11y.HookInput
	}{
		{
			name:        "empty transform",
			prompt:      []provider.Message{provider.UserText("secret")},
			transformed: agento11y.HookInput{},
		},
		{
			name: "multimodal input",
			prompt: []provider.Message{provider.NewUserMessage(
				provider.TextPart("describe this"),
				provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/image.png"}),
			)},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role:  agento11y.RoleUser,
				Parts: []agento11y.Part{agento11y.TextPart("redacted")},
			}}},
		},
		{
			name: "message provider options",
			prompt: []provider.Message{{
				Role:    provider.RoleUser,
				Content: []provider.ContentPart{provider.TextPart("input")},
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`)},
				},
			}},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("filtered")},
			}}},
		},
		{
			name: "text provider options",
			prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{
				Type: provider.ContentPartTypeText,
				Text: "input",
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`)},
				},
			})},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("filtered")},
			}}},
		},
		{
			name: "empty reasoning metadata",
			prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{
				Type: provider.ContentPartTypeReasoning,
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"redactedData":"secret"}`)},
				},
			})},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.TextPart("filtered")},
			}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHooksTestServer(t, agento11y.HookEvaluateResponse{
				Action:           agento11y.HookActionAllow,
				TransformedInput: &tc.transformed,
			})
			model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
			client := h.clientWithHooksEnabled()
			wrapped := middleware.Wrap(middleware.WrapOptions{
				Model: model,
				Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
					ClientResolver: func(context.Context) *agento11y.Client { return client },
				})},
			})

			_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{Prompt: tc.prompt})
			require.ErrorIs(t, err, ErrHookTransformFailed)
			assert.Equal(t, 0, model.generateHit)
		})
	}
}

func TestHooksMiddleware_TransformFailureBlocksStream(t *testing.T) {
	transformed := agento11y.HookInput{}
	h := newHooksTestServer(t, agento11y.HookEvaluateResponse{
		Action:           agento11y.HookActionAllow,
		TransformedInput: &transformed,
	})
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	client := h.clientWithHooksEnabled()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return client },
		})},
	})

	_, err := wrapped.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("secret")}})
	require.ErrorIs(t, err, ErrHookTransformFailed)
	assert.Equal(t, 0, model.streamHit)
}

func TestHooksMiddleware_TransformAppliesToStream(t *testing.T) {
	originalTools := []provider.Tool{
		{Type: provider.ToolTypeFunction, Name: "keep", Description: "kept", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Type: provider.ToolTypeFunction, Name: "remove", Description: "removed", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	transformed := agento11y.HookInput{
		Messages: []agento11y.Message{{
			Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("filtered")},
		}},
		Tools: toolsToAgento11y(originalTools[:1]),
	}
	h := newHooksTestServer(t, agento11y.HookEvaluateResponse{
		Action: agento11y.HookActionAllow, TransformedInput: &transformed,
	})
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	client := h.clientWithHooksEnabled()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return client },
		})},
	})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("secret")}, Tools: originalTools,
	})
	require.NoError(t, err)
	for range result.Stream {
	}
	require.Equal(t, 1, model.streamHit)
	require.Equal(t, []provider.Message{provider.UserText("filtered")}, model.lastParams.Prompt)
	require.Equal(t, originalTools[:1], model.lastParams.Tools)
}

func TestBuildHookEvaluateRequest_ExcludesMedia(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	request := buildHookEvaluateRequest(model, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.TextPart("describe this"),
				provider.FilePart("image/png", provider.DataContent{URL: "https://cdn.example.com/image.png?token=secret"}),
			),
			provider.NewAssistantMessage(
				provider.ReasoningFilePart("image/png", provider.DataContent{Base64: "AQID"}),
			),
		},
	}, ContextInfo{})

	require.Len(t, request.Input.Messages, 1)
	require.Len(t, request.Input.Messages[0].Parts, 1)
	assert.Equal(t, agento11y.PartKindText, request.Input.Messages[0].Parts[0].Kind)
	assert.Equal(t, "describe this", request.Input.ConversationPreview)
}

func TestHooksMiddleware_DenyReturnsTypedError(t *testing.T) {
	h := newHooksTestServer(t, agento11y.HookEvaluateResponse{
		Action: agento11y.HookActionDeny,
		RuleID: "rule-42",
		Reason: "policy violation",
	})
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	client := h.clientWithHooksEnabled()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
			ClientResolver: func(ctx context.Context) *agento11y.Client { return client },
		})},
	})
	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.Error(t, err)

	var denial *HookDenialError
	require.True(t, errors.As(err, &denial))
	assert.Equal(t, "rule-42", denial.RuleID)
	assert.Equal(t, "policy violation", denial.Reason)
	assert.True(t, errors.Is(err, ErrHookDenied))

	assert.Equal(t, 0, model.generateHit, "inner model NOT invoked on deny")
}

func TestHooksMiddleware_MaxLatencyTimeout(t *testing.T) {
	h := newHooksTestServer(t, agento11y.HookEvaluateResponse{Action: agento11y.HookActionAllow})
	h.delay = 200 * time.Millisecond
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	client := h.clientWithHooksEnabled()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
			ClientResolver: func(ctx context.Context) *agento11y.Client { return client },
			MaxLatency:     50 * time.Millisecond,
		})},
	})
	// The request context is fresh; MaxLatency cancels only the hook RPC.
	ctx := context.Background()
	_, err := wrapped.DoGenerate(ctx, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.Error(t, err, "hook timeout surfaces because FailOpen=false")
	assert.Contains(t, strings.ToLower(err.Error()), "context deadline", "deadline-related error reaches caller")

	// Crucially the request context is NOT cancelled — we can still do other work.
	require.NoError(t, ctx.Err())
}

func TestHooksMiddleware_TransformedInput_PreservesReasoningSignatureWithoutRestoringRemovedParts(t *testing.T) {
	originalReasoning := provider.ContentPart{
		Type: provider.ContentPartTypeReasoning,
		Text: "thinking…",
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"signature":"sig-xyz"}`),
			},
		},
	}
	originalPrompt := []provider.Message{
		provider.UserText("question"),
		provider.NewAssistantMessage(
			originalReasoning,
			provider.ToolCallPart("call-1", "lookup", json.RawMessage(`{"query":"secret"}`)),
			provider.TextPart("Here is your answer"),
		),
	}
	transformed := agento11y.HookInput{
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("modified question")}},
			{Role: agento11y.RoleAssistant, Parts: []agento11y.Part{
				agento11y.ThinkingPart("thinking…"),
				agento11y.TextPart("Here is your answer"),
			}},
		},
	}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 2)
	assert.Equal(t, "modified question", newPrompt[0].Content[0].Text)

	assistant := newPrompt[1]
	require.Len(t, assistant.Content, 2, "removed tool call must not be restored")
	assert.Equal(t, provider.ContentPartTypeReasoning, assistant.Content[0].Type)
	raw, ok := assistant.Content[0].ProviderOptions["anthropic"].(provider.RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, `{"signature":"sig-xyz"}`, string(raw.Raw))
	assert.Equal(t, "Here is your answer", assistant.Content[1].Text)
}

func TestHooksMiddleware_TransformedInput_DoesNotRestoreOmittedReasoning(t *testing.T) {
	originalPrompt := []provider.Message{provider.NewAssistantMessage(
		provider.ContentPart{
			Type: provider.ContentPartTypeReasoning,
			Text: "thinking",
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"signature":"sig"}`)},
			},
		},
		provider.TextPart("answer"),
	)}
	transformed := agento11y.HookInput{Messages: []agento11y.Message{{
		Role:  agento11y.RoleAssistant,
		Parts: []agento11y.Part{agento11y.TextPart("answer")},
	}}}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 1)
	require.Len(t, newPrompt[0].Content, 1)
	assert.Equal(t, provider.ContentPartTypeText, newPrompt[0].Content[0].Type)
}

func TestHooksMiddleware_TransformedInput_PreservesProviderToolFields(t *testing.T) {
	originalToolCall := provider.ToolCallPart("call-1", "remote_lookup", json.RawMessage(`{"query":"x"}`))
	originalToolCall.ProviderOptions = provider.ProviderOptions{
		"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"type":"mcp-tool-use"}`)},
	}
	originalPrompt := []provider.Message{provider.NewAssistantMessage(originalToolCall)}
	transformedCall := agento11y.ToolCallPart(agento11y.ToolCall{
		ID: "call-1", Name: "remote_lookup", InputJSON: json.RawMessage(`{"query":"x"}`),
	})
	transformedCall.Metadata.ProviderType = "mcp_tool_use"
	transformed := agento11y.HookInput{Messages: []agento11y.Message{{
		Role:  agento11y.RoleAssistant,
		Parts: []agento11y.Part{transformedCall},
	}}}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 1)
	require.Len(t, newPrompt[0].Content, 1)
	assert.False(t, newPrompt[0].Content[0].ProviderExecuted)
	assert.Equal(t, originalToolCall.ProviderOptions, newPrompt[0].Content[0].ProviderOptions)
}

func TestHooksMiddleware_TransformedInput_MatchesSignedReasoningAfterRemovedAssistant(t *testing.T) {
	reasoning := func(text, signature string) provider.ContentPart {
		return provider.ContentPart{
			Type: provider.ContentPartTypeReasoning,
			Text: text,
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"signature":"` + signature + `"}`)},
			},
		}
	}
	originalPrompt := []provider.Message{
		provider.NewAssistantMessage(reasoning("first thought", "sig-1"), provider.TextPart("first")),
		provider.NewAssistantMessage(reasoning("second thought", "sig-2"), provider.TextPart("second")),
	}
	transformed := agento11y.HookInput{Messages: []agento11y.Message{{
		Role: agento11y.RoleAssistant,
		Parts: []agento11y.Part{
			agento11y.ThinkingPart("second thought"),
			agento11y.TextPart("second"),
		},
	}}}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	raw := newPrompt[0].Content[0].ProviderOptions["anthropic"].(provider.RawProviderOption)
	assert.JSONEq(t, `{"signature":"sig-2"}`, string(raw.Raw))
}

func TestApplyTransformedInput_RejectsMalformedContent(t *testing.T) {
	signedReasoning := provider.ContentPart{
		Type: provider.ContentPartTypeReasoning,
		Text: "original",
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"signature":"sig"}`)},
		},
	}
	tests := []struct {
		name        string
		original    []provider.Message
		transformed agento11y.HookInput
	}{
		{
			name:     "unknown role",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: "unknown", Parts: []agento11y.Part{agento11y.TextPart("input")},
			}}},
		},
		{
			name:     "message name",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleUser, Name: "named", Parts: []agento11y.Part{agento11y.TextPart("input")},
			}}},
		},
		{
			name:     "text provider metadata",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleUser,
				Parts: []agento11y.Part{{
					Kind: agento11y.PartKindText, Text: "input",
					Metadata: agento11y.PartMetadata{ProviderType: "server_tool_use"},
				}},
			}}},
		},
		{
			name: "original reasoning under user role",
			original: []provider.Message{{
				Role: provider.RoleUser,
				Content: []provider.ContentPart{{
					Type: provider.ContentPartTypeReasoning, Text: "thinking",
				}},
			}},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.ThinkingPart("thinking")},
			}}},
		},
		{
			name:     "missing tool payload",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant, Parts: []agento11y.Part{{Kind: agento11y.PartKindToolCall}},
			}}},
		},
		{
			name:     "empty tool name",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role:  agento11y.RoleAssistant,
				Parts: []agento11y.Part{agento11y.ToolCallPart(agento11y.ToolCall{ID: "call-1"})},
			}}},
		},
		{
			name:     "empty tool call id",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role:  agento11y.RoleAssistant,
				Parts: []agento11y.Part{agento11y.ToolCallPart(agento11y.ToolCall{Name: "lookup"})},
			}}},
		},
		{
			name:     "whitespace tool call id",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant,
				Parts: []agento11y.Part{agento11y.ToolCallPart(agento11y.ToolCall{
					ID: " ", Name: "lookup",
				})},
			}}},
		},
		{
			name:     "tool call in user message",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role:  agento11y.RoleUser,
				Parts: []agento11y.Part{agento11y.ToolCallPart(agento11y.ToolCall{Name: "lookup"})},
			}}},
		},
		{
			name:     "multiple payload fields",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant,
				Parts: []agento11y.Part{{
					Kind:     agento11y.PartKindToolCall,
					Text:     "extra",
					ToolCall: &agento11y.ToolCall{Name: "lookup"},
				}},
			}}},
		},
		{
			name:     "invalid tool JSON",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant,
				Parts: []agento11y.Part{agento11y.ToolCallPart(agento11y.ToolCall{
					ID: "call-1", Name: "lookup", InputJSON: json.RawMessage(`{`),
				})},
			}}},
		},
		{
			name:     "duplicate-key tool JSON",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant,
				Parts: []agento11y.Part{agento11y.ToolCallPart(agento11y.ToolCall{
					ID: "call-1", Name: "lookup", InputJSON: json.RawMessage(`{"value":1,"\u0076alue":2}`),
				})},
			}}},
		},
		{
			name:     "invalid tool result JSON",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					ToolCallID: "call-1", Name: "lookup", ContentJSON: json.RawMessage(`{`),
				})},
			}}},
		},
		{
			name:     "duplicate-key tool result JSON",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					ToolCallID: "call-1", Name: "lookup", ContentJSON: json.RawMessage(`{"value":1,"value":2}`),
				})},
			}}},
		},
		{
			name:     "tool result without correlation",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role:  agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{})},
			}}},
		},
		{
			name:     "tool result without name",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					ToolCallID: "call-1",
				})},
			}}},
		},
		{
			name:     "tool result without id",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					Name: "lookup",
				})},
			}}},
		},
		{
			name:     "whitespace tool result name",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					ToolCallID: "call-1", Name: " ",
				})},
			}}},
		},
		{
			name:     "tool result with both payloads",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					ToolCallID: "call-1", Name: "lookup", Content: "text", ContentJSON: json.RawMessage(`{"value":1}`),
				})},
			}}},
		},
		{
			name:     "tool result without payload",
			original: []provider.Message{provider.UserText("input")},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					ToolCallID: "call-1", Name: "lookup",
				})},
			}}},
		},
		{
			name: "ambiguous signed and unsigned reasoning",
			original: []provider.Message{provider.NewAssistantMessage(
				signedReasoning,
				provider.ReasoningPart("original"),
			)},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.ThinkingPart("original")},
			}}},
		},
		{
			name: "reasoning signature with extra metadata",
			original: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{
				Type: provider.ContentPartTypeReasoning,
				Text: "original",
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{
						Key: "anthropic", Raw: json.RawMessage(`{"signature":"sig","extra":true}`),
					},
				},
			})},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.ThinkingPart("original")},
			}}},
		},
		{
			name:     "changed signed reasoning",
			original: []provider.Message{provider.NewAssistantMessage(signedReasoning)},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.ThinkingPart("changed")},
			}}},
		},
		{
			name: "changed provider tool result",
			original: []provider.Message{provider.NewToolMessage(provider.ContentPart{
				Type: provider.ContentPartTypeToolResult, ToolCallID: "call-1", ToolName: "server_tool",
				ProviderExecuted: true,
				Output:           &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"value":"original"}`)},
			})},
			transformed: agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleTool,
				Parts: []agento11y.Part{agento11y.ToolResultPart(agento11y.ToolResult{
					ToolCallID: "call-1", Name: "server_tool", ContentJSON: json.RawMessage(`{"value":"changed"}`),
				})},
			}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := applyTransformedInput(tc.original, tc.transformed)
			require.ErrorIs(t, err, ErrHookTransformFailed)
		})
	}
}

func TestApplyTransformedInput_DistinguishesLargeJSONIntegers(t *testing.T) {
	unchangedCall := provider.ToolCallPart("call-1", "lookup", json.RawMessage(`{"value":9007199254740993}`))
	unchangedTools := []provider.Tool{{
		Type: provider.ToolTypeFunction, Name: "lookup",
		InputSchema: json.RawMessage(`{"const":9007199254740993}`),
	}}
	_, mappedMessages := messagesToAgento11yWithMediaAndTools(
		[]provider.Message{provider.NewAssistantMessage(unchangedCall)}, true, unchangedTools,
	)
	mappedTools := toolsToAgento11y(unchangedTools)
	require.Equal(t, `{"value":9007199254740993}`, string(mappedMessages[0].Parts[0].ToolCall.InputJSON))
	require.Equal(t, `{"const":9007199254740993}`, string(mappedTools[0].InputSchema))
	unchanged, err := applyTransformedInputWithTools(
		[]provider.Message{provider.NewAssistantMessage(unchangedCall)}, unchangedTools,
		agento11y.HookInput{Messages: mappedMessages, Tools: mappedTools},
	)
	require.NoError(t, err)
	require.Equal(t, `{"value":9007199254740993}`, string(unchanged[0].Content[0].Input))

	originalCall := provider.ToolCallPart("call-1", "server_tool", json.RawMessage(`{"value":9007199254740992}`))
	originalCall.ProviderExecuted = true
	transformedCall := agento11y.ToolCallPart(agento11y.ToolCall{
		ID: "call-1", Name: "server_tool", InputJSON: json.RawMessage(`{"value":9007199254740993}`),
	})
	transformedCall.Metadata.ProviderType = "server_tool_use"

	_, err = applyTransformedInput(
		[]provider.Message{provider.NewAssistantMessage(originalCall)},
		agento11y.HookInput{Messages: []agento11y.Message{{
			Role: agento11y.RoleAssistant, Parts: []agento11y.Part{transformedCall},
		}}},
	)
	require.ErrorIs(t, err, ErrHookTransformFailed)

	duplicateInput := json.RawMessage(`{"value":1,"value":2}`)
	require.Equal(t, string(duplicateInput), string(normalizeJSONObject(duplicateInput)))
	duplicateCall := provider.ToolCallPart("call-2", "server_tool", duplicateInput)
	duplicateCall.ProviderExecuted = true
	collapsedCall := agento11y.ToolCallPart(agento11y.ToolCall{
		ID: "call-2", Name: "server_tool", InputJSON: json.RawMessage(`{"value":2}`),
	})
	collapsedCall.Metadata.ProviderType = "server_tool_use"
	_, err = applyTransformedInput(
		[]provider.Message{provider.NewAssistantMessage(duplicateCall)},
		agento11y.HookInput{Messages: []agento11y.Message{{
			Role: agento11y.RoleAssistant, Parts: []agento11y.Part{collapsedCall},
		}}},
	)
	require.ErrorIs(t, err, ErrHookTransformFailed)

	duplicateSchemaTools := []provider.Tool{{
		Type: provider.ToolTypeFunction, Name: "lookup",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	duplicateSchema := toolsToAgento11y(duplicateSchemaTools)[0]
	duplicateSchema.InputSchema = json.RawMessage(`{"type":"object","type":"array"}`)
	_, err = applyTransformedTools(duplicateSchemaTools, []agento11y.ToolDefinition{duplicateSchema})
	require.ErrorIs(t, err, ErrHookTransformFailed)

	originalTools := []provider.Tool{{
		Type: provider.ToolTypeFunction, Name: "lookup",
		InputSchema: json.RawMessage(`{"const":9007199254740992}`),
	}}
	transformedTool := toolsToAgento11y(originalTools)[0]
	transformedTool.InputSchema = json.RawMessage(`{"const":9007199254740993}`)
	_, err = applyTransformedTools(originalTools, []agento11y.ToolDefinition{transformedTool})
	require.ErrorIs(t, err, ErrHookTransformFailed)
}

func TestApplyTransformedInput_RequiresExactProviderDiscriminator(t *testing.T) {
	originalCall := provider.ToolCallPart("call-1", "remote", json.RawMessage(`{"query":"x"}`))
	originalCall.ProviderExecuted = true
	originalCall.ProviderOptions = provider.ProviderOptions{
		"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"type":"mcp-tool-use"}`)},
	}
	transformedCall := agento11y.ToolCallPart(agento11y.ToolCall{
		ID: "call-1", Name: "remote", InputJSON: json.RawMessage(`{"query":"x"}`),
	})
	transformedCall.Metadata.ProviderType = "server_tool_use"

	_, err := applyTransformedInput(
		[]provider.Message{provider.NewAssistantMessage(originalCall)},
		agento11y.HookInput{Messages: []agento11y.Message{{
			Role: agento11y.RoleAssistant, Parts: []agento11y.Part{transformedCall},
		}}},
	)
	require.ErrorIs(t, err, ErrHookTransformFailed)
}

func TestApplyTransformedInput_PreservesAliasedProviderResult(t *testing.T) {
	tools := []provider.Tool{{
		Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20250910", Name: "fetch_page",
	}}
	original := provider.ContentPart{
		Type: provider.ContentPartTypeToolResult, ToolCallID: "call-1", ToolName: "fetch_page",
		Output: &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"url":"https://example.com"}`)},
	}
	_, messages := messagesToAgento11yWithMediaAndTools(
		[]provider.Message{provider.NewToolMessage(original)}, true, tools,
	)
	require.Equal(t, "web_fetch_tool_result", messages[0].Parts[0].Metadata.ProviderType)

	transformed, err := applyTransformedInputWithTools(
		[]provider.Message{provider.NewToolMessage(original)}, tools,
		agento11y.HookInput{Messages: messages, Tools: toolsToAgento11y(tools)},
	)
	require.NoError(t, err)
	require.Len(t, transformed, 1)
	assert.Equal(t, original, transformed[0].Content[0])

	messages[0].Parts[0].Metadata.ProviderType = "tool_result"
	_, err = applyTransformedInputWithTools(
		[]provider.Message{provider.NewToolMessage(original)}, tools,
		agento11y.HookInput{Messages: messages, Tools: toolsToAgento11y(tools)},
	)
	require.ErrorIs(t, err, ErrHookTransformFailed)
}

func TestApplyTransformedInput_RejectsNewProviderSpecificPart(t *testing.T) {
	call := agento11y.ToolCallPart(agento11y.ToolCall{ID: "call-1", Name: "server_tool"})
	call.Metadata.ProviderType = "server_tool_use"
	_, err := applyTransformedInput(
		[]provider.Message{provider.UserText("input")},
		agento11y.HookInput{Messages: []agento11y.Message{{
			Role: agento11y.RoleAssistant, Parts: []agento11y.Part{call},
		}}},
	)
	require.ErrorIs(t, err, ErrHookTransformFailed)
}

func TestHooksMiddleware_TransformedInput_ModifiedTextRebuilds(t *testing.T) {
	originalAssistant := provider.NewAssistantMessage(
		provider.ContentPart{
			Type: provider.ContentPartTypeReasoning,
			Text: "thinking",
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(`{"signature":"sig-old"}`),
				},
			},
		},
		provider.TextPart("original answer"),
	)
	originalPrompt := []provider.Message{originalAssistant}

	transformed := agento11y.HookInput{
		Messages: []agento11y.Message{
			{Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.TextPart("REDACTED answer")}},
		},
	}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 1)
	require.Len(t, newPrompt[0].Content, 1, "rebuilt message has only the transformed text part")
	assert.Equal(t, provider.ContentPartTypeText, newPrompt[0].Content[0].Type)
	assert.Equal(t, "REDACTED answer", newPrompt[0].Content[0].Text)
}

func TestHooksMiddleware_TransformedInput_NonAssistantRebuiltFromParts(t *testing.T) {
	originalPrompt := []provider.Message{
		provider.UserText("hello"),
	}
	transformed := agento11y.HookInput{
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("hello (filtered)")}},
		},
	}
	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 1)
	assert.Equal(t, provider.RoleUser, newPrompt[0].Role)
	assert.Equal(t, "hello (filtered)", newPrompt[0].Content[0].Text)
}

func TestHooksMiddleware_TransformedInput_DropsOmittedSystemMessages(t *testing.T) {
	originalPrompt := []provider.Message{
		provider.NewSystemMessage("you are helpful"),
		provider.UserText("hello"),
	}
	transformed := agento11y.HookInput{
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("hello (filtered)")}},
		},
	}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 1)
	assert.Equal(t, provider.RoleUser, newPrompt[0].Role)
	assert.Equal(t, "hello (filtered)", newPrompt[0].Content[0].Text)
}

func TestHooksMiddleware_TransformedInput_SystemOnlyReplacement(t *testing.T) {
	newPrompt, err := applyTransformedInput(
		[]provider.Message{provider.UserText("remove me")},
		agento11y.HookInput{SystemPrompt: "system only"},
	)
	require.NoError(t, err)
	require.Len(t, newPrompt, 1)
	assert.Equal(t, provider.RoleSystem, newPrompt[0].Role)
	assert.Equal(t, "system only", newPrompt[0].Content[0].Text)
}

func TestHooksMiddleware_TransformedInput_OverridesSystemPrompt(t *testing.T) {
	// A hook that explicitly sets SystemPrompt should replace the originals.
	originalPrompt := []provider.Message{
		provider.NewSystemMessage("you are helpful"),
		provider.NewSystemMessage("also be concise"),
		provider.UserText("hello"),
	}
	transformed := agento11y.HookInput{
		SystemPrompt: "you are an internal-only assistant",
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("hello")}},
		},
	}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 2, "two originals collapsed into one transformed system")

	assert.Equal(t, provider.RoleSystem, newPrompt[0].Role)
	require.Len(t, newPrompt[0].Content, 1)
	assert.Equal(t, "you are an internal-only assistant", newPrompt[0].Content[0].Text)

	assert.Equal(t, provider.RoleUser, newPrompt[1].Role)
}

func TestHooksMiddleware_TransformedInput_AddsSystemWhenAbsent(t *testing.T) {
	// Original prompt has no system message; hook injects one.
	originalPrompt := []provider.Message{
		provider.UserText("hello"),
	}
	transformed := agento11y.HookInput{
		SystemPrompt: "you are helpful",
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("hello")}},
		},
	}

	newPrompt, err := applyTransformedInput(originalPrompt, transformed)
	require.NoError(t, err)
	require.Len(t, newPrompt, 2)

	assert.Equal(t, provider.RoleSystem, newPrompt[0].Role)
	assert.Equal(t, "you are helpful", newPrompt[0].Content[0].Text)
	assert.Equal(t, provider.RoleUser, newPrompt[1].Role)
}

func TestHooksMiddleware_TransformedInput_RemovesTools(t *testing.T) {
	originalTool := provider.Tool{
		Type:        provider.ToolTypeFunction,
		Name:        "lookup",
		Description: "look up data",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
	transformed := agento11y.HookInput{Messages: []agento11y.Message{{
		Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("filtered")},
	}}}
	h := newHooksTestServer(t, agento11y.HookEvaluateResponse{
		Action:           agento11y.HookActionAllow,
		TransformedInput: &transformed,
	})
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	client := h.clientWithHooksEnabled()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return client },
		})},
	})

	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("input")},
		Tools:  []provider.Tool{originalTool},
	})
	require.NoError(t, err)
	assert.Empty(t, model.lastParams.Tools)
}

func TestHooksMiddleware_TransformedInput_RejectsUnsatisfiedToolChoice(t *testing.T) {
	tests := []struct {
		name       string
		toolChoice *provider.ToolChoice
	}{
		{
			name:       "required",
			toolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired},
		},
		{
			name:       "selected tool",
			toolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transformed := agento11y.HookInput{Messages: []agento11y.Message{{
				Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("filtered")},
			}}}
			h := newHooksTestServer(t, agento11y.HookEvaluateResponse{
				Action: agento11y.HookActionAllow, TransformedInput: &transformed,
			})
			model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
			wrapped := middleware.Wrap(middleware.WrapOptions{
				Model: model,
				Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
					ClientResolver: func(context.Context) *agento11y.Client { return h.clientWithHooksEnabled() },
				})},
			})

			_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
				Prompt:     []provider.Message{provider.UserText("input")},
				Tools:      []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup"}},
				ToolChoice: tc.toolChoice,
			})
			require.ErrorIs(t, err, ErrHookTransformFailed)
			assert.Zero(t, model.generateHit)
		})
	}
}

func TestApplyTransformedTools_PreservesRetainedToolsAndRejectsModifiedTools(t *testing.T) {
	original := []provider.Tool{
		{Type: provider.ToolTypeFunction, Name: "first", Description: "first tool"},
		{Type: provider.ToolTypeFunction, Name: "second", Description: "second tool", Strict: boolPtr(true)},
	}
	mapped := toolsToAgento11y(original)

	transformed, err := applyTransformedTools(original, []agento11y.ToolDefinition{mapped[1]})
	require.NoError(t, err)
	require.Len(t, transformed, 1)
	assert.Equal(t, original[1], transformed[0])

	modified := mapped[0]
	modified.Description = "changed"
	_, err = applyTransformedTools(original, []agento11y.ToolDefinition{modified})
	require.ErrorIs(t, err, ErrHookTransformFailed)
}

func TestBuildHookEvaluateRequest(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude-3-5-sonnet"}
	params := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("be helpful"),
			provider.UserText("hi"),
		},
		Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "get_time"}},
	}
	ctxInfo := ContextInfo{AgentName: "lodestone", AgentVersion: "v1", Tags: map[string]string{"env": "prod"}}

	req := buildHookEvaluateRequest(model, params, ctxInfo)
	assert.Equal(t, agento11y.HookPhasePreflight, req.Phase)
	assert.Equal(t, "lodestone", req.Context.AgentName)
	assert.Equal(t, "v1", req.Context.AgentVersion)
	require.NotNil(t, req.Context.Model)
	assert.Equal(t, "anthropic", req.Context.Model.Provider)
	assert.Equal(t, "claude-3-5-sonnet", req.Context.Model.Name)
	assert.Equal(t, map[string]string{"env": "prod"}, req.Context.Tags)
	assert.Equal(t, "be helpful", req.Input.SystemPrompt)
	require.Len(t, req.Input.Messages, 1)
	assert.Equal(t, agento11y.RoleUser, req.Input.Messages[0].Role)
	require.Len(t, req.Input.Tools, 1)
	assert.Equal(t, "get_time", req.Input.Tools[0].Name)
	assert.Contains(t, req.Input.ConversationPreview, "hi")
}
