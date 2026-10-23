package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeApproval_Request(t *testing.T) {
	for _, decision := range []string{`true`, `false`, `null`, `"false"`, ``} {
		t.Run(decision, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			field := ""
			if decision != "" {
				field = `,"approved":` + decision
			}
			response := h.serve(validRequest(`{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"native-id"` + field + `,"reason":"","providerOptions":{"openai":{"extension":false}}}]}]}`))
			if decision != "true" && decision != "false" {
				require.Equal(t, http.StatusBadRequest, response.Code)
				assert.Zero(t, h.model.callCount())
				return
			}
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			part := h.model.receivedOptions().Prompt[0].Content[0]
			require.NotNil(t, part.Approved)
			assert.Equal(t, decision == "true", *part.Approved)
			assert.Empty(t, part.Reason)
			raw, err := json.Marshal(part.ProviderOptions)
			require.NoError(t, err)
			assert.JSONEq(t, `{"openai":{"extension":false}}`, string(raw))
		})
	}
	for _, role := range []string{"assistant", "user", "system"} {
		h := newRuntimeHarness(t, testLimits())
		response := h.serve(validRequest(`{"prompt":[{"role":"` + role + `","content":[{"type":"tool-approval-response","approvalId":"a","approved":false}]}]}`))
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Zero(t, h.model.callCount())
	}
}

func TestRuntimeApproval_UnaryCorrelation(t *testing.T) {
	call := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "mcp", Input: json.RawMessage(`{}`), ProviderExecuted: true}
	approval := provider.GenerateContentPart{Type: provider.ContentToolApprovalRequest, ToolCallID: "call", ApprovalID: "native", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"native","extension":false}`)}}
	local := call
	local.ProviderExecuted = false
	completed := provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "mcp", Result: json.RawMessage(`{}`)}
	for _, tc := range []struct {
		name  string
		parts []provider.GenerateContentPart
		good  bool
	}{
		{"valid", []provider.GenerateContentPart{call, approval}, true},
		{"unknown", []provider.GenerateContentPart{approval}, false},
		{"local", []provider.GenerateContentPart{local, approval}, false},
		{"completed", []provider.GenerateContentPart{call, completed, approval}, false},
		{"duplicate", []provider.GenerateContentPart{call, approval, approval}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			h.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				r := validGenerateResult()
				r.Content = tc.parts
				return r, nil
			}
			response := h.serve(validRequest(`{"prompt":[]}`))
			if tc.good {
				compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
				require.NoError(t, err)
				require.NoError(t, compiled.Validate(json.RawMessage(response.Body.Bytes())))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), `"approvalId":"native"`)
				assert.Contains(t, response.Body.String(), `"extension":false`)
			} else {
				require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
			}
		})
	}
}

func TestRuntimeApproval_DenialAndNestedOptions(t *testing.T) {
	for _, output := range []string{
		`{"type":"execution-denied"}`, `{"type":"execution-denied","reason":""}`, `{"type":"execution-denied","reason":"no","providerOptions":{"openai":{"approvalId":"native"}}}`,
		`{"type":"text","value":"","providerOptions":{"anthropic":{"cacheControl":{"type":"ephemeral"}}}}`,
		`{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"anthropic":{"cacheControl":{"type":"ephemeral"}}}}]}`,
	} {
		t.Run(output, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			response := h.serve(validRequest(`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"mcp","output":` + output + `}]}]}`))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			got, err := json.Marshal(h.model.receivedOptions().Prompt[0].Content[0].Output)
			require.NoError(t, err)
			assert.JSONEq(t, output, string(got))
		})
	}
}

func TestRuntimeApproval_StreamCorrelationAndValidity(t *testing.T) {
	call := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "mcp", Input: `{}`, ProviderExecuted: true}
	approval := provider.StreamPart{Type: provider.PartToolApprovalRequest, ToolCallID: "call", ApprovalID: "native", ProviderMetadata: provider.ProviderMetadata{"future": json.RawMessage(`{"empty":{},"false":false}`)}}
	local := call
	local.ProviderExecuted = false
	done := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "mcp", Result: json.RawMessage(`{}`)}
	conflicting := approval
	conflicting.ApprovalID = "another"
	otherCall := call
	otherCall.ToolCallID = "other"
	duplicateID := approval
	duplicateID.ToolCallID = "other"
	large := approval
	large.ApprovalID = string(make([]byte, 1<<20))
	invalid := approval
	invalid.ApprovalID = string([]byte{0xff})
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
		valid bool
	}{
		{"valid", []provider.StreamPart{call, approval}, true},
		{"unknown", []provider.StreamPart{approval}, false},
		{"local", []provider.StreamPart{local, approval}, false},
		{"completed", []provider.StreamPart{call, done, approval}, false},
		{"duplicate", []provider.StreamPart{call, approval, approval}, false},
		{"conflicting", []provider.StreamPart{call, approval, conflicting}, false},
		{"duplicate ID across calls", []provider.StreamPart{call, approval, otherCall, duplicateID}, false},
		{"large", []provider.StreamPart{call, large}, true},
		{"invalid utf8", []provider.StreamPart{call, invalid}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHarness(t, testLimits())
			h.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				parts := make(chan provider.StreamPart, len(tc.parts)+1)
				for _, p := range tc.parts {
					parts <- p
				}
				parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
				close(parts)
				return &provider.StreamResult{Stream: parts}, nil
			}
			response := h.serve(streamRequest(`{"prompt":[]}`))
			require.Equal(t, http.StatusOK, response.Code)
			if tc.valid {
				compiled, err := schema.CompileSchema(streamEventSchemaJSON)
				require.NoError(t, err)
				for _, line := range strings.Split(response.Body.String(), "\n") {
					if strings.HasPrefix(line, "data: ") {
						payload := strings.TrimPrefix(line, "data: ")
						if payload != "[DONE]" {
							require.NoError(t, compiled.Validate(json.RawMessage(payload)))
						}
					}
				}
				assert.Contains(t, response.Body.String(), `"type":"finish"`)
				assert.Contains(t, response.Body.String(), `"empty":{}`)
			} else {
				assert.Contains(t, response.Body.String(), `"type":"error"`)
				assert.NotContains(t, response.Body.String(), `"type":"finish"`)
			}
			assert.Equal(t, 1, h.model.callCount())
		})
	}
}

func TestRuntimeApproval_CancellationReleasesProducer(t *testing.T) {
	h := newRuntimeHarness(t, testLimits())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	h.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		parts := make(chan provider.StreamPart)
		go func() {
			defer close(parts)
			defer close(stopped)
			for _, part := range []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "mcp", Input: `{}`, ProviderExecuted: true}, {Type: provider.PartToolApprovalRequest, ToolCallID: "call", ApprovalID: "native"}} {
				select {
				case parts <- part:
				case <-ctx.Done():
					return
				}
			}
			<-ctx.Done()
		}()
		return &provider.StreamResult{Stream: parts}, nil
	}
	writer := &responseWriterProbe{}
	writer.onFlush = func(int) {
		if strings.Contains(writer.body.String(), `"approvalId":"native"`) {
			cancel()
		}
	}
	h.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`).WithContext(ctx))
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("approval producer not released")
	}
	assert.Contains(t, writer.body.String(), `"approvalId":"native"`)
	assert.NotContains(t, writer.body.String(), `"type":"finish"`)
	assert.Equal(t, 1, h.model.callCount())
}
