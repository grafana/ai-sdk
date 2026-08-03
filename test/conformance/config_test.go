//go:build conformance

package conformance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfig_Operation(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		extra     string
		want      Operation
		wantErr   bool
	}{
		{name: "defaults to stream", want: OperationStream},
		{name: "accepts generate", operation: "generate", want: OperationGenerate},
		{
			name:      "accepts empty unsupported collections",
			operation: "generate",
			extra:     "uiMessages: []\ntools: {}\nproviderTools: {}\nactiveTools: []\napprovals: []\nreasoning: \"\"\n",
			want:      OperationGenerate,
		},
		{name: "rejects unknown", operation: "invalid", wantErr: true},
		{
			name:      "rejects unsupported generate fields",
			operation: "generate",
			extra:     "reasoning: high\n",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			config := "model: test\n"
			if tc.operation != "" {
				config += "operation: " + tc.operation + "\n"
			}
			config += tc.extra
			path := filepath.Join(dir, "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(config), 0o600))

			cfg, err := LoadConfig(path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.Operation)
		})
	}
}

func TestConfig_UIToolModelOutput(t *testing.T) {
	cfg, err := LoadConfig("anthropic/recorded/ui-tool-model-output/config.yaml")
	require.NoError(t, err)
	require.Len(t, cfg.UIMessages, 2)
	require.NotNil(t, cfg.Tools["weather"].ModelOutput)

	tools, err := cfg.BuildToolSet()
	require.NoError(t, err)
	require.NotNil(t, tools["weather"].ToModelOutput)
	output, err := tools["weather"].ToModelOutput(aisdk.ToolOutputContext{})
	require.NoError(t, err)
	require.Equal(t, provider.ToolOutputContent, output.Type)
	require.Len(t, output.Content, 2)
	assert.Equal(t, "image/png", output.Content[1].MediaType)

	uiMessages, err := cfg.BuildUIMessages()
	require.NoError(t, err)
	messages, err := aisdk.ConvertToModelMessages(uiMessages, aisdk.WithTools(tools))
	require.NoError(t, err)
	require.Len(t, messages, 3)
	require.Equal(t, provider.ToolOutputContent, messages[2].Content[0].Output.Type)
}

type configCaptureModel struct {
	streamFunc func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error)
}

func (m *configCaptureModel) SpecificationVersion() string               { return "v4" }
func (m *configCaptureModel) Provider() string                           { return "mock" }
func (m *configCaptureModel) ModelID() string                            { return "mock-1" }
func (m *configCaptureModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *configCaptureModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	return m.streamFunc(ctx, opts)
}
func (m *configCaptureModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

func TestConfig_ToolChoiceAndStreamOptions(t *testing.T) {
	sendReasoning := false
	sendSources := true
	sendFinish := false
	sendStart := false
	cfg := Config{
		ToolChoice: &ToolChoiceConfig{Type: provider.ToolChoiceTool, ToolName: "weather"},
		StreamOptions: &StreamOptionsConfig{
			SendReasoning: &sendReasoning,
			SendSources:   &sendSources,
			SendFinish:    &sendFinish,
			SendStart:     &sendStart,
		},
	}

	choice := cfg.BuildToolChoice()
	require.NotNil(t, choice)
	assert.Equal(t, provider.ToolChoiceTool, choice.Type)
	assert.Equal(t, "weather", choice.ToolName)

	opts := cfg.BuildUIMessageStreamOptions()
	require.Len(t, opts, 4)
	assert.NotNil(t, opts)
}

func TestConfig_BuildToolSetProviderToolOptions(t *testing.T) {
	cfg := Config{
		ProviderTools: map[string]ProviderToolConfig{
			"web_search": {
				ID: "anthropic.web_search_20250305",
				Args: map[string]any{
					"maxUses": 1,
				},
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"query": map[string]any{"type": "string"}},
					"required":   []string{"query"},
				},
				ProviderOptions: map[string]any{
					"anthropic": map[string]any{"deferLoading": true},
				},
			},
		},
	}

	tools, err := cfg.BuildToolSet()
	require.NoError(t, err)

	tool := tools["web_search"]
	assert.Equal(t, "anthropic.web_search_20250305", tool.ID)
	assert.Contains(t, tool.Args, "maxUses")
	assert.Contains(t, tool.ProviderOptions, "anthropic")
	assert.NoError(t, tool.InputSchema.Validate(json.RawMessage(`{"query":"weather"}`)))
	assert.Error(t, tool.InputSchema.Validate(json.RawMessage(`{}`)))
}

func TestConfig_BuildToolSetPreservesExplicitFalseStrict(t *testing.T) {
	strict := false
	cfg := Config{
		Tools: map[string]ToolConfig{
			"weather": {
				Description: "weather",
				InputSchema: map[string]any{"type": "object"},
				Strict:      &strict,
			},
		},
	}

	tools, err := cfg.BuildToolSet()
	require.NoError(t, err)
	assert.Equal(t, &strict, tools["weather"].Strict)
}

