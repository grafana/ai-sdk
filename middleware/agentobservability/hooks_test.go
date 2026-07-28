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

func TestHooksMiddleware_TransformedInput_PreservesReasoningSignature(t *testing.T) {
	// Original assistant message carries a reasoning part with a signature.
	// The hook returns a transform that keeps the assistant text identical.
	// We expect the resulting prompt to keep the original assistant message
	// verbatim (signature preserved).
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
	originalAssistant := provider.NewAssistantMessage(
		originalReasoning,
		provider.TextPart("Here is your answer"),
	)
	originalPrompt := []provider.Message{
		provider.UserText("question"),
		originalAssistant,
	}

	// Hook response: same assistant text, modified user message.
	transformed := agento11y.HookInput{
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("modified question")}},
			{Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.TextPart("Here is your answer")}},
		},
	}

	newPrompt := applyTransformedInput(originalPrompt, transformed)
	require.Len(t, newPrompt, 2)

	assert.Equal(t, provider.RoleUser, newPrompt[0].Role)
	require.Len(t, newPrompt[0].Content, 1)
	assert.Equal(t, "modified question", newPrompt[0].Content[0].Text)

	// Assistant message must be the original verbatim (with reasoning + signature).
	assistant := newPrompt[1]
	assert.Equal(t, provider.RoleAssistant, assistant.Role)
	require.Len(t, assistant.Content, 2)
	assert.Equal(t, provider.ContentPartTypeReasoning, assistant.Content[0].Type)
	// Signature byte-equal preserved.
	raw, ok := assistant.Content[0].ProviderOptions["anthropic"].(provider.RawProviderOption)
	require.True(t, ok, "anthropic option preserved as RawProviderOption")
	assert.JSONEq(t, `{"signature":"sig-xyz"}`, string(raw.Raw))
}

func TestHooksMiddleware_TransformedInput_ModifiedTextRebuilds(t *testing.T) {
	// When the hook changes the assistant text, the matching algorithm
	// can't preserve the signature; the message is rebuilt from agento11y parts
	// (without the signature). This is the documented behavior — the wire
	// format can't round-trip signatures.
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

	newPrompt := applyTransformedInput(originalPrompt, transformed)
	require.Len(t, newPrompt, 1)
	require.Len(t, newPrompt[0].Content, 1, "rebuilt message has only the transformed text part")
	assert.Equal(t, "REDACTED answer", newPrompt[0].Content[0].Text)
	// No reasoning part survived because the rebuild drops thinking parts.
	for _, p := range newPrompt[0].Content {
		assert.NotEqual(t, provider.ContentPartTypeReasoning, p.Type)
	}
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
	newPrompt := applyTransformedInput(originalPrompt, transformed)
	require.Len(t, newPrompt, 1)
	assert.Equal(t, provider.RoleUser, newPrompt[0].Role)
	assert.Equal(t, "hello (filtered)", newPrompt[0].Content[0].Text)
}

// TestHooksMiddleware_TransformedInput_PreservesSystemMessages guards
// against a regression where a hook transform silently dropped the
// original system messages: messagesToAgento11y folds them into a separate
// HookInput.SystemPrompt field, so a hook response that only modifies user
// messages comes back with SystemPrompt == "". applyTransformedInput must
// treat the zero value as "didn't touch the system context" and keep the
// originals.
func TestHooksMiddleware_TransformedInput_PreservesSystemMessages(t *testing.T) {
	originalPrompt := []provider.Message{
		provider.NewSystemMessage("you are helpful"),
		provider.UserText("hello"),
	}
	transformed := agento11y.HookInput{
		// SystemPrompt deliberately empty — the hook only modified the user
		// message. Without the fix, the resulting prompt has no system.
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("hello (filtered)")}},
		},
	}

	newPrompt := applyTransformedInput(originalPrompt, transformed)
	require.Len(t, newPrompt, 2)

	assert.Equal(t, provider.RoleSystem, newPrompt[0].Role)
	require.Len(t, newPrompt[0].Content, 1)
	assert.Equal(t, "you are helpful", newPrompt[0].Content[0].Text)

	assert.Equal(t, provider.RoleUser, newPrompt[1].Role)
	assert.Equal(t, "hello (filtered)", newPrompt[1].Content[0].Text)
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

	newPrompt := applyTransformedInput(originalPrompt, transformed)
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

	newPrompt := applyTransformedInput(originalPrompt, transformed)
	require.Len(t, newPrompt, 2)

	assert.Equal(t, provider.RoleSystem, newPrompt[0].Role)
	assert.Equal(t, "you are helpful", newPrompt[0].Content[0].Text)
	assert.Equal(t, provider.RoleUser, newPrompt[1].Role)
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