func TestConfig_BuildStreamOptionsActiveTools(t *testing.T) {
	var receivedTools []provider.Tool
	var receivedReasoning *provider.ReasoningEffort
	model := &configCaptureModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			receivedTools = opts.Tools
			receivedReasoning = opts.Reasoning
			ch := make(chan provider.StreamPart, 4)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "ok"}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}
	cfg := Config{ActiveTools: []string{"search", "weather"}, Reasoning: provider.ReasoningHigh}

	streamOpts := cfg.buildStreamOptions(
		[]provider.Message{provider.UserText("hi")},
		aisdk.ToolSet{
			"search":     {Description: "search"},
			"calculator": {Description: "calculator"},
			"weather":    {Description: "weather"},
		},
		nil,
		[]aisdk.StopCondition{aisdk.StepCountIs(1)},
		nil,
		nil,
	)
	result := aisdk.StreamText(context.Background(), model, streamOpts...)
	for range result.FullStream() {
	}

	require.Len(t, receivedTools, 2)
	names := map[string]bool{}
	for _, tool := range receivedTools {
		names[tool.Name] = true
	}
	assert.True(t, names["search"])
	assert.True(t, names["weather"])
	require.NotNil(t, receivedReasoning)
	assert.Equal(t, provider.ReasoningHigh, *receivedReasoning)
}

func TestConfig_BuildMessagesConfiguredFileReference(t *testing.T) {
	raw := []byte(`
model: m
messages:
  - role: user
    content:
      - type: file
        mediaType: application/pdf
        filename: doc.pdf
        reference:
          openai: file-abc123
`)
	var cfg Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	messages, err := cfg.BuildMessages("fallback")
	require.NoError(t, err)

	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)
	part := messages[0].Content[0]
	assert.Equal(t, provider.ContentPartTypeFile, part.Type)
	assert.Equal(t, "application/pdf", part.MediaType)
	assert.Equal(t, "doc.pdf", part.Filename)
	require.NotNil(t, part.Data)
	assert.JSONEq(t, `{"openai":"file-abc123"}`, string(part.Data.Reference))
}

func TestConfig_BuildMessagesConfiguredToolApproval(t *testing.T) {
	raw := []byte(`
model: m
messages:
  - role: assistant
    content:
      - type: tool-approval-request
        approvalId: approval-1
        toolCallId: call-1
        isAutomatic: true
  - role: tool
    content:
      - type: tool-approval-response
        approvalId: approval-1
        approved: false
        reason: denied
        providerExecuted: true
`)
	var cfg Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	messages, err := cfg.BuildMessages("fallback")
	require.NoError(t, err)

	require.Len(t, messages, 2)
	request := messages[0].Content[0]
	assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, request.Type)
	assert.Equal(t, "approval-1", request.ApprovalID)
	assert.Equal(t, "call-1", request.ToolCallID)
	assert.True(t, request.IsAutomatic)

	response := messages[1].Content[0]
	assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, response.Type)
	assert.Equal(t, "approval-1", response.ApprovalID)
	require.NotNil(t, response.Approved)
	assert.False(t, *response.Approved)
	assert.Equal(t, "denied", response.Reason)
	assert.True(t, response.ProviderExecuted)
}

func TestConfig_BuildMessagesConfiguredReasoning(t *testing.T) {
	raw := []byte(`
model: m
prompt: ignored
messages:
  - role: assistant
    content:
      - type: reasoning
        text: thinking
        providerOptions:
          openai:
            itemId: rs_prev
  - role: user
    content: continue
`)
	var cfg Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	messages, err := cfg.BuildMessages("fallback")
	require.NoError(t, err)

	require.Len(t, messages, 2)
	assert.Equal(t, provider.RoleAssistant, messages[0].Role)
	require.Len(t, messages[0].Content, 1)
	assert.Equal(t, provider.ContentPartTypeReasoning, messages[0].Content[0].Type)
	assert.Equal(t, "thinking", messages[0].Content[0].Text)
	assert.Contains(t, messages[0].Content[0].ProviderOptions, "openai")
	assert.Equal(t, provider.RoleUser, messages[1].Role)
	assert.Equal(t, "continue", messages[1].Content[0].Text)
}

func TestConfig_BuildMessagesConfiguredToolCall(t *testing.T) {
	raw := []byte(`
model: m
messages:
  - role: assistant
    content:
      - type: tool-call
        toolCallId: call-1
        toolName: $READFILE
        input:
          path: /tmp/file
`)
	var cfg Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	messages, err := cfg.BuildMessages("ignored")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)
	part := messages[0].Content[0]
	assert.Equal(t, provider.ContentPartTypeToolCall, part.Type)
	assert.Equal(t, "call-1", part.ToolCallID)
	assert.Equal(t, "$READFILE", part.ToolName)
	assert.JSONEq(t, `{"path":"/tmp/file"}`, string(part.Input))
}

func TestConfig_BuildMessagesConfiguredFile(t *testing.T) {
	raw := []byte(`
model: m
messages:
  - role: user
    content:
      - type: file
        data: AAECAw==
        mediaType: application/pdf
        filename: report.pdf
`)
	var cfg Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	messages, err := cfg.BuildMessages("ignored")
	require.NoError(t, err)

	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)
	part := messages[0].Content[0]
	assert.Equal(t, provider.ContentPartTypeFile, part.Type)
	assert.Equal(t, "application/pdf", part.MediaType)
	assert.Equal(t, "report.pdf", part.Filename)
	require.NotNil(t, part.Data)
	assert.Equal(t, "AAECAw==", part.Data.Base64)

	raw = []byte(`
model: m
messages:
  - role: user
    content:
      - type: file
        url: s3://bucket/image.png
        mediaType: image/png
`)
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	messages, err = cfg.BuildMessages("ignored")
	require.NoError(t, err)
	part = messages[0].Content[0]
	require.NotNil(t, part.Data)
	assert.Equal(t, "s3://bucket/image.png", part.Data.URL)
	assert.Empty(t, part.Data.Base64)
}
