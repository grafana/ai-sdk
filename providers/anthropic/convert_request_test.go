package anthropic

import (
	"encoding/json"
	"fmt"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ provider.ProviderOption = AnthropicOptions{}
	_ provider.ProviderOption = AnthropicToolOptions{}
	_ provider.ProviderOption = AnthropicCacheControl{}
)

func warningFeatures(warnings []provider.Warning) []string {
	features := make([]string, 0, len(warnings))
	for _, w := range warnings {
		features = append(features, w.Feature)
	}
	return features
}

func TestBuildParams_SystemMessage(t *testing.T) {
	t.Run("initial block uses top-level system", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewSystemMessage("You are helpful"),
				provider.NewSystemMessage("Reply concisely"),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.System, 2)
		assert.Equal(t, "You are helpful", p.System[0].Text)
		assert.Equal(t, "Reply concisely", p.System[1].Text)
		assert.Empty(t, p.Messages)
		assert.NotContains(t, p.Betas, midConversationSystemBeta)
	})

	t.Run("later block stays ordered in messages", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewSystemMessage("You are helpful"),
				provider.UserText("Hello"),
				provider.AssistantText("Hi"),
				provider.NewSystemMessage("Reply concisely"),
				provider.UserText("Continue"),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.System, 1)
		assert.Equal(t, "You are helpful", p.System[0].Text)
		require.Len(t, p.Messages, 4)
		assert.EqualValues(t, "user", p.Messages[0].Role)
		assert.EqualValues(t, "assistant", p.Messages[1].Role)
		assert.Equal(t, sdk.BetaMessageParamRoleSystem, p.Messages[2].Role)
		require.Len(t, p.Messages[2].Content, 1)
		require.NotNil(t, p.Messages[2].Content[0].OfText)
		assert.Equal(t, "Reply concisely", p.Messages[2].Content[0].OfText.Text)
		assert.EqualValues(t, "user", p.Messages[3].Role)
		assert.Contains(t, p.Betas, midConversationSystemBeta)
	})
}

func TestBuildParams_UserTextMessage(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("Hello"),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	assert.EqualValues(t, "user", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 1)
	require.NotNil(t, p.Messages[0].Content[0].OfText)
	assert.Equal(t, "Hello", p.Messages[0].Content[0].OfText.Text)
}

func TestBuildParams_UserImageMessage(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile,
				Data:      &provider.DataContent{Base64: "abc123"},
				MediaType: "image/png",
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	require.Len(t, p.Messages[0].Content, 1)
	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfImage)
	require.NotNil(t, block.OfImage.Source.OfBase64)
	assert.Equal(t, "abc123", block.OfImage.Source.OfBase64.Data)
	assert.EqualValues(t, "image/png", block.OfImage.Source.OfBase64.MediaType)
}

func TestBuildParams_AssistantWithToolCall(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(
				provider.TextPart("Let me help"),
				provider.ToolCallPart("call_1", "search", json.RawMessage(`{"q":"test"}`)),
			),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	msg := p.Messages[0]
	assert.EqualValues(t, "assistant", msg.Role)
	require.Len(t, msg.Content, 2)
	require.NotNil(t, msg.Content[0].OfText, "expected text block first")
	require.NotNil(t, msg.Content[1].OfToolUse, "expected tool_use block second")
	assert.Equal(t, "call_1", msg.Content[1].OfToolUse.ID)
	assert.Equal(t, "search", msg.Content[1].OfToolUse.Name)
}

func TestBuildParams_AssistantWithInvalidToolCallInput(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(
				provider.ToolCallPart("call_1", "cityAttractions", json.RawMessage(`{ "city": "San Francisco", }`)),
			),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	require.Len(t, p.Messages[0].Content, 1)
	block := p.Messages[0].Content[0].OfToolUse
	require.NotNil(t, block)
	input, err := json.Marshal(block.Input)
	require.NoError(t, err)
	assert.JSONEq(t, `{"rawInvalidInput":"{ \"city\": \"San Francisco\", }"}`, string(input))
}

func TestBuildParams_AssistantWithMappedProviderToolCall(t *testing.T) {
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			provider.Tool{Type: provider.ToolTypeProvider,
				ID:   "anthropic.web_search_20250305",
				Name: "search_docs",
			},
		},
		Prompt: []provider.Message{
			provider.NewAssistantMessage(
				provider.ToolCallPart("call_1", "search_docs", json.RawMessage(`{"q":"test"}`)),
			),
		},
	}

	p, mapping, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	require.Len(t, p.Messages[0].Content, 1)
	require.NotNil(t, p.Messages[0].Content[0].OfToolUse)
	assert.Equal(t, "web_search", p.Messages[0].Content[0].OfToolUse.Name)
	assert.Equal(t, "web_search", mapping.toProviderToolName("search_docs"))
	assert.Equal(t, "search_docs", mapping.toCustomToolName("web_search"))
}

func TestBuildParams_AssistantWithReasoning(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(
				provider.ReasoningPart("thinking..."),
				provider.TextPart("answer"),
			),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages[0].Content, 2)
	require.NotNil(t, p.Messages[0].Content[0].OfThinking)
	assert.Equal(t, "thinking...", p.Messages[0].Content[0].OfThinking.Thinking)
}

func TestBuildParams_AssistantPrefillWhitespace(t *testing.T) {
	tests := []struct {
		name   string
		prompt []provider.Message
		want   []string
	}{
		{
			name: "only final assistant text is trimmed",
			prompt: []provider.Message{
				provider.UserText("First question"),
				provider.AssistantText("Earlier assistant text  \n"),
				provider.UserText("Follow-up question"),
				provider.NewAssistantMessage(
					provider.TextPart("Prefill prefix  \n"),
					provider.TextPart("Final assistant prefill  \n"),
				),
			},
			want: []string{"Earlier assistant text  \n", "Prefill prefix  \n", "Final assistant prefill"},
		},
		{
			name: "ECMAScript byte order mark whitespace is trimmed",
			prompt: []provider.Message{
				provider.AssistantText("\ufeff Final assistant prefill \ufeff"),
			},
			want: []string{"Final assistant prefill"},
		},
		{
			name: "non-ECMAScript next line whitespace is preserved",
			prompt: []provider.Message{
				provider.AssistantText("\u0085Final assistant prefill\u0085"),
			},
			want: []string{"\u0085Final assistant prefill\u0085"},
		},
		{
			name: "text before final tool call is preserved",
			prompt: []provider.Message{
				provider.UserText("Question"),
				provider.NewAssistantMessage(
					provider.TextPart("Calling a tool  \n"),
					provider.ToolCallPart("call_1", "search", json.RawMessage(`{}`)),
				),
			},
			want: []string{"Calling a tool  \n"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: tc.prompt}, false)
			require.NoError(t, err)

			var got []string
			for _, message := range p.Messages {
				if message.Role != sdk.BetaMessageParamRoleAssistant {
					continue
				}
				for _, part := range message.Content {
					if part.OfText != nil {
						got = append(got, part.OfText.Text)
					}
				}
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildParams_AssistantCompaction(t *testing.T) {
	part := provider.TextPart("Compaction summary  \n")
	part.ProviderOptions = makeProviderOpts(`{"type":"compaction","cacheControl":{"type":"ephemeral"}}`)

	p, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("Continue"),
			provider.NewAssistantMessage(part),
		},
	}, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 2)
	require.Len(t, p.Messages[1].Content, 1)
	block := p.Messages[1].Content[0]
	require.NotNil(t, block.OfCompaction)
	assert.Nil(t, block.OfText)
	assert.True(t, block.OfCompaction.Content.Valid())
	assert.Equal(t, "Compaction summary  \n", block.OfCompaction.Content.Value)
	assert.EqualValues(t, "ephemeral", block.OfCompaction.CacheControl.Type)
}

func TestBuildParams_AssistantTextCitations(t *testing.T) {
	part := provider.TextPart("The Federal Reserve held rates steady.")
	part.ProviderOptions = makeProviderOpts(`{"citations":[
		{"type":"web_search_result_location","cited_text":"web text","url":"https://example.com/fed-decision","title":"Federal Reserve decision","encrypted_index":"encrypted-index"},
		{"type":"page_location","cited_text":"page text","document_index":0,"document_title":"Document","start_page_number":1,"end_page_number":2},
		{"type":"char_location","cited_text":"char text","document_index":0,"document_title":"Document","start_char_index":10,"end_char_index":20},
		{"type":"content_block_location","cited_text":"block text","document_index":0,"document_title":"Document","start_block_index":0,"end_block_index":1},
		{"type":"search_result_location","cited_text":"result text","search_result_index":0,"source":"https://example.com/result","title":"Result","start_block_index":0,"end_block_index":1}
	]}`)

	p, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(part),
			provider.UserText("What happened before that?"),
		},
	}, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 2)
	require.Len(t, p.Messages[0].Content, 1)
	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfText)
	require.Len(t, block.OfText.Citations, 5)
	webCitation := block.OfText.Citations[0].OfWebSearchResultLocation
	require.NotNil(t, webCitation)
	assert.Equal(t, "web text", webCitation.CitedText)
	assert.Equal(t, "https://example.com/fed-decision", webCitation.URL)
	assert.Equal(t, "Federal Reserve decision", webCitation.Title.Value)
	assert.Equal(t, "encrypted-index", webCitation.EncryptedIndex)
	pageCitation := block.OfText.Citations[1].OfPageLocation
	require.NotNil(t, pageCitation)
	assert.Equal(t, "page text", pageCitation.CitedText)
	assert.EqualValues(t, 1, pageCitation.StartPageNumber)
	assert.EqualValues(t, 2, pageCitation.EndPageNumber)
	charCitation := block.OfText.Citations[2].OfCharLocation
	require.NotNil(t, charCitation)
	assert.Equal(t, "char text", charCitation.CitedText)
	assert.EqualValues(t, 10, charCitation.StartCharIndex)
	assert.EqualValues(t, 20, charCitation.EndCharIndex)
	blockCitation := block.OfText.Citations[3].OfContentBlockLocation
	require.NotNil(t, blockCitation)
	assert.Equal(t, "block text", blockCitation.CitedText)
	assert.EqualValues(t, 0, blockCitation.StartBlockIndex)
	assert.EqualValues(t, 1, blockCitation.EndBlockIndex)
	searchCitation := block.OfText.Citations[4].OfSearchResultLocation
	require.NotNil(t, searchCitation)
	assert.Equal(t, "result text", searchCitation.CitedText)
	assert.Equal(t, "https://example.com/result", searchCitation.Source)
	assert.Equal(t, "Result", searchCitation.Title.Value)
}

func TestBuildParams_ToolMessage(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewToolMessage(provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "result"})),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	require.NotNil(t, p.Messages[0].Content[0].OfToolResult)
	assert.Equal(t, "call_1", p.Messages[0].Content[0].OfToolResult.ToolUseID)
}

func TestBuildParams_Tools(t *testing.T) {
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
			},
		},
	}

	p, mapping, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Tools, 1)
	require.NotNil(t, p.Tools[0].OfTool)
	assert.Equal(t, "search", p.Tools[0].OfTool.Name)
	assert.Empty(t, mapping.customToolNameToProviderToolName)
	assert.Empty(t, mapping.providerToolNameToCustomToolName)
}

func TestBuildParams_ReturnsToolNameMapping(t *testing.T) {
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			provider.Tool{Type: provider.ToolTypeProvider,
				ID:   "anthropic.tool_search_regex_20251119",
				Name: "search_regex",
			},
		},
	}

	_, mapping, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.Equal(t, "tool_search_tool_regex", mapping.toProviderToolName("search_regex"))
	assert.Equal(t, "search_regex", mapping.toCustomToolName("tool_search_tool_regex"))
}

func TestBuildParams_ToolChoice(t *testing.T) {
	tests := []struct {
		choice provider.ToolChoice
		check  func(t *testing.T, p interface{})
	}{
		{
			provider.ToolChoice{Type: provider.ToolChoiceAuto},
			func(t *testing.T, _ interface{}) {},
		},
		{
			provider.ToolChoice{Type: provider.ToolChoiceNone},
			func(t *testing.T, _ interface{}) {},
		},
		{
			provider.ToolChoice{Type: provider.ToolChoiceRequired},
			func(t *testing.T, _ interface{}) {},
		},
		{
			provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "search"},
			func(t *testing.T, _ interface{}) {},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.choice.Type), func(t *testing.T) {
			opts := provider.CallOptions{
				ToolChoice: &tt.choice,
			}
			_, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
			require.NoError(t, err)
		})
	}
}

func TestBuildParams_ToolChoiceNoneDropsTools(t *testing.T) {
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			{
				Type:        provider.ToolTypeFunction,
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
		ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceNone},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.Empty(t, p.Tools)
	assert.Nil(t, p.ToolChoice.OfNone)
	assert.Nil(t, p.ToolChoice.OfAuto)
	assert.Nil(t, p.ToolChoice.OfAny)
	assert.Nil(t, p.ToolChoice.OfTool)
}

func TestBuildParams_StrictFunctionTool(t *testing.T) {
	strictTrue := true
	strictFalse := false
	tests := []struct {
		name   string
		strict *bool
	}{
		{name: "omitted"},
		{name: "false", strict: &strictFalse},
		{name: "true", strict: &strictTrue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := provider.CallOptions{
				Tools: []provider.Tool{{
					Type:        provider.ToolTypeFunction,
					Name:        "search",
					Description: "Search the web",
					InputSchema: json.RawMessage(`{"type":"object"}`),
					Strict:      tc.strict,
				}},
			}

			p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
			require.NoError(t, err)
			require.Len(t, p.Tools, 1)
			require.NotNil(t, p.Tools[0].OfTool)
			if tc.strict == nil {
				assert.False(t, p.Tools[0].OfTool.Strict.Valid())
			} else {
				assert.True(t, p.Tools[0].OfTool.Strict.Valid())
				assert.Equal(t, *tc.strict, p.Tools[0].OfTool.Strict.Value)
			}
			assert.Contains(t, p.Betas, sdk.AnthropicBeta("structured-outputs-2025-11-13"))
		})
	}
}

func TestBuildParams_StrictFunctionToolUnsupported(t *testing.T) {
	tests := []struct {
		name                   string
		modelID                string
		strict                 bool
		providerSupportsStrict bool
	}{
		{name: "unsupported model true", modelID: "claude-3-haiku", strict: true, providerSupportsStrict: true},
		{name: "unsupported model false", modelID: "claude-3-haiku", strict: false, providerSupportsStrict: true},
		{name: "disabled provider false", modelID: "claude-sonnet-4-6", strict: false, providerSupportsStrict: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _, warnings, _, err := buildParamsWithCapabilities(tc.modelID, provider.CallOptions{
				Tools: []provider.Tool{{
					Type:        provider.ToolTypeFunction,
					Name:        "search",
					InputSchema: json.RawMessage(`{"type":"object"}`),
					Strict:      &tc.strict,
				}},
			}, false, providerCapabilities{
				supportsNativeStructuredOutput: true,
				supportsStrictTools:            tc.providerSupportsStrict,
			})
			require.NoError(t, err)
			require.Len(t, p.Tools, 1)
			require.NotNil(t, p.Tools[0].OfTool)
			assert.False(t, p.Tools[0].OfTool.Strict.Valid())
			require.Len(t, warnings, 1)
			assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
			assert.Equal(t, "strict", warnings[0].Feature)
			assert.Contains(t, warnings[0].Details, fmt.Sprintf("strict: %t", tc.strict))
		})
	}
}

func TestBuildParams_VertexFunctionTools(t *testing.T) {
	strictTrue := true
	strictFalse := false
	tests := []struct {
		name   string
		strict *bool
	}{
		{name: "omitted"},
		{name: "false", strict: &strictFalse},
		{name: "true", strict: &strictTrue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _, warnings, _, err := buildParamsWithCapabilities("claude-sonnet-4-6", provider.CallOptions{
				Tools: []provider.Tool{{
					Type:        provider.ToolTypeFunction,
					Name:        "search",
					InputSchema: json.RawMessage(`{"type":"object"}`),
					Strict:      tc.strict,
				}},
			}, false, vertexProviderCapabilities)
			require.NoError(t, err)
			require.Len(t, p.Tools, 1)
			require.NotNil(t, p.Tools[0].OfTool)
			assert.False(t, p.Tools[0].OfTool.Strict.Valid())
			assert.NotContains(t, p.Betas, sdk.AnthropicBeta("structured-outputs-2025-11-13"))
			if tc.strict == nil {
				assert.Empty(t, warnings)
			} else {
				require.Len(t, warnings, 1)
				assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
				assert.Equal(t, "strict", warnings[0].Feature)
			}
		})
	}
}

func TestBuildParams_VertexExplicitStructuredOutputBeta(t *testing.T) {
	testSchema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	tests := []struct {
		name           string
		responseFormat *provider.ResponseFormat
		wantFallback   bool
	}{
		{name: "function tool"},
		{
			name: "JSON tool fallback",
			responseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			wantFallback: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _, _, br, err := buildParamsWithCapabilities("claude-sonnet-4-6", provider.CallOptions{
				Tools: []provider.Tool{{
					Type:        provider.ToolTypeFunction,
					Name:        "search",
					InputSchema: json.RawMessage(`{"type":"object"}`),
				}},
				ResponseFormat: tc.responseFormat,
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{
						Key: "anthropic",
						Raw: json.RawMessage(`{"betas":["structured-outputs-2025-11-13"]}`),
					},
				},
			}, false, vertexProviderCapabilities)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFallback, br.usesJsonResponseTool)
			assert.Contains(t, p.Betas, sdk.AnthropicBeta("structured-outputs-2025-11-13"))
		})
	}
}

func TestBuildParams_ToolChoiceMapsProviderToolName(t *testing.T) {
	choice := provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "custom_search"}
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305", Name: "custom_search"},
		},
		ToolChoice: &choice,
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)
	require.NotNil(t, p.ToolChoice.OfTool)
	assert.Equal(t, "web_search", p.ToolChoice.OfTool.Name)
}

func TestBuildParams_ScalarParams(t *testing.T) {
	temp := 0.7
	maxTokens := 1000
	topP := 0.9
	topK := 40
	opts := provider.CallOptions{
		Temperature:     &temp,
		MaxOutputTokens: &maxTokens,
		TopP:            &topP,
		TopK:            &topK,
		StopSequences:   []string{"STOP"},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.Equal(t, int64(1000), p.MaxTokens)
	assert.Equal(t, []string{"STOP"}, p.StopSequences)
}

func TestBuildParams_DefaultMaxTokensFromCapabilities(t *testing.T) {
	p, _, warnings, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{}, false)
	require.NoError(t, err)
	assert.Equal(t, int64(128000), p.MaxTokens)
	assert.Empty(t, warnings)

	for _, tc := range []struct {
		modelID string
		wantMax int64
	}{
		{modelID: "some-future-model", wantMax: 4096},
		{modelID: "claude-future-9", wantMax: 128000},
	} {
		t.Run(tc.modelID, func(t *testing.T) {
			params, _, gotWarnings, _, err := buildParams(tc.modelID, provider.CallOptions{}, false)
			require.NoError(t, err)
			assert.Equal(t, tc.wantMax, params.MaxTokens)
			require.Len(t, gotWarnings, 1)
			assert.Equal(t, provider.WarnCompatibility, gotWarnings[0].Type)
			assert.Equal(t, "maxOutputTokens", gotWarnings[0].Feature)
			assert.Equal(t, fmt.Sprintf("The model %q is unknown. The max output tokens have been limited to %d. Set maxOutputTokens explicitly to override this limit.", tc.modelID, tc.wantMax), gotWarnings[0].Details)
		})
	}
}

func TestBuildParams_ExplicitMaxOutputTokensOverridesModelDefault(t *testing.T) {
	maxTok := 2048
	opts := provider.CallOptions{MaxOutputTokens: &maxTok}
	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)
	assert.Equal(t, int64(2048), p.MaxTokens)
}

func TestBuildParams_ClampMaxTokens_UserProvidedWithThinkingEmitsWarning(t *testing.T) {
	maxTok := 60000
	opts := provider.CallOptions{
		MaxOutputTokens: &maxTok,
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":10000}}`)},
		},
	}
	p, _, warnings, _, err := buildParams("claude-sonnet-4-5", opts, false)
	require.NoError(t, err)
	assert.Equal(t, int64(64000), p.MaxTokens)
	require.Len(t, warnings, 1)
	assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
	assert.Equal(t, "maxOutputTokens", warnings[0].Feature)
	assert.Contains(t, warnings[0].Details, "64000")
}

func TestBuildParams_ClampMaxTokens_DefaultPlusThinkingSilent(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":10000}}`)},
		},
	}
	p, _, warnings, _, err := buildParams("claude-3-haiku", opts, false)
	require.NoError(t, err)
	assert.Equal(t, int64(4096), p.MaxTokens)
	var clampWarn bool
	for _, w := range warnings {
		if w.Feature == "maxOutputTokens" {
			clampWarn = true
		}
	}
	assert.False(t, clampWarn)
}

func TestBuildParams_NoClampUnknownModel(t *testing.T) {
	maxTok := 200000
	opts := provider.CallOptions{MaxOutputTokens: &maxTok}
	p, _, warnings, _, err := buildParams("some-future-model", opts, false)
	require.NoError(t, err)
	assert.Equal(t, int64(200000), p.MaxTokens)
	for _, w := range warnings {
		assert.NotEqual(t, "maxOutputTokens", w.Feature)
	}
}

func TestBuildParams_NoClampWithinModelLimit(t *testing.T) {
	maxTok := 50000
	opts := provider.CallOptions{MaxOutputTokens: &maxTok}
	p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)
	assert.Equal(t, int64(50000), p.MaxTokens)
	for _, w := range warnings {
		assert.NotEqual(t, "maxOutputTokens", w.Feature)
	}
}

func TestBuildParams_ThinkingBudgetAddedUnderModelCap(t *testing.T) {
	maxTok := 50000
	opts := provider.CallOptions{
		MaxOutputTokens: &maxTok,
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":5000}}`)},
		},
	}
	p, _, warnings, _, err := buildParams("claude-sonnet-4-5", opts, false)
	require.NoError(t, err)
	assert.Equal(t, int64(55000), p.MaxTokens)
	for _, w := range warnings {
		assert.NotEqual(t, "maxOutputTokens", w.Feature)
	}
}

func TestBuildParams_DefaultThinkingBudget(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled"}}`)},
		},
	}
	p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)
	require.NotNil(t, p.Thinking.OfEnabled)
	assert.Equal(t, int64(1024), p.Thinking.OfEnabled.BudgetTokens)
	require.Len(t, warnings, 1)
	assert.Equal(t, provider.WarnCompatibility, warnings[0].Type)
	assert.Equal(t, "extended thinking", warnings[0].Feature)
	assert.Contains(t, warnings[0].Details, "default budget of 1024")
}

func TestBuildParams_UnsupportedParams(t *testing.T) {
	pp := 0.5
	fp := 0.5
	seed := 42
	opts := provider.CallOptions{
		PresencePenalty:  &pp,
		FrequencyPenalty: &fp,
		Seed:             &seed,
	}

	_, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, warnings, 3)
	features := map[string]bool{}
	for _, w := range warnings {
		features[w.Feature] = true
		assert.Equal(t, provider.WarnUnsupported, w.Type)
	}
	for _, f := range []string{"presencePenalty", "frequencyPenalty", "seed"} {
		assert.True(t, features[f], "missing warning for %q", f)
	}
}

func TestBuildParams_ProviderOptions_Thinking(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":5000}}`)},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.NotNil(t, p.Thinking.OfEnabled)
	assert.Equal(t, int64(5000), p.Thinking.OfEnabled.BudgetTokens)
	assert.Equal(t, int64(128000), p.MaxTokens, "default max plus budget exceeds model cap; clamp to maxOutputTokens")
}

func TestBuildParams_ProviderOptions_AdaptiveThinking(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"adaptive"}}`)},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.NotNil(t, p.Thinking.OfAdaptive)
	assert.Equal(t, int64(128000), p.MaxTokens, "adaptive thinking must not add a budget to max_tokens")
}

func TestBuildParams_ProviderOptions_DisabledThinking(t *testing.T) {
	t.Run("forwarded without budget", func(t *testing.T) {
		opts := provider.CallOptions{
			MaxOutputTokens: ptrInt(100),
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"disabled"}}`)},
			},
		}

		p, _, warnings, _, err := buildParams("claude-sonnet-5", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfDisabled)
		assert.Equal(t, int64(100), p.MaxTokens)
		assert.Empty(t, warnings)
	})

	t.Run("keeps sampling params", func(t *testing.T) {
		topK := 1
		temperature := 0.5
		opts := provider.CallOptions{
			Temperature: &temperature,
			TopK:        &topK,
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"disabled"}}`)},
			},
		}

		p, _, warnings, _, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfDisabled)
		assert.Equal(t, 0.5, p.Temperature.Value)
		assert.Equal(t, int64(1), p.TopK.Value)
		assert.Empty(t, warnings)
	})
}

func TestBuildParams_ProviderOptions_Betas(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"betas":["extended-context-1m"]}`)},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.Contains(t, p.Betas, "extended-context-1m")
}

func TestBuildParams_NoProviderOptions(t *testing.T) {
	opts := provider.CallOptions{}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.Nil(t, p.Thinking.OfEnabled, "expected no thinking config")
	assert.Nil(t, p.Thinking.OfAdaptive, "expected no thinking config")
	assert.Nil(t, p.Thinking.OfDisabled, "expected no thinking config")
	assert.Empty(t, p.OutputConfig.Effort)
	assert.Equal(t, int64(128000), p.MaxTokens)
}

func TestBuildParams_ProviderOptions_Effort(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"effort":"high"}`)},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.Equal(t, "high", string(p.OutputConfig.Effort))
	assert.NotContains(t, p.Betas, "effort-2025-11-24")
}

func TestBuildParams_ProviderOptions_EffortWithAdaptiveThinking(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"adaptive"},"effort":"max"}`)},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.NotNil(t, p.Thinking.OfAdaptive)
	assert.Equal(t, "max", string(p.OutputConfig.Effort))
	assert.Equal(t, int64(128000), p.MaxTokens)
}

func TestBuildParams_ProviderOptions_EffortWithEnabledThinking(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":5000},"effort":"high"}`)},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.NotNil(t, p.Thinking.OfEnabled)
	assert.Equal(t, int64(5000), p.Thinking.OfEnabled.BudgetTokens)
	assert.Equal(t, "high", string(p.OutputConfig.Effort))
	assert.Equal(t, int64(128000), p.MaxTokens)
}

func TestBuildParams_ProviderOptions_AdaptiveThinkingWithDisplay(t *testing.T) {
	t.Run("summarized display set on adaptive", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"adaptive","display":"summarized"}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfAdaptive)
		assert.Equal(t, "summarized", string(p.Thinking.OfAdaptive.Display))
	})

	t.Run("omitted display set on adaptive", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"adaptive","display":"omitted"}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfAdaptive)
		assert.Equal(t, "omitted", string(p.Thinking.OfAdaptive.Display))
	})

	t.Run("display absent on adaptive when not set", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"adaptive"}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfAdaptive)
		assert.Empty(t, string(p.Thinking.OfAdaptive.Display))
	})

	t.Run("display ignored on enabled thinking", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":5000,"display":"omitted"}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfEnabled)
		assert.Nil(t, p.Thinking.OfAdaptive, "expected adaptive to be unset for enabled thinking")
	})
}

func TestBuildParams_ProviderOptions_TaskBudget(t *testing.T) {
	t.Run("task budget with total only", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"taskBudget":{"type":"tokens","total":50000}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		assert.Equal(t, int64(50000), p.OutputConfig.TaskBudget.Total)
		assert.False(t, p.OutputConfig.TaskBudget.Remaining.Valid(), "remaining must be unset when not provided")
		assert.Contains(t, p.Betas, sdk.AnthropicBeta("task-budgets-2026-03-13"))
	})

	t.Run("task budget with remaining", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"taskBudget":{"type":"tokens","total":50000,"remaining":30000}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		assert.Equal(t, int64(50000), p.OutputConfig.TaskBudget.Total)
		require.True(t, p.OutputConfig.TaskBudget.Remaining.Valid())
		assert.Equal(t, int64(30000), p.OutputConfig.TaskBudget.Remaining.Value)
		assert.Contains(t, p.Betas, sdk.AnthropicBeta("task-budgets-2026-03-13"))
	})

	t.Run("task budget beta absent when not set", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		assert.Equal(t, int64(0), p.OutputConfig.TaskBudget.Total)
		assert.NotContains(t, p.Betas, sdk.AnthropicBeta("task-budgets-2026-03-13"))
	})

	t.Run("task budget marshals as expected JSON", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"taskBudget":{"type":"tokens","total":50000,"remaining":30000}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		data, err := json.Marshal(p.OutputConfig)
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(data, &got))

		tb, ok := got["task_budget"].(map[string]any)
		require.True(t, ok, "task_budget must be present in marshaled output_config")
		assert.Equal(t, "tokens", tb["type"])
		assert.EqualValues(t, 50000, tb["total"])
		assert.EqualValues(t, 30000, tb["remaining"])
	})
}

// TestBuildParams_ProviderOptions_TaskBudget_Validation asserts that
// caller-supplied task budgets are validated against upstream's Zod schema
// constraints before being sent to the API. Invalid budgets MUST produce an
// "other" warning and be dropped so the request still goes through without
// a 400.
func TestBuildParams_ProviderOptions_TaskBudget_Validation(t *testing.T) {
	taskBudgetWarning := func(t *testing.T, warnings []provider.Warning) provider.Warning {
		t.Helper()
		for _, w := range warnings {
			if w.Feature == "taskBudget" {
				return w
			}
		}
		t.Fatalf("expected a taskBudget warning, got: %+v", warnings)
		return provider.Warning{}
	}

	t.Run("unsupported type emits warning and skips budget", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"taskBudget":{"type":"requests","total":50000}}`)},
			},
		}

		p, _, warnings, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		w := taskBudgetWarning(t, warnings)
		assert.Equal(t, provider.WarnOther, w.Type)
		assert.Contains(t, w.Message, "tokens")

		assert.Equal(t, int64(0), p.OutputConfig.TaskBudget.Total, "task budget must be skipped on invalid type")
		assert.NotContains(t, p.Betas, sdk.AnthropicBeta("task-budgets-2026-03-13"))
	})

	t.Run("total below minimum emits warning and skips budget", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"taskBudget":{"type":"tokens","total":1000}}`)},
			},
		}

		p, _, warnings, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		w := taskBudgetWarning(t, warnings)
		assert.Equal(t, provider.WarnOther, w.Type)
		assert.Contains(t, w.Message, "20000")

		assert.Equal(t, int64(0), p.OutputConfig.TaskBudget.Total)
		assert.NotContains(t, p.Betas, sdk.AnthropicBeta("task-budgets-2026-03-13"))
	})

	t.Run("negative remaining emits warning and skips budget", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"taskBudget":{"type":"tokens","total":50000,"remaining":-1}}`)},
			},
		}

		p, _, warnings, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		w := taskBudgetWarning(t, warnings)
		assert.Equal(t, provider.WarnOther, w.Type)
		assert.Contains(t, w.Message, "remaining")

		assert.Equal(t, int64(0), p.OutputConfig.TaskBudget.Total)
		assert.NotContains(t, p.Betas, sdk.AnthropicBeta("task-budgets-2026-03-13"))
	})

	t.Run("empty type defaults to tokens with no warning", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"taskBudget":{"total":50000}}`)},
			},
		}

		p, _, warnings, _, err := buildParams("claude-opus-4-7", opts, false)
		require.NoError(t, err)

		for _, w := range warnings {
			assert.NotEqual(t, "taskBudget", w.Feature, "no taskBudget warning expected for empty type")
		}

		assert.Equal(t, int64(50000), p.OutputConfig.TaskBudget.Total)
		assert.Contains(t, p.Betas, sdk.AnthropicBeta("task-budgets-2026-03-13"))
	})
}

func TestBuildParams_ResponseFormatWarning_SchemalessJSON(t *testing.T) {
	opts := provider.CallOptions{
		ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON},
	}

	_, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, warnings, 1)
	assert.Equal(t, "responseFormat", warnings[0].Feature)
	assert.Contains(t, warnings[0].Details, "schemaless")
}

func TestBuildParams_BetaDeduplication(t *testing.T) {
	opts := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":5000},"betas":["interleaved-thinking-2025-05-14","other-beta"]}`)},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.Equal(t, int64(128000), p.MaxTokens)

	count := 0
	for _, b := range p.Betas {
		if b == "interleaved-thinking-2025-05-14" {
			count++
		}
	}
	assert.Equal(t, 1, count, "expected 1 interleaved-thinking beta, got %d (betas: %v)", count, p.Betas)

	assert.Contains(t, p.Betas, "other-beta")
}

func TestSerializeToolOutput_ExecutionDenied(t *testing.T) {
	output := provider.ToolResultOutput{
		Type:   provider.ToolOutputExecutionDenied,
		Reason: "user rejected the tool call",
	}

	blocks := serializeToolOutput(&output, nil)

	require.Len(t, blocks, 1)
	require.NotNil(t, blocks[0].OfText)
	assert.Equal(t, "user rejected the tool call", blocks[0].OfText.Text)
}

func TestSerializeToolOutput_ExecutionDenied_DefaultReason(t *testing.T) {
	output := provider.ToolResultOutput{
		Type: provider.ToolOutputExecutionDenied,
	}

	blocks := serializeToolOutput(&output, nil)

	assert.Equal(t, "tool execution was denied", blocks[0].OfText.Text)
}

func TestSerializeToolOutput_Content(t *testing.T) {
	output := provider.ToolResultOutput{
		Type: provider.ToolOutputContent,
		Content: []provider.ToolResultContentValue{
			{Type: provider.ToolContentText, Text: "result text"},
			{Type: provider.ToolContentFileData, Data: "imgdata", MediaType: "image/png"},
		},
	}

	blocks := serializeToolOutput(&output, nil)

	require.Len(t, blocks, 2)
	require.NotNil(t, blocks[0].OfText)
	assert.Equal(t, "result text", blocks[0].OfText.Text)
	require.NotNil(t, blocks[1].OfImage)
	assert.Equal(t, "imgdata", blocks[1].OfImage.Source.OfBase64.Data)
}

func TestSerializeToolOutput_UnsupportedTypeWarns(t *testing.T) {
	output := provider.ToolResultOutput{Type: provider.ToolResultOutputType("future")}
	var warnings []provider.Warning

	blocks := serializeToolOutput(&output, &warnings)

	require.Len(t, blocks, 1)
	require.NotNil(t, blocks[0].OfText)
	assert.Empty(t, blocks[0].OfText.Text)
	require.Len(t, warnings, 1)
	assert.Equal(t, "toolResultOutput", warnings[0].Feature)
	assert.Contains(t, warnings[0].Message, "future")
}

func TestBuildParams_UserImageFromBytes(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile,
				Data:      &provider.DataContent{Bytes: []byte{0x89, 0x50, 0x4E, 0x47}},
				MediaType: "image/png",
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfImage)
	require.NotNil(t, block.OfImage.Source.OfBase64)
	assert.NotEmpty(t, block.OfImage.Source.OfBase64.Data)
}

func TestBuildParams_UserImageFromURL(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile,
				Data:      &provider.DataContent{URL: "https://example.com/image.png"},
				MediaType: "image/png",
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfImage)
	require.NotNil(t, block.OfImage.Source.OfURL)
	assert.Equal(t, "https://example.com/image.png", block.OfImage.Source.OfURL.URL)
}

func TestBuildParams_UserImageWildcardMediaType(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile,
				Data:      &provider.DataContent{Base64: "abc123"},
				MediaType: "image/*",
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfImage)
	require.NotNil(t, block.OfImage.Source.OfBase64)
	assert.EqualValues(t, "image/jpeg", block.OfImage.Source.OfBase64.MediaType)
}

func TestBuildParams_FileFromURL(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile,
				Data:      &provider.DataContent{URL: "https://example.com/doc.pdf"},
				MediaType: "application/pdf",
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfDocument)
	require.NotNil(t, block.OfDocument.Source.OfURL)
	assert.Equal(t, "https://example.com/doc.pdf", block.OfDocument.Source.OfURL.URL)
}

func TestBuildParams_SystemMessageCacheControl(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.Message{
				Role:            provider.RoleSystem,
				Content:         []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: "You are helpful"}},
				ProviderOptions: makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`),
			},
		},
	}

	p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)
	assert.Len(t, warnings, 0)
	require.Len(t, p.System, 1)
	assert.EqualValues(t, "ephemeral", p.System[0].CacheControl.Type)
}

func TestBuildParams_UserTextCacheControl(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText,
				Text:            "Hello",
				ProviderOptions: makeProviderOpts(`{"cacheControl": {"type": "ephemeral", "ttl": "1h"}}`),
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfText)
	assert.EqualValues(t, "ephemeral", block.OfText.CacheControl.Type)
	assert.EqualValues(t, "1h", block.OfText.CacheControl.TTL)
}

func TestBuildParams_ToolDefinitionCacheControl(t *testing.T) {
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:            "search",
				Description:     "Search the web",
				InputSchema:     json.RawMessage(`{"type":"object","properties":{}}`),
				ProviderOptions: makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`),
			},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Tools, 1)
	require.NotNil(t, p.Tools[0].OfTool)
	assert.EqualValues(t, "ephemeral", p.Tools[0].OfTool.CacheControl.Type)
}

func TestBuildParams_LastPartCascade(t *testing.T) {
	msgOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.Message{Role: provider.RoleUser,
				Content: []provider.ContentPart{
					provider.TextPart("first"),
					provider.TextPart("last"),
				},
				ProviderOptions: msgOpts,
			},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages[0].Content, 2)

	first := p.Messages[0].Content[0].OfText
	assert.NotEqualValues(t, "ephemeral", first.CacheControl.Type, "first part should NOT inherit message-level cache_control")

	last := p.Messages[0].Content[1].OfText
	assert.EqualValues(t, "ephemeral", last.CacheControl.Type, "last part should inherit message-level cache_control")
}

func TestBuildParams_PartOverridesMessageCascade(t *testing.T) {
	msgOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral", "ttl": "1h"}}`)
	partOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral", "ttl": "5m"}}`)

	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.Message{Role: provider.RoleUser,
				Content: []provider.ContentPart{
					provider.ContentPart{Type: provider.ContentPartTypeText, Text: "only", ProviderOptions: partOpts},
				},
				ProviderOptions: msgOpts,
			},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0].OfText
	assert.EqualValues(t, "5m", block.CacheControl.TTL, "part-level should override message-level")
}

func TestBuildParams_BreakpointLimit(t *testing.T) {
	ccOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction, Name: "t1", InputSchema: json.RawMessage(`{}`), ProviderOptions: ccOpts},
			provider.Tool{Type: provider.ToolTypeFunction, Name: "t2", InputSchema: json.RawMessage(`{}`), ProviderOptions: ccOpts},
		},
		Prompt: []provider.Message{
			provider.Message{Role: provider.RoleSystem, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: "sys1"}}, ProviderOptions: ccOpts},
			provider.Message{Role: provider.RoleSystem, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: "sys2"}}, ProviderOptions: ccOpts},
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "u1", ProviderOptions: ccOpts}),
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "u2", ProviderOptions: ccOpts}),
		},
	}

	p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	cached := 0
	for _, tool := range p.Tools {
		if tool.OfTool != nil && tool.OfTool.CacheControl.Type == "ephemeral" {
			cached++
		}
	}
	for _, sys := range p.System {
		if sys.CacheControl.Type == "ephemeral" {
			cached++
		}
	}
	for _, msg := range p.Messages {
		for _, block := range msg.Content {
			if block.OfText != nil && block.OfText.CacheControl.Type == "ephemeral" {
				cached++
			}
		}
	}

	assert.Equal(t, 4, cached, "expected exactly 4 cached breakpoints")

	hasBreakpointWarning := false
	for _, w := range warnings {
		if w.Feature == "cacheControl" {
			hasBreakpointWarning = true
			break
		}
	}
	assert.True(t, hasBreakpointWarning, "expected warning about exceeding breakpoint limit")
}

func TestBuildParams_UserImageCacheControl(t *testing.T) {
	ccOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile,
				Data:            &provider.DataContent{Base64: "abc123"},
				MediaType:       "image/png",
				ProviderOptions: ccOpts,
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfImage)
	assert.EqualValues(t, "ephemeral", block.OfImage.CacheControl.Type)
}

func TestBuildParams_UserFileCacheControl(t *testing.T) {
	ccOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral", "ttl": "1h"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile,
				Data:            &provider.DataContent{Base64: "pdf-data"},
				MediaType:       "application/pdf",
				ProviderOptions: ccOpts,
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfDocument)
	assert.EqualValues(t, "ephemeral", block.OfDocument.CacheControl.Type)
	assert.EqualValues(t, "1h", block.OfDocument.CacheControl.TTL)
}

func TestBuildParams_ToolCallPartCacheControl(t *testing.T) {
	ccOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(
				provider.ContentPart{Type: provider.ContentPartTypeToolCall,
					ToolCallID:      "call_1",
					ToolName:        "search",
					Input:           json.RawMessage(`{"q":"test"}`),
					ProviderOptions: ccOpts,
				},
			),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfToolUse)
	assert.EqualValues(t, "ephemeral", block.OfToolUse.CacheControl.Type)
}

func TestBuildParams_ToolResultPartCacheControl(t *testing.T) {
	ccOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID:      "call_1",
				ToolName:        "search",
				Output:          &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "result"},
				ProviderOptions: ccOpts,
			}),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	block := p.Messages[0].Content[0]
	require.NotNil(t, block.OfToolResult)
	assert.EqualValues(t, "ephemeral", block.OfToolResult.CacheControl.Type)
}

// TestBuildParams_ToolResultIsError mirrors upstream
// `convert-to-anthropic-prompt.ts:467-475`: tool_result blocks must carry
// `is_error: true` when the output is an error variant, and must omit the
// field otherwise (upstream emits `undefined`).
func TestBuildParams_ToolResultIsError(t *testing.T) {
	tests := []struct {
		name           string
		output         *provider.ToolResultOutput
		expectIsErrSet bool
		expectIsErrVal bool
	}{
		{
			name:           "error-text sets is_error=true",
			output:         &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "boom"},
			expectIsErrSet: true,
			expectIsErrVal: true,
		},
		{
			name:           "error-json sets is_error=true",
			output:         &provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"err":"x"}`)},
			expectIsErrSet: true,
			expectIsErrVal: true,
		},
		{
			name:           "text output omits is_error",
			output:         &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"},
			expectIsErrSet: false,
		},
		{
			name:           "json output omits is_error",
			output:         &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)},
			expectIsErrSet: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := provider.CallOptions{
				Prompt: []provider.Message{
					provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolResult,
						ToolCallID: "call_1",
						ToolName:   "search",
						Output:     tc.output,
					}),
				},
			}

			p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
			require.NoError(t, err)
			require.Len(t, p.Messages, 1)
			require.Len(t, p.Messages[0].Content, 1)
			block := p.Messages[0].Content[0].OfToolResult
			require.NotNil(t, block)

			if tc.expectIsErrSet {
				require.True(t, block.IsError.Valid(), "is_error should be set")
				assert.Equal(t, tc.expectIsErrVal, block.IsError.Value)
			} else {
				assert.False(t, block.IsError.Valid(), "is_error should be omitted (zero value)")
			}
		})
	}
}

func TestBuildParams_ReasoningBlockNonCacheable(t *testing.T) {
	ccOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(
				provider.ContentPart{Type: provider.ContentPartTypeReasoning,
					Text:            "thinking...",
					ProviderOptions: ccOpts,
				},
				provider.TextPart("answer"),
			),
		},
	}

	_, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	hasWarning := false
	for _, w := range warnings {
		if w.Feature == "cacheControl" {
			hasWarning = true
			break
		}
	}
	assert.True(t, hasWarning, "expected warning about cache_control on thinking block")
}

func TestBuildParams_AssistantLastPartCascade(t *testing.T) {
	msgOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.Message{Role: provider.RoleAssistant,
				Content: []provider.ContentPart{
					provider.TextPart("first"),
					provider.ToolCallPart("call_1", "search", json.RawMessage(`{}`)),
				},
				ProviderOptions: msgOpts,
			},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	first := p.Messages[0].Content[0].OfText
	assert.NotEqualValues(t, "ephemeral", first.CacheControl.Type, "first part should NOT inherit message-level cache_control")

	last := p.Messages[0].Content[1].OfToolUse
	assert.EqualValues(t, "ephemeral", last.CacheControl.Type, "last tool_use part should inherit message-level cache_control")
}

func TestBuildParams_ToolMessageLastPartCascade(t *testing.T) {
	msgOpts := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.Message{Role: provider.RoleTool,
				Content: []provider.ContentPart{
					provider.ContentPart{Type: provider.ContentPartTypeToolResult, ToolCallID: "c1", ToolName: "t1", Output: &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r1"}},
					provider.ContentPart{Type: provider.ContentPartTypeToolResult, ToolCallID: "c2", ToolName: "t2", Output: &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r2"}},
				},
				ProviderOptions: msgOpts,
			},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	first := p.Messages[0].Content[0].OfToolResult
	assert.NotEqualValues(t, "ephemeral", first.CacheControl.Type, "first tool result should NOT inherit message-level cache_control")

	last := p.Messages[0].Content[1].OfToolResult
	assert.EqualValues(t, "ephemeral", last.CacheControl.Type, "last tool result should inherit message-level cache_control")
}

func TestConvertTools_WebSearchWithArgs(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.web_search_20250305",
			Args: map[string]json.RawMessage{
				"maxUses":        json.RawMessage(`5`),
				"allowedDomains": json.RawMessage(`["example.com","test.org"]`),
				"blockedDomains": json.RawMessage(`["spam.com"]`),
			},
		},
	}

	v := &cacheControlValidator{}
	result, warnings, _ := convertTools(v, tools, false)
	assert.Len(t, warnings, 0)
	require.Len(t, result, 1)
	ws := result[0].OfWebSearchTool20250305
	require.NotNil(t, ws)
	assert.True(t, ws.MaxUses.Valid(), "MaxUses should be valid")
	assert.Equal(t, int64(5), ws.MaxUses.Value)
	assert.Equal(t, []string{"example.com", "test.org"}, ws.AllowedDomains)
	assert.Equal(t, []string{"spam.com"}, ws.BlockedDomains)
}

func TestConvertTools_WebSearchNoArgs(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.web_search_20250305",
		},
	}

	v := &cacheControlValidator{}
	result, warnings, _ := convertTools(v, tools, false)
	assert.Len(t, warnings, 0)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfWebSearchTool20250305)
}

func TestConvertTools_ToolSearchBm25(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.tool_search_bm25_20251119",
		},
	}

	v := &cacheControlValidator{}
	result, warnings, _ := convertTools(v, tools, false)
	assert.Len(t, warnings, 0)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfToolSearchToolBm25_20251119)
}

func TestConvertTools_ToolSearchRegex(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.tool_search_regex_20251119",
		},
	}

	v := &cacheControlValidator{}
	result, warnings, _ := convertTools(v, tools, false)
	assert.Len(t, warnings, 0)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfToolSearchToolRegex20251119)
}

func TestConvertTools_UnrecognizedProviderDefined(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.unknown_tool",
		},
	}

	v := &cacheControlValidator{}
	result, warnings, _ := convertTools(v, tools, false)
	assert.Len(t, result, 0)
	require.Len(t, warnings, 1)
	assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
}

func TestConvertTools_MixedFunctionAndProviderDefined(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeFunction,
			Name:        "search",
			Description: "Search the web",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.web_search_20250305",
		},
	}

	v := &cacheControlValidator{}
	result, warnings, _ := convertTools(v, tools, false)
	assert.Len(t, warnings, 0)
	require.Len(t, result, 2)
	assert.NotNil(t, result[0].OfTool, "first tool should be OfTool (function)")
	assert.NotNil(t, result[1].OfWebSearchTool20250305, "second tool should be OfWebSearchTool20250305")
}

func TestConvertTools_FunctionToolProducesOfTool(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeFunction,
			Name:        "search",
			Description: "Search the web",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}

	v := &cacheControlValidator{}
	result, warnings, _ := convertTools(v, tools, false)
	assert.Len(t, warnings, 0)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfTool)
	assert.Equal(t, "search", result[0].OfTool.Name)
}

func TestBuildParams_ProviderOptions_MCPServers(t *testing.T) {
	t.Run("single_server_all_fields", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{
					"mcpServers": [{
						"name": "my-server",
						"url": "https://mcp.example.com",
						"authorizationToken": "token123",
						"toolConfiguration": {
							"enabled": true,
							"allowedTools": ["tool_a", "tool_b"]
						}
					}]
				}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.MCPServers, 1)
		srv := p.MCPServers[0]
		assert.Equal(t, "my-server", srv.Name)
		assert.Equal(t, "https://mcp.example.com", srv.URL)
		assert.True(t, srv.AuthorizationToken.Valid())
		assert.Equal(t, "token123", srv.AuthorizationToken.Value)
		assert.True(t, srv.ToolConfiguration.Enabled.Valid())
		assert.True(t, srv.ToolConfiguration.Enabled.Value)
		assert.Equal(t, []string{"tool_a", "tool_b"}, srv.ToolConfiguration.AllowedTools)
		assert.Contains(t, p.Betas, "mcp-client-2025-04-04")
	})

	t.Run("multiple_servers", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{
					"mcpServers": [
						{"name": "server-1", "url": "https://one.example.com"},
						{"name": "server-2", "url": "https://two.example.com"}
					]
				}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.MCPServers, 2)
		assert.Equal(t, "server-1", p.MCPServers[0].Name)
		assert.Equal(t, "server-2", p.MCPServers[1].Name)
	})

	t.Run("minimal_fields", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{
					"mcpServers": [{"name": "basic", "url": "https://mcp.example.com"}]
				}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.MCPServers, 1)
		assert.Equal(t, "basic", p.MCPServers[0].Name)
		assert.False(t, p.MCPServers[0].AuthorizationToken.Valid())
	})

	t.Run("no_servers_no_beta", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.Empty(t, p.MCPServers)
		for _, b := range p.Betas {
			assert.NotEqual(t, "mcp-client-2025-04-04", string(b))
		}
	})

	t.Run("beta_dedup", func(t *testing.T) {
		opts := provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{
					"mcpServers": [{"name": "s", "url": "https://example.com"}],
					"betas": ["mcp-client-2025-04-04"]
				}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		count := 0
		for _, b := range p.Betas {
			if b == "mcp-client-2025-04-04" {
				count++
			}
		}
		assert.Equal(t, 1, count, "expected 1 mcp beta, got %d (betas: %v)", count, p.Betas)
	})
}

func TestBuildParams_MCPToolCallRoundTrip(t *testing.T) {
	t.Run("mcp_tool_call_in_assistant_message", func(t *testing.T) {
		mcpOpts := makeProviderOpts(`{"type": "mcp-tool-use", "serverName": "my-server"}`)
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.ContentPart{Type: provider.ContentPartTypeToolCall,
						ToolCallID:      "tc_1",
						ToolName:        "remote_search",
						Input:           json.RawMessage(`{"q":"hello"}`),
						ProviderOptions: mcpOpts,
					},
				),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.Messages, 1)
		require.Len(t, p.Messages[0].Content, 1)
		block := p.Messages[0].Content[0]
		require.NotNil(t, block.OfMCPToolUse, "expected OfMCPToolUse block")
		assert.Nil(t, block.OfToolUse, "should NOT emit OfToolUse for MCP")
		assert.Equal(t, "tc_1", block.OfMCPToolUse.ID)
		assert.Equal(t, "remote_search", block.OfMCPToolUse.Name)
		assert.Equal(t, "my-server", block.OfMCPToolUse.ServerName)
	})

	t.Run("mcp_tool_call_without_server_name_skips_with_warning", func(t *testing.T) {
		mcpOpts := makeProviderOpts(`{"type": "mcp-tool-use"}`)
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.ContentPart{Type: provider.ContentPartTypeToolCall,
						ToolCallID:      "tc_1",
						ToolName:        "remote_search",
						Input:           json.RawMessage(`{"q":"hello"}`),
						ProviderOptions: mcpOpts,
					},
				),
			},
		}

		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		require.Len(t, p.Messages, 1)
		assert.Empty(t, p.Messages[0].Content)
		require.Len(t, warnings, 1)
		assert.Equal(t, provider.WarnOther, warnings[0].Type)
		assert.Contains(t, warnings[0].Message, "server name is required")
	})

	t.Run("mcp_tool_result_in_tool_message", func(t *testing.T) {
		mcpOpts := makeProviderOpts(`{"type": "mcp-tool-use", "serverName": "my-server"}`)
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.ContentPart{Type: provider.ContentPartTypeToolCall,
						ToolCallID:      "tc_1",
						ToolName:        "remote_search",
						Input:           json.RawMessage(`{"q":"hello"}`),
						ProviderOptions: mcpOpts,
					},
				),
				provider.NewToolMessage(provider.ToolResultPart("tc_1", "remote_search", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`"result data"`)})),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.Messages, 2)

		require.Len(t, p.Messages[1].Content, 1)
		block := p.Messages[1].Content[0]
		require.NotNil(t, block.OfMCPToolResult, "expected OfMCPToolResult block")
		assert.Nil(t, block.OfToolResult, "should NOT emit OfToolResult for MCP")
		assert.Equal(t, "tc_1", block.OfMCPToolResult.ToolUseID)
	})

	t.Run("regular_tools_unaffected", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.ToolCallPart("call_1", "search", json.RawMessage(`{"q":"test"}`)),
				),
				provider.NewToolMessage(provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "result"})),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assistantBlock := p.Messages[0].Content[0]
		require.NotNil(t, assistantBlock.OfToolUse, "regular tool should use OfToolUse")
		assert.Nil(t, assistantBlock.OfMCPToolUse)

		toolBlock := p.Messages[1].Content[0]
		require.NotNil(t, toolBlock.OfToolResult, "regular tool result should use OfToolResult")
		assert.Nil(t, toolBlock.OfMCPToolResult)
	})

	t.Run("mixed_mcp_and_regular", func(t *testing.T) {
		mcpOpts := makeProviderOpts(`{"type": "mcp-tool-use", "serverName": "srv"}`)
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.ToolCallPart("call_1", "local_search", json.RawMessage(`{}`)),
					provider.ContentPart{Type: provider.ContentPartTypeToolCall,
						ToolCallID:      "tc_1",
						ToolName:        "remote_tool",
						Input:           json.RawMessage(`{}`),
						ProviderOptions: mcpOpts,
					},
				),
				provider.NewToolMessage(
					provider.ToolResultPart("call_1", "local_search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "local result"}),
					provider.ToolResultPart("tc_1", "remote_tool", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`"remote result"`)}),
				),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assistantBlocks := p.Messages[0].Content
		require.Len(t, assistantBlocks, 2)
		assert.NotNil(t, assistantBlocks[0].OfToolUse)
		assert.NotNil(t, assistantBlocks[1].OfMCPToolUse)

		toolBlocks := p.Messages[1].Content
		require.Len(t, toolBlocks, 2)
		assert.NotNil(t, toolBlocks[0].OfToolResult)
		assert.NotNil(t, toolBlocks[1].OfMCPToolResult)
	})
}

func TestBuildParams_ProviderDefinedToolWarningsThreaded(t *testing.T) {
	opts := provider.CallOptions{
		Tools: []provider.Tool{
			provider.Tool{Type: provider.ToolTypeProvider,
				ID: "anthropic.unknown_tool",
			},
		},
	}

	_, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	found := false
	for _, w := range warnings {
		if w.Type == provider.WarnUnsupported {
			found = true
		}
	}
	assert.True(t, found, "expected unsupported-tool warning to be threaded through buildParams")
}

func TestConvertTools_ToolProviderOptions(t *testing.T) {
	t.Run("DeferLoading", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:            "search",
				Description:     "Search the web",
				InputSchema:     json.RawMessage(`{"type":"object","properties":{}}`),
				ProviderOptions: makeProviderOpts(`{"deferLoading": true}`),
			},
		}

		v := &cacheControlValidator{}
		result, _, _ := convertTools(v, tools, false)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].OfTool)
		assert.True(t, result[0].OfTool.DeferLoading.Valid())
		assert.True(t, result[0].OfTool.DeferLoading.Value)
	})

	t.Run("AllowedCallers", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:            "search",
				Description:     "Search the web",
				InputSchema:     json.RawMessage(`{"type":"object","properties":{}}`),
				ProviderOptions: makeProviderOpts(`{"allowedCallers": ["direct", "code_execution_20250825"]}`),
			},
		}

		v := &cacheControlValidator{}
		result, _, _ := convertTools(v, tools, false)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].OfTool)
		assert.Equal(t, []string{"direct", "code_execution_20250825"}, result[0].OfTool.AllowedCallers)
	})

	t.Run("EagerInputStreaming", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:            "search",
				Description:     "Search the web",
				InputSchema:     json.RawMessage(`{"type":"object","properties":{}}`),
				ProviderOptions: makeProviderOpts(`{"eagerInputStreaming": true}`),
			},
		}

		v := &cacheControlValidator{}
		result, _, _ := convertTools(v, tools, false)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].OfTool)
		assert.True(t, result[0].OfTool.EagerInputStreaming.Valid())
		assert.True(t, result[0].OfTool.EagerInputStreaming.Value)
	})

	t.Run("DefaultEagerInputStreaming", func(t *testing.T) {
		tools := []provider.Tool{{
			Type:        provider.ToolTypeFunction,
			Name:        "search",
			Description: "Search the web",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		}}

		v := &cacheControlValidator{}
		result, _, _ := convertTools(v, tools, true)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].OfTool)
		assert.True(t, result[0].OfTool.EagerInputStreaming.Valid())
		assert.True(t, result[0].OfTool.EagerInputStreaming.Value)
	})

	t.Run("AllThreeOptions", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:            "search",
				Description:     "Search the web",
				InputSchema:     json.RawMessage(`{"type":"object","properties":{}}`),
				ProviderOptions: makeProviderOpts(`{"deferLoading": true, "allowedCallers": ["direct"], "eagerInputStreaming": true}`),
			},
		}

		v := &cacheControlValidator{}
		result, _, _ := convertTools(v, tools, false)
		require.Len(t, result, 1)
		tp := result[0].OfTool
		require.NotNil(t, tp)
		assert.True(t, tp.DeferLoading.Valid())
		assert.True(t, tp.DeferLoading.Value)
		assert.Equal(t, []string{"direct"}, tp.AllowedCallers)
		assert.True(t, tp.EagerInputStreaming.Valid())
		assert.True(t, tp.EagerInputStreaming.Value)
	})

	t.Run("NoAnthropicKey", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			},
		}

		v := &cacheControlValidator{}
		result, _, _ := convertTools(v, tools, false)
		require.Len(t, result, 1)
		tp := result[0].OfTool
		require.NotNil(t, tp)
		assert.False(t, tp.DeferLoading.Valid())
		assert.Nil(t, tp.AllowedCallers)
		assert.False(t, tp.EagerInputStreaming.Valid())
	})

	t.Run("MalformedAnthropicJSON", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{invalid json`)},
				},
			},
		}

		v := &cacheControlValidator{}
		result, warnings, _ := convertTools(v, tools, false)
		require.Len(t, result, 1)
		tp := result[0].OfTool
		require.NotNil(t, tp)
		assert.False(t, tp.DeferLoading.Valid())
		assert.Nil(t, tp.AllowedCallers)
		assert.False(t, tp.EagerInputStreaming.Valid())
		require.Len(t, warnings, 1)
		assert.Equal(t, provider.WarnOther, warnings[0].Type)
		assert.Contains(t, warnings[0].Message, "search")
	})

	t.Run("InputExamples", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
				InputExamples: []provider.InputExample{
					{Input: json.RawMessage(`{"x": 1}`)},
					{Input: json.RawMessage(`{"x": 2}`)},
				},
			},
		}

		v := &cacheControlValidator{}
		result, _, _ := convertTools(v, tools, false)
		require.Len(t, result, 1)
		tp := result[0].OfTool
		require.NotNil(t, tp)
		require.Len(t, tp.InputExamples, 2)
		assert.Equal(t, float64(1), tp.InputExamples[0]["x"])
		assert.Equal(t, float64(2), tp.InputExamples[1]["x"])
	})

	t.Run("BetaAutoDetection_InputExamples", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
				InputExamples: []provider.InputExample{
					{Input: json.RawMessage(`{"q": "test"}`)},
				},
			},
		}

		v := &cacheControlValidator{}
		_, _, betas := convertTools(v, tools, false)
		assert.Contains(t, betas, "advanced-tool-use-2025-11-20")
	})

	t.Run("BetaAutoDetection_AllowedCallers", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:            "search",
				Description:     "Search the web",
				InputSchema:     json.RawMessage(`{"type":"object","properties":{}}`),
				ProviderOptions: makeProviderOpts(`{"allowedCallers": ["direct"]}`),
			},
		}

		v := &cacheControlValidator{}
		_, _, betas := convertTools(v, tools, false)
		assert.Contains(t, betas, "advanced-tool-use-2025-11-20")
	})

	t.Run("BetaDeduplication", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeFunction,
				Name:        "search",
				Description: "Search the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
				InputExamples: []provider.InputExample{
					{Input: json.RawMessage(`{"q": "test"}`)},
				},
				ProviderOptions: makeProviderOpts(`{"allowedCallers": ["direct"]}`),
			},
		}

		v := &cacheControlValidator{}
		_, _, betas := convertTools(v, tools, false)
		count := 0
		for _, b := range betas {
			if b == "advanced-tool-use-2025-11-20" {
				count++
			}
		}
		assert.Equal(t, 1, count, "expected exactly 1 advanced-tool-use beta, got %d", count)
	})

	t.Run("ProviderDefinedToolIgnoresProviderOptions", func(t *testing.T) {
		tools := []provider.Tool{
			provider.Tool{Type: provider.ToolTypeProvider,
				ID:   "anthropic.web_search_20250305",
				Name: "web_search",
			},
		}

		v := &cacheControlValidator{}
		result, _, betas := convertTools(v, tools, false)
		require.Len(t, result, 1)
		assert.NotNil(t, result[0].OfWebSearchTool20250305, "should still convert as web search tool")
		assert.Nil(t, result[0].OfTool, "should not be converted as a function tool")
		assert.Empty(t, betas, "provider tool should not trigger beta auto-detection")
	})
}

func hasUnsupportedWarning(warnings []provider.Warning, feature string) bool {
	for _, w := range warnings {
		if w.Type == provider.WarnUnsupported && w.Feature == feature {
			return true
		}
	}
	return false
}

func TestBuildParams_UnsupportedContentWarnings(t *testing.T) {
	t.Run("CustomContentPart in assistant message", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.TextPart("hello"),
					provider.ContentPart{Type: provider.ContentPartTypeCustom, Kind: "anthropic.cache-control"},
				),
			},
		}

		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.Messages, 1)
		require.Len(t, p.Messages[0].Content, 1, "custom content should be skipped")
		require.NotNil(t, p.Messages[0].Content[0].OfText)
		assert.True(t, hasUnsupportedWarning(warnings, "customContent"), "expected unsupported warning for CustomContentPart")
	})

	t.Run("ReasoningFileContentPart in assistant message", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.TextPart("hello"),
					provider.ContentPart{Type: provider.ContentPartTypeReasoningFile,
						Data:      &provider.DataContent{Base64: "abc"},
						MediaType: "image/png",
					},
				),
			},
		}

		_, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		assert.True(t, hasUnsupportedWarning(warnings, "reasoningFile"), "expected unsupported warning for ReasoningFileContentPart")
	})

	t.Run("ToolApprovalResponseContentPart in tool message", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewToolMessage(
					provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "result"}),
					provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse,
						ApprovalID: "apr_123",
						Approved:   boolPtr(true),
					},
				),
			},
		}

		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.Messages, 1)
		require.Len(t, p.Messages[0].Content, 1, "approval response should be skipped")
		require.NotNil(t, p.Messages[0].Content[0].OfToolResult)
		assert.False(t, hasUnsupportedWarning(warnings, "toolApprovalResponse"), "local approval responses should not warn")
	})

	t.Run("ToolApprovalRequestContentPart in assistant message", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.TextPart("hello"),
					provider.ToolApprovalRequestPart("apr_123", "call_1", false),
				),
			},
		}

		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.Len(t, p.Messages, 1)
		require.Len(t, p.Messages[0].Content, 1, "approval request should be stripped")
		require.NotNil(t, p.Messages[0].Content[0].OfText)
		assert.False(t, hasUnsupportedWarning(warnings, "toolApprovalRequest"), "local approval requests should not warn")
	})
}

func TestBuildParams_StructuredOutput(t *testing.T) {
	testSchema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"integer"}},"required":["name","age"]}`)

	t.Run("NativeMode_SupportedModel", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
		}

		p, _, warnings, br, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool, "should use native mode, not tool fallback")
		assert.Empty(t, warnings)
		assert.NotEmpty(t, p.OutputConfig.Format.Schema, "OutputConfig.Format should be set")
		assert.Empty(t, p.Tools, "no synthetic tool should be added in native mode")
	})

	t.Run("NativeMode_PreservesEffort", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"effort":"high"}`)},
			},
		}

		p, _, _, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		assert.NotEmpty(t, p.OutputConfig.Format.Schema, "Format should be set")
		assert.Equal(t, "high", string(p.OutputConfig.Effort), "Effort should be preserved alongside Format")
	})

	t.Run("NativeMode_ToolChoiceUnchanged", func(t *testing.T) {
		tc := provider.ToolChoice{Type: provider.ToolChoiceAuto}
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			ToolChoice: &tc,
			Tools: []provider.Tool{
				provider.Tool{Type: provider.ToolTypeFunction, Name: "search", Description: "Search", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
			},
		}

		p, _, _, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		assert.NotNil(t, p.ToolChoice.OfAuto, "tool choice should remain auto")
		assert.Nil(t, p.ToolChoice.OfAny, "should not be overridden to required")
		assert.Contains(t, p.Betas, sdk.AnthropicBeta("structured-outputs-2025-11-13"),
			"structured outputs beta should be added when native mode + tools")
	})

	t.Run("VertexUsesToolFallback", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			Tools: []provider.Tool{{
				Type:        provider.ToolTypeFunction,
				Name:        "search",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			}},
		}

		p, _, warnings, br, err := buildParamsWithCapabilities("claude-sonnet-4-6", opts, false, vertexProviderCapabilities)
		require.NoError(t, err)

		assert.True(t, br.usesJsonResponseTool)
		assert.Empty(t, warnings)
		assert.Empty(t, p.OutputConfig.Format.Schema)
		require.Len(t, p.Tools, 2)
		assert.Equal(t, "search", p.Tools[0].OfTool.Name)
		assert.Equal(t, jsonResponseToolName, p.Tools[1].OfTool.Name)
		require.NotNil(t, p.ToolChoice.OfAny)
		assert.True(t, p.ToolChoice.OfAny.DisableParallelToolUse.Value)
		assert.NotContains(t, p.Betas, sdk.AnthropicBeta("structured-outputs-2025-11-13"))
	})

	t.Run("NativeMode_NoBetaWithoutTools", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
		}

		p, _, _, br, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		for _, b := range p.Betas {
			assert.NotEqual(t, sdk.AnthropicBeta("structured-outputs-2025-11-13"), b,
				"structured outputs beta should not be added without function tools")
		}
	})

	t.Run("NativeMode_NoBetaWithOnlyProviderTools", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			Tools: []provider.Tool{
				provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305"},
			},
		}

		p, _, _, br, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		for _, b := range p.Betas {
			assert.NotEqual(t, sdk.AnthropicBeta("structured-outputs-2025-11-13"), b,
				"structured outputs beta should not be added with only provider tools")
		}
	})

	t.Run("ToolFallback_UnsupportedModel", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
		}

		p, _, warnings, br, err := buildParams("claude-3-haiku", opts, false)
		require.NoError(t, err)

		assert.True(t, br.usesJsonResponseTool, "should use tool fallback for unsupported model")
		assert.Empty(t, warnings)

		require.Len(t, p.Tools, 1)
		require.NotNil(t, p.Tools[0].OfTool)
		assert.Equal(t, "json", p.Tools[0].OfTool.Name)
		assert.True(t, p.Tools[0].OfTool.Description.Valid())
		assert.Equal(t, "Respond with a JSON object.", p.Tools[0].OfTool.Description.Value)

		require.NotNil(t, p.ToolChoice.OfAny, "tool choice should be required")
		assert.True(t, p.ToolChoice.OfAny.DisableParallelToolUse.Valid())
		assert.True(t, p.ToolChoice.OfAny.DisableParallelToolUse.Value, "parallel tool use should be disabled")
	})

	t.Run("ToolFallback_ExplicitParallelUseWarning", func(t *testing.T) {
		disableParallelToolUse := false
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: testSchema},
			ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
				StructuredOutputMode:   StructuredOutputJSONTool,
				DisableParallelToolUse: &disableParallelToolUse,
			}),
		}

		p, _, warnings, br, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)
		require.True(t, br.usesJsonResponseTool)
		require.Len(t, warnings, 1)
		assert.Equal(t, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "providerOptions.anthropic.disableParallelToolUse",
			Details: "`disableParallelToolUse: false` is ignored when using the JSON response tool. Parallel tool use is disabled to ensure a single coherent JSON tool call.",
		}, warnings[0])
		require.NotNil(t, p.ToolChoice.OfAny)
		assert.True(t, p.ToolChoice.OfAny.DisableParallelToolUse.Value)
	})

	t.Run("ToolFallback_StreamingDefaultsEagerOnJsonTool", func(t *testing.T) {
		// Upstream calls prepareTools once with [...tools, jsonResponseTool]
		// and the same defaultEagerInputStreaming flag, so the synthetic JSON
		// fallback tool gets eager_input_streaming: true on streaming requests
		// just like any user-provided function tool
		// (anthropic-language-model.ts:519-545, test snapshot in PR #14542).
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
		}

		p, _, _, br, err := buildParams("claude-3-haiku", opts, true)
		require.NoError(t, err)
		require.True(t, br.usesJsonResponseTool)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.Equal(t, "json", tp.Name)
		assert.True(t, tp.EagerInputStreaming.Valid(),
			"streaming request must default eager_input_streaming on the JSON fallback tool")
		assert.True(t, tp.EagerInputStreaming.Value)
	})

	t.Run("ToolFallback_ToolStreamingFalseSuppressesEagerOnJsonTool", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(`{"toolStreaming": false}`),
				},
			},
		}

		p, _, _, br, err := buildParams("claude-3-haiku", opts, true)
		require.NoError(t, err)
		require.True(t, br.usesJsonResponseTool)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.False(t, tp.EagerInputStreaming.Valid(),
			"ToolStreaming=false must suppress the default on the JSON fallback tool")
	})

	t.Run("ToolFallback_GenerateDoesNotDefaultEagerOnJsonTool", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
		}

		p, _, _, br, err := buildParams("claude-3-haiku", opts, false)
		require.NoError(t, err)
		require.True(t, br.usesJsonResponseTool)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.False(t, tp.EagerInputStreaming.Valid(),
			"non-streaming requests must not set eager_input_streaming on the JSON fallback tool")
	})

	t.Run("ToolFallback_AppendsToExistingTools", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			Tools: []provider.Tool{
				provider.Tool{Type: provider.ToolTypeFunction, Name: "search", Description: "Search", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
			},
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto},
		}

		p, _, _, br, err := buildParams("claude-3-haiku", opts, false)
		require.NoError(t, err)

		assert.True(t, br.usesJsonResponseTool)
		require.Len(t, p.Tools, 2, "should have user tool + json tool")
		assert.Equal(t, "search", p.Tools[0].OfTool.Name)
		assert.Equal(t, "json", p.Tools[1].OfTool.Name)
		require.NotNil(t, p.ToolChoice.OfAny, "tool choice should be overridden to required")
		for _, b := range p.Betas {
			assert.NotEqual(t, sdk.AnthropicBeta("structured-outputs-2025-11-13"), b,
				"structured outputs beta should not be added for tool fallback path")
		}
	})

	t.Run("ToolFallback_OverridesNoneToolChoice", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: testSchema,
			},
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceNone},
		}

		p, _, _, br, err := buildParams("claude-3-haiku", opts, false)
		require.NoError(t, err)

		assert.True(t, br.usesJsonResponseTool)
		require.NotNil(t, p.ToolChoice.OfAny, "none should be overridden to required")
	})

	t.Run("SchemalessJSON_Warning", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON},
		}

		_, _, warnings, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		require.Len(t, warnings, 1)
		assert.Equal(t, "responseFormat", warnings[0].Feature)
		assert.Contains(t, warnings[0].Details, "schemaless")
	})

	t.Run("TextFormat_NoOp", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText},
		}

		p, _, warnings, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		assert.Empty(t, warnings)
		assert.Empty(t, p.OutputConfig.Format.Schema)
		assert.Empty(t, p.Tools)
	})

	t.Run("UnknownFormat_Warning", func(t *testing.T) {
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatType("future")},
		}

		_, _, warnings, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		require.Len(t, warnings, 1)
		assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
		assert.Equal(t, "responseFormat", warnings[0].Feature)
		assert.Contains(t, warnings[0].Details, "future")
	})

	t.Run("NilResponseFormat_NoOp", func(t *testing.T) {
		opts := provider.CallOptions{}

		p, _, warnings, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		assert.Empty(t, warnings)
		assert.Empty(t, p.OutputConfig.Format.Schema)
		assert.Empty(t, p.Tools)
	})

	t.Run("NativeMode_SanitizesSchema", func(t *testing.T) {
		// applyResponseFormat bypasses the SDK's BetaJSONSchemaOutputFormat
		// helper to avoid a second, lossy schema transform; the schema set on
		// OutputConfig.Format SHALL therefore reflect exactly what
		// sanitizeJSONSchema produced.
		raw := []byte(`{"type":"object","properties":{"slug":{"type":"string","minLength":1,"maxLength":20,"pattern":"^[a-z0-9-]+$"}},"required":["slug"]}`)
		rawCopy := make([]byte, len(raw))
		copy(rawCopy, raw)

		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: json.RawMessage(raw),
			},
		}

		p, _, _, br, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		assert.False(t, br.usesJsonResponseTool)
		require.NotEmpty(t, p.OutputConfig.Format.Schema)

		// anthropic-sdk-go v1.43.0 changed BetaJSONOutputFormatParam.Schema
		// from map[string]any to any; assert back to a map before indexing.
		schema, ok := p.OutputConfig.Format.Schema.(map[string]any)
		require.True(t, ok, "Format.Schema must be a map[string]any")

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok, "properties should be a map")
		slug, ok := props["slug"].(map[string]any)
		require.True(t, ok, "slug should be a map")

		_, hasMinLength := slug["minLength"]
		_, hasMaxLength := slug["maxLength"]
		_, hasPattern := slug["pattern"]
		assert.False(t, hasMinLength, "minLength should be stripped")
		assert.False(t, hasMaxLength, "maxLength should be stripped")
		assert.False(t, hasPattern, "pattern should be stripped")
		assert.Equal(t, "min length: 1; max length: 20; pattern: ^[a-z0-9-]+$.", slug["description"])
		assert.Equal(t, false, schema["additionalProperties"], "additionalProperties forced to false on object nodes")

		assert.Equal(t, rawCopy, []byte(raw), "caller's ResponseFormat.Schema bytes must not be mutated")
	})

	t.Run("NativeMode_PreservesDefinitionsAndRefIntegrity", func(t *testing.T) {
		// Regression: the SDK's transformSchema would strip the older
		// `definitions` keyword into description text, breaking $ref pointers
		// like #/definitions/Foo. Our direct construction must keep
		// definitions structured and resolvable.
		raw := json.RawMessage(`{
			"type": "object",
			"definitions": {
				"Foo": {"type": "string", "minLength": 1}
			},
			"properties": {
				"foo": {"$ref": "#/definitions/Foo"}
			}
		}`)
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: raw},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		schema, ok := p.OutputConfig.Format.Schema.(map[string]any)
		require.True(t, ok, "Format.Schema must be a map[string]any")

		defs, ok := schema["definitions"].(map[string]any)
		require.True(t, ok, "definitions must remain a structured field")
		foo, ok := defs["Foo"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "string", foo["type"])
		assert.Equal(t, "min length: 1.", foo["description"], "constraints inside definitions are sanitized to description")

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		fooProp, ok := props["foo"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "#/definitions/Foo", fooProp["$ref"], "$ref must remain pointing at structured definitions")
	})

	t.Run("NativeMode_PreservesRootAllOf", func(t *testing.T) {
		// Regression: the SDK helper returns nil for a root schema whose only
		// composition is allOf (no type, no anyOf, no oneOf). Our direct
		// construction must preserve the root and the allOf branches.
		raw := json.RawMessage(`{
			"allOf": [
				{"type": "object", "properties": {"a": {"type": "string"}}},
				{"type": "object", "properties": {"b": {"type": "integer"}}}
			]
		}`)
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: raw},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		require.NotEmpty(t, p.OutputConfig.Format.Schema, "root allOf-only schema must not collapse to nil")
		schema, ok := p.OutputConfig.Format.Schema.(map[string]any)
		require.True(t, ok, "Format.Schema must be a map[string]any")
		allOf, ok := schema["allOf"].([]any)
		require.True(t, ok, "allOf must remain on the root")
		require.Len(t, allOf, 2)

		first, ok := allOf[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "object", first["type"])
		assert.Equal(t, false, first["additionalProperties"], "object branches inside allOf get additionalProperties: false")
	})

	t.Run("NativeMode_PreservesMetadataAndValueConstraints", func(t *testing.T) {
		// Regression: $schema, $id, enum, const, and default are upstream-
		// preserved keywords. The SDK helper would have moved them into a
		// Go-formatted description appendix; our direct construction keeps
		// them as structured fields.
		raw := json.RawMessage(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id": "https://example.com/color",
			"title": "Color",
			"type": "string",
			"enum": ["red", "green", "blue"],
			"const": "red",
			"default": "red"
		}`)
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: raw},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-5", opts, false)
		require.NoError(t, err)

		s, ok := p.OutputConfig.Format.Schema.(map[string]any)
		require.True(t, ok, "Format.Schema must be a map[string]any")
		assert.Equal(t, "http://json-schema.org/draft-07/schema#", s["$schema"])
		assert.Equal(t, "https://example.com/color", s["$id"])
		assert.Equal(t, "Color", s["title"])
		assert.Equal(t, "string", s["type"])
		assert.Equal(t, []any{"red", "green", "blue"}, s["enum"])
		assert.Equal(t, "red", s["const"])
		assert.Equal(t, "red", s["default"])
		_, hasDescription := s["description"]
		assert.False(t, hasDescription, "no description should be synthesized when no constraints are stripped")
	})

	t.Run("ToolFallback_DoesNotApplyOurSanitizer", func(t *testing.T) {
		// The Anthropic SDK runs its own `transformSchema` on tool input
		// schemas, so constraints will be stripped either way. What this test
		// verifies is that we do NOT additionally run our `sanitizeJSONSchema`
		// helper on the tool-fallback path (matching upstream behavior): our
		// sanitizer would synthesize an appendix in the form
		//   "min length: 1; max length: 20; pattern: ^[a-z0-9-]+$."
		// (camelCase split into space-separated lowercase, joined by `; `,
		// terminated with `.`). The SDK's own transform uses a distinct
		// `{minLength: 1, maxLength: 20, pattern: ^[a-z0-9-]+$}` form. If
		// either text is present, we know which path ran.
		raw := json.RawMessage(`{"type":"object","properties":{"slug":{"type":"string","minLength":1,"maxLength":20,"pattern":"^[a-z0-9-]+$"}}}`)
		opts := provider.CallOptions{
			ResponseFormat: &provider.ResponseFormat{
				Type:   provider.ResponseFormatJSON,
				Schema: raw,
			},
		}

		p, _, _, br, err := buildParams("claude-3-haiku", opts, false)
		require.NoError(t, err)
		assert.True(t, br.usesJsonResponseTool)

		// Round-trip the marshaled tool input schema so we inspect the wire
		// form regardless of where the SDK stashes the transformed schema
		// internally.
		require.Len(t, p.Tools, 1)
		require.NotNil(t, p.Tools[0].OfTool)
		wire, err := json.Marshal(p.Tools[0].OfTool.InputSchema)
		require.NoError(t, err)

		assert.NotContains(t, string(wire), "min length:",
			"our upstream-style sanitizer must NOT run on the tool-fallback path")
		assert.NotContains(t, string(wire), "max length:",
			"our upstream-style sanitizer must NOT run on the tool-fallback path")
	})
}

func TestConvertProviderTool_ComplexServerTools(t *testing.T) {
	t.Run("code_execution_20250522", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfCodeExecutionTool20250522)
		assert.Equal(t, []string{"code-execution-2025-05-22"}, betas)
	})

	t.Run("code_execution_20250825", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250825"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfCodeExecutionTool20250825)
		assert.Equal(t, []string{"code-execution-2025-08-25"}, betas)
	})

	t.Run("code_execution_20260120_no_beta", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20260120"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfCodeExecutionTool20260120)
		assert.Nil(t, betas)
	})

	t.Run("computer_20241022_with_args", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.computer_20241022",
			Args: map[string]json.RawMessage{
				"displayWidthPx":  json.RawMessage(`1920`),
				"displayHeightPx": json.RawMessage(`1080`),
				"displayNumber":   json.RawMessage(`1`),
			},
		}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfComputerUseTool20241022)
		assert.Equal(t, int64(1920), result.OfComputerUseTool20241022.DisplayWidthPx)
		assert.Equal(t, int64(1080), result.OfComputerUseTool20241022.DisplayHeightPx)
		assert.Equal(t, int64(1), result.OfComputerUseTool20241022.DisplayNumber.Value)
		assert.Equal(t, []string{"computer-use-2024-10-22"}, betas)
	})

	t.Run("computer_20251124_with_enable_zoom", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.computer_20251124",
			Args: map[string]json.RawMessage{
				"displayWidthPx":  json.RawMessage(`1920`),
				"displayHeightPx": json.RawMessage(`1080`),
				"enableZoom":      json.RawMessage(`true`),
			},
		}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfComputerUseTool20251124)
		assert.True(t, result.OfComputerUseTool20251124.EnableZoom.Value)
		assert.Equal(t, []string{"computer-use-2025-11-24"}, betas)
	})

	t.Run("text_editor_20241022", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.text_editor_20241022"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfTextEditor20241022)
		assert.Equal(t, []string{"computer-use-2024-10-22"}, betas)
	})

	t.Run("computer_20250124_with_args", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.computer_20250124",
			Args: map[string]json.RawMessage{
				"displayWidthPx":  json.RawMessage(`1920`),
				"displayHeightPx": json.RawMessage(`1080`),
				"displayNumber":   json.RawMessage(`2`),
			},
		}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfComputerUseTool20250124)
		assert.Equal(t, int64(1920), result.OfComputerUseTool20250124.DisplayWidthPx)
		assert.Equal(t, int64(1080), result.OfComputerUseTool20250124.DisplayHeightPx)
		assert.Equal(t, int64(2), result.OfComputerUseTool20250124.DisplayNumber.Value)
		assert.Equal(t, []string{"computer-use-2025-01-24"}, betas)
	})

	t.Run("text_editor_20250124", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.text_editor_20250124"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfTextEditor20250124)
		assert.Equal(t, []string{"computer-use-2025-01-24"}, betas)
	})

	t.Run("text_editor_20250429_uses_correct_beta", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.text_editor_20250429"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfTextEditor20250429)
		assert.Equal(t, []string{"computer-use-2025-01-24"}, betas)
	})

	t.Run("text_editor_20250728_with_maxCharacters", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.text_editor_20250728",
			Args: map[string]json.RawMessage{
				"maxCharacters": json.RawMessage(`50000`),
			},
		}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfTextEditor20250728)
		assert.Equal(t, int64(50000), result.OfTextEditor20250728.MaxCharacters.Value)
		assert.Nil(t, betas, "text_editor_20250728 should have no beta")
	})

	t.Run("bash_20241022", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.bash_20241022"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfBashTool20241022)
		assert.Equal(t, []string{"computer-use-2024-10-22"}, betas)
	})

	t.Run("bash_20250124", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.bash_20250124"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfBashTool20250124)
		assert.Equal(t, []string{"computer-use-2025-01-24"}, betas)
	})

	t.Run("memory_20250818", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.memory_20250818"}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfMemoryTool20250818)
		assert.Equal(t, []string{"context-management-2025-06-27"}, betas)
	})

	t.Run("web_fetch_20250910_with_args", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.web_fetch_20250910",
			Args: map[string]json.RawMessage{
				"maxUses":          json.RawMessage(`5`),
				"allowedDomains":   json.RawMessage(`["example.com"]`),
				"blockedDomains":   json.RawMessage(`["blocked.com"]`),
				"citations":        json.RawMessage(`{"enabled":true}`),
				"maxContentTokens": json.RawMessage(`10000`),
			},
		}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfWebFetchTool20250910)
		assert.Equal(t, int64(5), result.OfWebFetchTool20250910.MaxUses.Value)
		assert.Equal(t, []string{"example.com"}, result.OfWebFetchTool20250910.AllowedDomains)
		assert.Equal(t, []string{"blocked.com"}, result.OfWebFetchTool20250910.BlockedDomains)
		assert.True(t, result.OfWebFetchTool20250910.Citations.Enabled.Value)
		assert.Equal(t, int64(10000), result.OfWebFetchTool20250910.MaxContentTokens.Value)
		assert.Equal(t, []string{"web-fetch-2025-09-10"}, betas)
	})

	t.Run("web_fetch_20260209_with_args", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.web_fetch_20260209",
			Args: map[string]json.RawMessage{
				"maxUses":        json.RawMessage(`3`),
				"allowedDomains": json.RawMessage(`["docs.example.com"]`),
			},
		}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfWebFetchTool20260209)
		assert.Equal(t, int64(3), result.OfWebFetchTool20260209.MaxUses.Value)
		assert.Equal(t, []string{"docs.example.com"}, result.OfWebFetchTool20260209.AllowedDomains)
		assert.Equal(t, []string{"code-execution-web-tools-2026-02-09"}, betas)
	})

	t.Run("web_search_20260209_with_args", func(t *testing.T) {
		tool := provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.web_search_20260209",
			Args: map[string]json.RawMessage{
				"maxUses":        json.RawMessage(`10`),
				"allowedDomains": json.RawMessage(`["example.com"]`),
				"blockedDomains": json.RawMessage(`["blocked.com"]`),
				"userLocation":   json.RawMessage(`{"type":"approximate","city":"Berlin","country":"DE"}`),
			},
		}
		result, betas, warning := convertProviderTool(tool)
		require.Nil(t, warning)
		require.NotNil(t, result.OfWebSearchTool20260209)
		assert.Equal(t, int64(10), result.OfWebSearchTool20260209.MaxUses.Value)
		assert.Equal(t, []string{"example.com"}, result.OfWebSearchTool20260209.AllowedDomains)
		assert.Equal(t, []string{"blocked.com"}, result.OfWebSearchTool20260209.BlockedDomains)
		assert.Equal(t, "Berlin", result.OfWebSearchTool20260209.UserLocation.City.Value)
		assert.Equal(t, "DE", result.OfWebSearchTool20260209.UserLocation.Country.Value)
		assert.Equal(t, []string{"code-execution-web-tools-2026-02-09"}, betas)
	})
}

func TestConvertTools_BetasFromProviderTools(t *testing.T) {
	tools := []provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250825", Name: "code_exec"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.bash_20250124", Name: "my_bash"},
	}

	v := &cacheControlValidator{}
	_, _, betas := convertTools(v, tools, false)

	assert.Contains(t, betas, "code-execution-2025-08-25")
	assert.Contains(t, betas, "computer-use-2025-01-24")
}

// TestBuildParams_DefaultEagerInputStreaming covers the per-tool default for
// `eager_input_streaming` driven by the model-level ToolStreaming option.
// Mirrors upstream PR vercel/ai#14542 (commit ad0b376).
func TestBuildParams_DefaultEagerInputStreaming(t *testing.T) {
	functionTool := provider.Tool{
		Type:        provider.ToolTypeFunction,
		Name:        "search",
		Description: "Search the web",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}

	t.Run("StreamingDefaultsEagerOn", func(t *testing.T) {
		p, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Tools: []provider.Tool{functionTool},
		}, true)
		require.NoError(t, err)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.True(t, tp.EagerInputStreaming.Valid())
		assert.True(t, tp.EagerInputStreaming.Value)
	})

	t.Run("GenerateDoesNotDefault", func(t *testing.T) {
		p, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Tools: []provider.Tool{functionTool},
		}, false)
		require.NoError(t, err)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.False(t, tp.EagerInputStreaming.Valid())
	})

	t.Run("ToolStreamingFalseDisablesDefault", func(t *testing.T) {
		opts := provider.CallOptions{
			Tools: []provider.Tool{functionTool},
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(`{"toolStreaming": false}`),
				},
			},
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, true)
		require.NoError(t, err)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.False(t, tp.EagerInputStreaming.Valid())
	})

	t.Run("ToolStreamingTrueKeepsDefault", func(t *testing.T) {
		opts := provider.CallOptions{
			Tools: []provider.Tool{functionTool},
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(`{"toolStreaming": true}`),
				},
			},
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, true)
		require.NoError(t, err)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.True(t, tp.EagerInputStreaming.Valid())
		assert.True(t, tp.EagerInputStreaming.Value)
	})

	t.Run("PerToolFalseSuppressesDefault", func(t *testing.T) {
		// Upstream emits `eager_input_streaming` only when truthy
		// (anthropic-prepare-tools.ts:105). An explicit per-tool false must
		// therefore suppress the model-level default without sending the
		// field on the wire.
		toolWithFalse := functionTool
		toolWithFalse.ProviderOptions = provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"eagerInputStreaming": false}`),
			},
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Tools: []provider.Tool{toolWithFalse},
		}, true)
		require.NoError(t, err)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.False(t, tp.EagerInputStreaming.Valid(),
			"explicit false must omit the field, not send eager_input_streaming: false")
	})

	t.Run("PerToolTrueWinsWhenToolStreamingFalse", func(t *testing.T) {
		toolWithTrue := functionTool
		toolWithTrue.ProviderOptions = provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"eagerInputStreaming": true}`),
			},
		}
		opts := provider.CallOptions{
			Tools: []provider.Tool{toolWithTrue},
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(`{"toolStreaming": false}`),
				},
			},
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, true)
		require.NoError(t, err)
		require.Len(t, p.Tools, 1)
		tp := p.Tools[0].OfTool
		require.NotNil(t, tp)
		assert.True(t, tp.EagerInputStreaming.Valid())
		assert.True(t, tp.EagerInputStreaming.Value)
	})

	t.Run("ProviderDefinedToolsUnaffected", func(t *testing.T) {
		opts := provider.CallOptions{
			Tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305", Name: "search"},
			},
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, true)
		require.NoError(t, err)
		require.Len(t, p.Tools, 1)
		// Provider-defined tools are not OfTool; ensure no function-tool union
		// path was taken and there is no eager_input_streaming field to set.
		assert.Nil(t, p.Tools[0].OfTool)
	})
}

func TestConvertResponse_CodeExecutionInputRewriting(t *testing.T) {
	t.Run("bash_code_execution_wraps_input", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "server_tool_use", "id": "stu_1", "name": "bash_code_execution", "input": {"code": "ls -la"}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.Equal(t, provider.ContentToolCall, part.Type)
		assert.Equal(t, "code_execution", part.ToolName)
		assert.Contains(t, string(part.Input), `"type":"bash_code_execution"`)
		assert.Contains(t, string(part.Input), `"code":"ls -la"`)
		assert.True(t, part.ProviderExecuted)
	})

	t.Run("code_execution_programmatic_injection", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "server_tool_use", "id": "stu_1", "name": "code_execution", "input": {"code": "print('hi')"}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		assert.Contains(t, string(result.Content[0].Input), `"type":"programmatic-tool-call"`)
	})

	t.Run("code_execution_with_existing_type_not_injected", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "server_tool_use", "id": "stu_1", "name": "code_execution", "input": {"type": "bash", "code": "ls"}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		assert.NotContains(t, string(result.Content[0].Input), "programmatic-tool-call")
	})

}

func TestConvertResponse_ResultBlocks(t *testing.T) {
	t.Run("code_execution_tool_result", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "code_execution_tool_result", "tool_use_id": "stu_1", "content": {"type": "code_execution_result", "stdout": "hello\n", "stderr": "", "return_code": 0}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.code_execution_20250825", Name: "code_exec",
		}})
		result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.Equal(t, provider.ContentToolResult, part.Type)
		assert.Equal(t, "stu_1", part.ToolCallID)
		assert.Equal(t, "code_exec", part.ToolName)
		assert.True(t, part.ProviderExecuted)
	})

	t.Run("bash_code_execution_tool_result", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "bash_code_execution_tool_result", "tool_use_id": "stu_2", "content": {"type": "bash_code_execution_result", "stdout": "done", "stderr": "", "return_code": 0}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID: "anthropic.code_execution_20250825", Name: "code_exec",
		}})
		result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		assert.Equal(t, provider.ContentToolResult, result.Content[0].Type)
		assert.Equal(t, "code_exec", result.Content[0].ToolName)
	})

	t.Run("code_execution_tool_result_error", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "code_execution_tool_result", "tool_use_id": "stu_1", "content": {"type": "code_execution_tool_result_error", "error_code": "execution_time_exceeded"}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.Equal(t, provider.ContentToolResult, part.Type)
		assert.Equal(t, "code_execution", part.ToolName)
		assert.True(t, part.IsError)
		assert.Contains(t, string(part.Result), `"errorCode":"execution_time_exceeded"`)
		assert.Contains(t, string(part.Result), `"type":"code_execution_tool_result_error"`)
	})

	t.Run("code_execution_tool_result_normalizes_fields", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "code_execution_tool_result", "tool_use_id": "stu_1", "content": {"type": "code_execution_result", "stdout": "hello\n", "stderr": "warn", "return_code": 1, "content": null}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.False(t, part.IsError)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(part.Result, &parsed))
		assert.Equal(t, "code_execution_result", parsed["type"])
		assert.Equal(t, "hello\n", parsed["stdout"])
		assert.Equal(t, "warn", parsed["stderr"])
		assert.Equal(t, float64(1), parsed["return_code"])
		assert.NotNil(t, parsed["content"], "null content should be normalized to empty array")
		assert.Empty(t, parsed["content"], "null content should be normalized to empty array")
	})
}

func TestConvertResponse_ToolUseCallerMetadata(t *testing.T) {
	t.Run("tool_use_with_caller", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "tool_use", "id": "call_1", "name": "my_func", "input": {"x": 1}, "caller": {"type": "code_execution_20250825", "tool_id": "toolu_456"}}
			],
			"stop_reason": "tool_use", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.Equal(t, provider.ContentToolCall, part.Type)
		require.NotNil(t, part.ProviderMetadata)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(part.ProviderMetadata["anthropic"], &meta))
		caller := meta["caller"].(map[string]any)
		assert.Equal(t, "code_execution_20250825", caller["type"])
		assert.Equal(t, "toolu_456", caller["toolId"])
	})

	t.Run("tool_use_with_direct_caller", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "tool_use", "id": "call_1", "name": "my_func", "input": {}, "caller": {"type": "direct"}}
			],
			"stop_reason": "tool_use", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		require.NotNil(t, part.ProviderMetadata)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(part.ProviderMetadata["anthropic"], &meta))
		caller := meta["caller"].(map[string]any)
		assert.Equal(t, "direct", caller["type"])
	})

	t.Run("tool_use_without_caller", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "tool_use", "id": "call_1", "name": "my_func", "input": {}}
			],
			"stop_reason": "tool_use", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		assert.Nil(t, result.Content[0].ProviderMetadata)
	})
}

func TestConvertAssistantContent_ProviderExecutedToolCalls(t *testing.T) {
	mapping := toolNameMapping{}
	v := &cacheControlValidator{}
	mcpIDs := map[string]bool{}
	var warnings []provider.Warning

	t.Run("web_search emits server_tool_use", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:       "srv-1",
				ToolName:         "web_search",
				Input:            json.RawMessage(`{"query":"test"}`),
				ProviderExecuted: true,
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfServerToolUse)
		assert.Equal(t, "srv-1", blocks[0].OfServerToolUse.ID)
		assert.Equal(t, sdk.BetaServerToolUseBlockParamNameWebSearch, blocks[0].OfServerToolUse.Name)
		assert.Empty(t, warnings)
	})

	t.Run("code_execution emits server_tool_use", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:       "srv-2",
				ToolName:         "code_execution",
				Input:            json.RawMessage(`{"code":"print('hi')"}`),
				ProviderExecuted: true,
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfServerToolUse)
		assert.Equal(t, sdk.BetaServerToolUseBlockParamNameCodeExecution, blocks[0].OfServerToolUse.Name)
	})

	t.Run("bash_code_execution sub-tool uses sub-tool name", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:       "srv-3",
				ToolName:         "code_execution",
				Input:            json.RawMessage(`{"type":"bash_code_execution","code":"ls"}`),
				ProviderExecuted: true,
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfServerToolUse)
		assert.Equal(t, sdk.BetaServerToolUseBlockParamName("bash_code_execution"), blocks[0].OfServerToolUse.Name)
	})

	t.Run("programmatic-tool-call type stripped", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:       "srv-4",
				ToolName:         "code_execution",
				Input:            json.RawMessage(`{"type":"programmatic-tool-call","code":"print('hi')"}`),
				ProviderExecuted: true,
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfServerToolUse)
		assert.Equal(t, sdk.BetaServerToolUseBlockParamNameCodeExecution, blocks[0].OfServerToolUse.Name)
		inputMap, ok := blocks[0].OfServerToolUse.Input.(map[string]any)
		require.True(t, ok)
		_, hasType := inputMap["type"]
		assert.False(t, hasType, "programmatic-tool-call type should be stripped")
		assert.Equal(t, "print('hi')", inputMap["code"])
	})

	t.Run("tool_search_tool_regex emits server_tool_use", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:       "srv-5",
				ToolName:         "tool_search_tool_regex",
				Input:            json.RawMessage(`{}`),
				ProviderExecuted: true,
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfServerToolUse)
		assert.Equal(t, sdk.BetaServerToolUseBlockParamNameToolSearchToolRegex, blocks[0].OfServerToolUse.Name)
	})

	t.Run("unknown provider-executed tool produces warning", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:       "srv-6",
				ToolName:         "unknown_tool",
				Input:            json.RawMessage(`{}`),
				ProviderExecuted: true,
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		assert.Empty(t, blocks)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "unknown_tool")
	})

	t.Run("non-provider-executed still emits tool_use", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ToolCallPart("tc-1", "my_func", json.RawMessage(`{}`)),
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfToolUse)
		assert.Equal(t, "tc-1", blocks[0].OfToolUse.ID)
	})

	t.Run("non-provider-executed preserves caller metadata", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:      "tc-2",
				ToolName:        "my_func",
				Input:           json.RawMessage(`{}`),
				ProviderOptions: makeProviderOpts(`{"caller":{"type":"code_execution_20250825","toolId":"toolu_123"}}`),
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfToolUse)
		require.NotNil(t, blocks[0].OfToolUse.Caller.OfCodeExecution20250825)
		assert.Equal(t, "toolu_123", blocks[0].OfToolUse.Caller.OfCodeExecution20250825.ToolID)
	})

	t.Run("MCP tool call emits mcp_tool_use even when ProviderExecuted", func(t *testing.T) {
		mcpOpts := makeProviderOpts(`{"type":"mcp-tool-use","serverName":"test-server"}`)
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolCall,
				ToolCallID:       "mcp-1",
				ToolName:         "echo",
				Input:            json.RawMessage(`{"msg":"hi"}`),
				ProviderExecuted: true,
				ProviderOptions:  mcpOpts,
			},
		}
		localMCPIDs := map[string]bool{}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, localMCPIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfMCPToolUse, "should emit mcp_tool_use, not server_tool_use")
		assert.Equal(t, "mcp-1", blocks[0].OfMCPToolUse.ID)
		assert.Equal(t, "echo", blocks[0].OfMCPToolUse.Name)
		assert.Equal(t, "test-server", blocks[0].OfMCPToolUse.ServerName)
		assert.True(t, localMCPIDs["mcp-1"], "should register in mcpToolUseIDs")
	})
}

func TestConvertAssistantContent_InlineToolResults(t *testing.T) {
	mapping := toolNameMapping{}
	v := &cacheControlValidator{}
	mcpIDs := map[string]bool{}
	var warnings []provider.Warning

	t.Run("web_search result emits web_search_tool_result", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-1",
				ToolName:   "web_search",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(`[{"type":"web_search_result","url":"https://example.com","title":"Example","encryptedContent":"abc","pageAge":"1 day"}]`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfWebSearchToolResult)
		assert.Equal(t, "srv-1", blocks[0].OfWebSearchToolResult.ToolUseID)
		results := blocks[0].OfWebSearchToolResult.Content.OfResultBlock
		require.Len(t, results, 1)
		assert.Equal(t, "https://example.com", results[0].URL)
		assert.Equal(t, "Example", results[0].Title)
		assert.Equal(t, "abc", results[0].EncryptedContent)
		assert.Equal(t, "1 day", results[0].PageAge.Value)
	})

	t.Run("bash code execution error preserves error variant", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-bash-error",
				ToolName:   "code_execution",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(`{"type":"bash_code_execution_tool_result_error","error_code":"execution_time_exceeded"}`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfBashCodeExecutionToolResult)
		content := blocks[0].OfBashCodeExecutionToolResult.Content
		require.NotNil(t, content.OfRequestBashCodeExecutionToolResultError)
		assert.Nil(t, content.OfRequestBashCodeExecutionResultBlock)
		assert.Equal(t, sdk.BetaBashCodeExecutionToolResultErrorParamErrorCodeExecutionTimeExceeded, content.OfRequestBashCodeExecutionToolResultError.ErrorCode)
	})

	t.Run("web_search result without pageAge omits page_age", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-1b",
				ToolName:   "web_search",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(`[{"type":"web_search_result","url":"https://example.com","title":"No Age","encryptedContent":"def"}]`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfWebSearchToolResult)
		results := blocks[0].OfWebSearchToolResult.Content.OfResultBlock
		require.Len(t, results, 1)
		assert.Equal(t, "def", results[0].EncryptedContent)
		assert.False(t, results[0].PageAge.Valid(), "pageAge should not be set")
	})

	t.Run("nil provider executed output warns instead of panicking", func(t *testing.T) {
		warnings = nil
		block := convertInlineWebSearchResult(provider.ContentPart{
			Type:       provider.ContentPartTypeToolResult,
			ToolCallID: "srv-nil",
			ToolName:   "web_search",
			Output:     nil,
		}, sdk.BetaCacheControlEphemeralParam{}, &warnings)
		assert.Nil(t, block)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "<nil>")
	})

	t.Run("code_execution result emits code_execution_tool_result", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-2",
				ToolName:   "code_execution",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(`{"type":"code_execution_result","stdout":"hello\n","stderr":"","return_code":0}`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfCodeExecutionToolResult)
		assert.Equal(t, "srv-2", blocks[0].OfCodeExecutionToolResult.ToolUseID)
	})

	t.Run("MCP tool result emits mcp_tool_result", func(t *testing.T) {
		mcpIDsLocal := map[string]bool{"mcp-1": true}
		parts := []provider.ContentPart{
			provider.ToolResultPart("mcp-1", "echo", &provider.ToolResultOutput{
				Type: provider.ToolOutputJSON,
				JSON: json.RawMessage(`"result text"`),
			}),
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDsLocal, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfMCPToolResult)
		assert.Equal(t, "mcp-1", blocks[0].OfMCPToolResult.ToolUseID)
	})

	t.Run("tool_search result emits tool_search_tool_result", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-3",
				ToolName:   "tool_search_tool_bm25",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(`[{"toolName":"my_func"}]`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfToolSearchToolResult)
		assert.Equal(t, "srv-3", blocks[0].OfToolSearchToolResult.ToolUseID)
	})

	t.Run("web_fetch result emits web_fetch_tool_result", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-wf-1",
				ToolName:   "web_fetch",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(`{"type":"web_fetch_result","url":"https://example.com","retrievedAt":"2026-01-01T00:00:00Z","content":{"type":"document","title":"Example","citations":{"enabled":true},"source":{"type":"text","mediaType":"text/plain","data":"hello"}}}`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfWebFetchToolResult)
		assert.Equal(t, "srv-wf-1", blocks[0].OfWebFetchToolResult.ToolUseID)
		fetchBlock := blocks[0].OfWebFetchToolResult.Content.OfRequestWebFetchResultBlock
		require.NotNil(t, fetchBlock)
		assert.Equal(t, "https://example.com", fetchBlock.URL)
		assert.Equal(t, "2026-01-01T00:00:00Z", fetchBlock.RetrievedAt.Value)
		require.NotNil(t, fetchBlock.Content.Source.OfText)
		assert.Equal(t, "hello", fetchBlock.Content.Source.OfText.Data)
		assert.Equal(t, "Example", fetchBlock.Content.Title.Value)
	})

	t.Run("web_fetch error result emits web_fetch_tool_result with error", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-wf-2",
				ToolName:   "web_fetch",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputErrorJSON,
					JSON: json.RawMessage(`{"errorCode":"unavailable"}`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		require.Len(t, blocks, 1)
		require.NotNil(t, blocks[0].OfWebFetchToolResult)
		assert.Equal(t, "srv-wf-2", blocks[0].OfWebFetchToolResult.ToolUseID)
	})

	t.Run("unknown tool result produces warning", func(t *testing.T) {
		parts := []provider.ContentPart{
			provider.ContentPart{Type: provider.ContentPartTypeToolResult,
				ToolCallID: "srv-4",
				ToolName:   "unknown_tool",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(`{}`),
				},
			},
		}
		warnings = nil
		blocks := convertAssistantContent(v, mapping, parts, nil, mcpIDs, &warnings)
		assert.Empty(t, blocks)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "unknown_tool")
	})
}

// TestBuildParams_DocumentMediaTypes asserts upstream-aligned handling of
// non-image file parts: application/pdf maps to a PDF document block (with
// the pdfs-2024-09-25 beta), text/plain maps to a plain-text document block,
// and provider options provide title/context/citations metadata. Mirrors
// upstream convert-to-anthropic-prompt.ts:226-283.
func TestBuildParams_DocumentMediaTypes(t *testing.T) {
	t.Run("application/pdf base64 with title and citations", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(provider.ContentPart{
					Type:      provider.ContentPartTypeFile,
					MediaType: "application/pdf",
					Filename:  "report.pdf",
					Data:      &provider.DataContent{Base64: "JVBERi0=" /* %PDF- */},
					ProviderOptions: provider.BuildProviderOptions(provider.RawProviderOption{
						Key: "anthropic",
						Raw: json.RawMessage(`{"title":"Q4 Report","context":"finance","citations":{"enabled":true}}`),
					}),
				}),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		require.Len(t, p.Messages, 1)
		require.Len(t, p.Messages[0].Content, 1)

		block := p.Messages[0].Content[0]
		require.NotNil(t, block.OfDocument, "PDF must map to a document block")
		require.NotNil(t, block.OfDocument.Source.OfBase64, "base64 PDF must use base64 source")
		assert.Equal(t, "JVBERi0=", block.OfDocument.Source.OfBase64.Data)
		assert.Equal(t, "Q4 Report", block.OfDocument.Title.Or(""))
		assert.Equal(t, "finance", block.OfDocument.Context.Or(""))
		assert.True(t, block.OfDocument.Citations.Enabled.Or(false))

		// pdfs-2024-09-25 beta must be advertised.
		var found bool
		for _, b := range p.Betas {
			if b == "pdfs-2024-09-25" {
				found = true
				break
			}
		}
		assert.True(t, found, "pdfs-2024-09-25 beta must be advertised for application/pdf parts")
	})

	t.Run("application/pdf URL", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(provider.ContentPart{
					Type:      provider.ContentPartTypeFile,
					MediaType: "application/pdf",
					Data:      &provider.DataContent{URL: "https://example.com/r.pdf"},
				}),
			},
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		block := p.Messages[0].Content[0]
		require.NotNil(t, block.OfDocument)
		require.NotNil(t, block.OfDocument.Source.OfURL)
		assert.Equal(t, "https://example.com/r.pdf", block.OfDocument.Source.OfURL.URL)
	})

	t.Run("text/plain bytes maps to plain-text document", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(provider.ContentPart{
					Type:      provider.ContentPartTypeFile,
					MediaType: "text/plain",
					Filename:  "notes.txt",
					Data:      &provider.DataContent{Bytes: []byte("hello world")},
					ProviderOptions: provider.BuildProviderOptions(provider.RawProviderOption{
						Key: "anthropic",
						Raw: json.RawMessage(`{"citations":{"enabled":true}}`),
					}),
				}),
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		block := p.Messages[0].Content[0]
		require.NotNil(t, block.OfDocument, "text/plain must map to a document block")
		require.NotNil(t, block.OfDocument.Source.OfText, "byte data must use plain-text source, not base64")
		assert.Equal(t, "hello world", block.OfDocument.Source.OfText.Data)
		// Title falls back to the filename when no override is set.
		assert.Equal(t, "notes.txt", block.OfDocument.Title.Or(""))
		assert.True(t, block.OfDocument.Citations.Enabled.Or(false))

		// text/plain must NOT trigger the pdfs beta.
		for _, b := range p.Betas {
			assert.NotEqual(t, sdk.AnthropicBeta("pdfs-2024-09-25"), b, "text/plain must not advertise the pdfs beta")
		}
	})

	t.Run("unsupported media type produces no block", func(t *testing.T) {
		opts := provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(provider.ContentPart{
					Type:      provider.ContentPartTypeFile,
					MediaType: "application/zip",
					Data:      &provider.DataContent{Bytes: []byte{0x00}},
				}),
			},
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		require.Len(t, p.Messages, 1)
		assert.Empty(t, p.Messages[0].Content, "unsupported media types must be silently dropped, not misencoded as PDF")
	})
}

// TestBuildParams_ThinkingClearsSamplingParams asserts that enabling thinking
// (or adaptive thinking) drops temperature/topP/topK and emits unsupported
// warnings, matching upstream anthropic-language-model.ts:608-633.
func TestBuildParams_ThinkingClearsSamplingParams(t *testing.T) {
	temp := 0.7
	topP := 0.9
	topK := 50

	opts := provider.CallOptions{
		Temperature: &temp,
		TopP:        &topP,
		TopK:        &topK,
		Prompt:      []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
			Thinking: &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: 2048},
		}),
	}

	p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	assert.False(t, p.Temperature.Valid(), "temperature must be cleared when thinking is enabled")
	assert.False(t, p.TopP.Valid(), "topP must be cleared when thinking is enabled")
	assert.False(t, p.TopK.Valid(), "topK must be cleared when thinking is enabled")

	features := map[string]bool{}
	for _, w := range warnings {
		features[w.Feature] = true
	}
	assert.True(t, features["temperature"], "expected unsupported temperature warning")
	assert.True(t, features["topP"], "expected unsupported topP warning")
	assert.True(t, features["topK"], "expected unsupported topK warning")
}

func TestBuildParams_SamplingNormalization(t *testing.T) {
	t.Run("temperature clamped high", func(t *testing.T) {
		temp := 2.0
		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Temperature: &temp,
			Prompt:      []provider.Message{provider.UserText("hi")},
		}, false)
		require.NoError(t, err)

		assert.True(t, p.Temperature.Valid())
		assert.Equal(t, 1.0, p.Temperature.Value)
		assert.Contains(t, warningFeatures(warnings), "temperature")
	})

	t.Run("temperature clamped low", func(t *testing.T) {
		temp := -0.1
		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Temperature: &temp,
			Prompt:      []provider.Message{provider.UserText("hi")},
		}, false)
		require.NoError(t, err)

		assert.True(t, p.Temperature.Valid())
		assert.Equal(t, 0.0, p.Temperature.Value)
		assert.Contains(t, warningFeatures(warnings), "temperature")
	})

	t.Run("topP dropped when temperature set", func(t *testing.T) {
		temp := 0.7
		topP := 0.9
		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
			Temperature: &temp,
			TopP:        &topP,
			Prompt:      []provider.Message{provider.UserText("hi")},
		}, false)
		require.NoError(t, err)

		assert.True(t, p.Temperature.Valid())
		assert.False(t, p.TopP.Valid())
		assert.Contains(t, warningFeatures(warnings), "topP")
	})
}

func TestBuildParams_Opus47RejectsSamplingParams(t *testing.T) {
	temp := 0.7
	topP := 0.9
	topK := 50

	opts := provider.CallOptions{
		Temperature: &temp,
		TopP:        &topP,
		TopK:        &topK,
		Prompt:      []provider.Message{provider.UserText("hi")},
	}

	p, _, warnings, _, err := buildParams("claude-opus-4-7", opts, false)
	require.NoError(t, err)

	assert.False(t, p.Temperature.Valid(), "temperature must be cleared for claude-opus-4-7")
	assert.False(t, p.TopP.Valid(), "topP must be cleared for claude-opus-4-7")
	assert.False(t, p.TopK.Valid(), "topK must be cleared for claude-opus-4-7")

	features := map[string]string{}
	for _, w := range warnings {
		features[w.Feature] = w.Details
	}
	assert.Contains(t, features["temperature"], "claude-opus-4-7")
	assert.Contains(t, features["topP"], "claude-opus-4-7")
	assert.Contains(t, features["topK"], "claude-opus-4-7")
}

// TestBuildParams_ReasoningEffortFallback asserts that a top-level Reasoning
// hint maps to OutputConfig.Effort even when AnthropicOptions only sets
// thinking (and not effort), matching upstream
// anthropic-language-model.ts:390-413.
func TestBuildParams_ReasoningEffortFallback(t *testing.T) {
	t.Run("provider thinking only -> top-level reasoning fills effort", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		opts := provider.CallOptions{
			MaxOutputTokens: ptrInt(8000),
			Prompt:          []provider.Message{provider.UserText("hi")},
			Reasoning:       &reasoning,
			ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
				Thinking: &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: 2048},
			}),
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		// Effort must be derived from top-level reasoning since AnthropicOptions.Effort is empty.
		assert.NotEmpty(t, string(p.OutputConfig.Effort), "top-level reasoning must fill OutputConfig.Effort when provider effort is unset")
	})

	t.Run("provider effort wins over top-level reasoning", func(t *testing.T) {
		reasoning := provider.ReasoningLow
		opts := provider.CallOptions{
			MaxOutputTokens: ptrInt(8000),
			Prompt:          []provider.Message{provider.UserText("hi")},
			Reasoning:       &reasoning,
			ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
				Effort: "high",
			}),
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		assert.Equal(t, "high", string(p.OutputConfig.Effort), "provider effort must take precedence over top-level reasoning")
	})

	t.Run("provider thinking=disabled blocks effort derivation", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		opts := provider.CallOptions{
			MaxOutputTokens: ptrInt(8000),
			Prompt:          []provider.Message{provider.UserText("hi")},
			Reasoning:       &reasoning,
			ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
				Thinking: &ThinkingConfig{Type: ThinkingDisabled},
			}),
		}
		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		assert.Empty(t, string(p.OutputConfig.Effort), "effort must not be derived when provider thinking is disabled")
	})
}

// --- groupIntoBlocks / convertUserContent tool-result parity tests (issue #173) ---

// TestBuildParams_UserMessageWithToolResult covers the basic regression from
// issue #173: a `RoleUser` provider message that mixes text and tool_result
// parts must surface BOTH blocks on the wire, in source order. Before the
// fix, `convertUserContent`'s switch silently dropped tool_result.
func TestBuildParams_UserMessageWithToolResult(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "out"}),
				provider.TextPart("then text"),
			),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1, "single user message expected")
	assert.EqualValues(t, "user", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 2, "tool_result and text must both appear")

	require.NotNil(t, p.Messages[0].Content[0].OfToolResult, "first block must be tool_result")
	assert.Equal(t, "call_1", p.Messages[0].Content[0].OfToolResult.ToolUseID)
	require.NotNil(t, p.Messages[0].Content[1].OfText, "second block must be text")
	assert.Equal(t, "then text", p.Messages[0].Content[1].OfText.Text)
}

// TestBuildParams_ConsecutiveUserAndToolMessagesMerge covers the
// groupIntoBlocks pre-pass: a RoleUser then RoleTool sequence merges into
// one Anthropic user message preserving order.
func TestBuildParams_ConsecutiveUserAndToolMessagesMerge(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("hi"),
			provider.NewToolMessage(provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "out"})),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1, "user + tool must merge into one Anthropic user message")
	assert.EqualValues(t, "user", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 2)
	require.NotNil(t, p.Messages[0].Content[0].OfText)
	assert.Equal(t, "hi", p.Messages[0].Content[0].OfText.Text)
	require.NotNil(t, p.Messages[0].Content[1].OfToolResult)
	assert.Equal(t, "call_1", p.Messages[0].Content[1].OfToolResult.ToolUseID)
}

// TestBuildParams_ConsecutiveToolAndUserMessagesMerge covers the reverse
// adjacency: RoleTool then RoleUser also merges into one Anthropic user
// message.
func TestBuildParams_ConsecutiveToolAndUserMessagesMerge(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewToolMessage(provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "out"})),
			provider.UserText("ok"),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1, "tool + user must merge into one Anthropic user message")
	assert.EqualValues(t, "user", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 2)
	require.NotNil(t, p.Messages[0].Content[0].OfToolResult)
	assert.Equal(t, "call_1", p.Messages[0].Content[0].OfToolResult.ToolUseID)
	require.NotNil(t, p.Messages[0].Content[1].OfText)
	assert.Equal(t, "ok", p.Messages[0].Content[1].OfText.Text)
}

// TestBuildParams_AssistantToolUserGrouping covers the canonical multi-step
// scenario from #173: assistant emits tool_use, tool message returns
// tool_result, user injects follow-up text. The fix must produce exactly:
// [assistant(tool_use), user(tool_result, text)] -- not three messages.
func TestBuildParams_AssistantToolUserGrouping(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(provider.ToolCallPart("call_1", "search", json.RawMessage(`{}`))),
			provider.NewToolMessage(provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "out"})),
			provider.UserText("ok"),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 2, "assistant + (tool + user) must produce two Anthropic messages")

	assert.EqualValues(t, "assistant", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 1)
	require.NotNil(t, p.Messages[0].Content[0].OfToolUse)
	assert.Equal(t, "call_1", p.Messages[0].Content[0].OfToolUse.ID)

	assert.EqualValues(t, "user", p.Messages[1].Role)
	require.Len(t, p.Messages[1].Content, 2)
	require.NotNil(t, p.Messages[1].Content[0].OfToolResult)
	assert.Equal(t, "call_1", p.Messages[1].Content[0].OfToolResult.ToolUseID)
	require.NotNil(t, p.Messages[1].Content[1].OfText)
	assert.Equal(t, "ok", p.Messages[1].Content[1].OfText.Text)
}

// TestBuildParams_StandaloneToolMessage is a regression guard: a single
// RoleTool message with no surrounding user/tool messages still produces
// exactly one Anthropic user message with one tool_result block.
func TestBuildParams_StandaloneToolMessage(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewToolMessage(provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "out"})),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	assert.EqualValues(t, "user", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 1)
	require.NotNil(t, p.Messages[0].Content[0].OfToolResult)
	assert.Equal(t, "call_1", p.Messages[0].Content[0].OfToolResult.ToolUseID)
}

// TestBuildParams_ConsecutiveAssistantMessagesMerge mirrors upstream's
// SystemBlock/AssistantBlock merge on the assistant side: two adjacent
// assistant messages collapse into one.
func TestBuildParams_ConsecutiveAssistantMessagesMerge(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.AssistantText("hi"),
			provider.NewAssistantMessage(provider.ToolCallPart("c1", "search", json.RawMessage(`{}`))),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1, "consecutive assistant messages must merge")
	assert.EqualValues(t, "assistant", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 2)
	require.NotNil(t, p.Messages[0].Content[0].OfText)
	assert.Equal(t, "hi", p.Messages[0].Content[0].OfText.Text)
	require.NotNil(t, p.Messages[0].Content[1].OfToolUse)
	assert.Equal(t, "c1", p.Messages[0].Content[1].OfToolUse.ID)
}

// TestBuildParams_ThreeConsecutiveToolMessagesMerge ensures the merge
// spans more than two adjacent messages and preserves source-order.
func TestBuildParams_ThreeConsecutiveToolMessagesMerge(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewToolMessage(provider.ToolResultPart("c1", "t1", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r1"})),
			provider.NewToolMessage(provider.ToolResultPart("c2", "t2", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r2"})),
			provider.NewToolMessage(provider.ToolResultPart("c3", "t3", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r3"})),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1, "three adjacent tool messages must merge into one user message")
	require.Len(t, p.Messages[0].Content, 3)
	for i, expectID := range []string{"c1", "c2", "c3"} {
		require.NotNil(t, p.Messages[0].Content[i].OfToolResult, "block %d must be tool_result", i)
		assert.Equal(t, expectID, p.Messages[0].Content[i].OfToolResult.ToolUseID)
	}
}

// TestBuildParams_ApprovalResponseInUserMessageSilentlySkipped covers the
// `convertUserContent` no-op skip for tool-approval-response parts (mirrors
// upstream's `if (part.type === 'tool-approval-response') { continue; }` in
// the user-block handler). The text part on the same message must still
// reach the wire, and no warning is emitted for the silent skip.
func TestBuildParams_ApprovalResponseInUserMessageSilentlySkipped(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.ToolApprovalResponsePart("a1", true, ""),
				provider.TextPart("hello"),
			),
		},
	}

	p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	require.Len(t, p.Messages[0].Content, 1, "approval response must produce no block")
	require.NotNil(t, p.Messages[0].Content[0].OfText)
	assert.Equal(t, "hello", p.Messages[0].Content[0].OfText.Text)

	for _, w := range warnings {
		assert.NotEqual(t, "toolApprovalResponse", w.Feature, "user-role approval response must NOT add a warning (only the tool-role path warns)")
	}
}

// TestBuildParams_CacheControlCascadePreservedAcrossMerge confirms that
// each source message's message-level cache_control cascade applies to its
// own last part inside the merged Anthropic block (D4 in design).
func TestBuildParams_CacheControlCascadePreservedAcrossMerge(t *testing.T) {
	cc := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.Message{
				Role: provider.RoleTool,
				Content: []provider.ContentPart{
					provider.ToolResultPart("c1", "t1", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r1"}),
				},
				ProviderOptions: cc,
			},
			provider.Message{
				Role: provider.RoleUser,
				Content: []provider.ContentPart{
					provider.TextPart("hello"),
				},
				ProviderOptions: cc,
			},
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1, "tool + user merge into one user message")
	require.Len(t, p.Messages[0].Content, 2)
	require.NotNil(t, p.Messages[0].Content[0].OfToolResult)
	assert.EqualValues(t, "ephemeral", p.Messages[0].Content[0].OfToolResult.CacheControl.Type,
		"first block (tool_result) must inherit its source-message cache_control")
	require.NotNil(t, p.Messages[0].Content[1].OfText)
	assert.EqualValues(t, "ephemeral", p.Messages[0].Content[1].OfText.CacheControl.Type,
		"second block (text) must inherit its own source-message cache_control")
}

// TestBuildParams_CacheControlSourceMessageScoped: when only the FIRST
// source message in a merged user block has message-level cache_control,
// only the first block carries it. The cascade is keyed off source-message
// last-part, not merged-block last-part.
func TestBuildParams_CacheControlSourceMessageScoped(t *testing.T) {
	cc := makeProviderOpts(`{"cacheControl": {"type": "ephemeral"}}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.Message{
				Role: provider.RoleTool,
				Content: []provider.ContentPart{
					provider.ToolResultPart("c1", "t1", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r1"}),
				},
				ProviderOptions: cc,
			},
			provider.UserText("hello"),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 1)
	require.Len(t, p.Messages[0].Content, 2)
	require.NotNil(t, p.Messages[0].Content[0].OfToolResult)
	assert.EqualValues(t, "ephemeral", p.Messages[0].Content[0].OfToolResult.CacheControl.Type,
		"tool_result (last part of first source message) must inherit cache_control")
	require.NotNil(t, p.Messages[0].Content[1].OfText)
	assert.NotEqualValues(t, "ephemeral", p.Messages[0].Content[1].OfText.CacheControl.Type,
		"text (last part of second source message, which has no cache_control) must NOT have cache_control")
}

// TestBuildParams_MCPToolResultInUserMessage confirms the shared
// appendToolResultBlock helper detects MCP tool-call IDs even when the
// matching tool_result lands inside a `RoleUser` message rather than a
// `RoleTool` message.
func TestBuildParams_MCPToolResultInUserMessage(t *testing.T) {
	mcpOpts := makeProviderOpts(`{"type": "mcp-tool-use", "serverName": "srv"}`)
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(provider.ContentPart{
				Type:            provider.ContentPartTypeToolCall,
				ToolCallID:      "mcp-1",
				ToolName:        "echo",
				Input:           json.RawMessage(`{}`),
				ProviderOptions: mcpOpts,
			}),
			provider.NewUserMessage(
				provider.ToolResultPart("mcp-1", "echo", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`"out"`)}),
			),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.Messages, 2)
	require.Len(t, p.Messages[1].Content, 1)
	require.NotNil(t, p.Messages[1].Content[0].OfMCPToolResult, "tool-result inside a user message must still detect MCP")
	assert.Nil(t, p.Messages[1].Content[0].OfToolResult)
	assert.Equal(t, "mcp-1", p.Messages[1].Content[0].OfMCPToolResult.ToolUseID)
}

// TestBuildParams_NonGroupingPromptUnchanged is a regression guard: a
// prompt that does not exercise any merge or tool-result-in-user path
// produces the same one-Anthropic-message-per-source-message structure as
// before the change.
func TestBuildParams_NonGroupingPromptUnchanged(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("you are helpful"),
			provider.UserText("hi"),
			provider.AssistantText("hello there"),
		},
	}

	p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)

	require.Len(t, p.System, 1)
	assert.Equal(t, "you are helpful", p.System[0].Text)
	require.Len(t, p.Messages, 2)
	assert.EqualValues(t, "user", p.Messages[0].Role)
	require.Len(t, p.Messages[0].Content, 1)
	require.NotNil(t, p.Messages[0].Content[0].OfText)
	assert.Equal(t, "hi", p.Messages[0].Content[0].OfText.Text)
	assert.EqualValues(t, "assistant", p.Messages[1].Role)
	require.Len(t, p.Messages[1].Content, 1)
	require.NotNil(t, p.Messages[1].Content[0].OfText)
	assert.Equal(t, "hello there", p.Messages[1].Content[0].OfText.Text)
}

// TestGroupIntoBlocks_GroupingRules unit-tests the pre-pass directly.
// Mirrors the role-switch table in upstream's `groupIntoBlocks`
// (`convert-to-anthropic-prompt.ts:1136-1183`).
func TestGroupIntoBlocks_GroupingRules(t *testing.T) {
	tests := []struct {
		name   string
		prompt []provider.Message
		want   []promptBlock
	}{
		{
			name:   "empty",
			prompt: nil,
			want:   nil,
		},
		{
			name: "system_user",
			prompt: []provider.Message{
				provider.NewSystemMessage("s"),
				provider.UserText("u"),
			},
			want: []promptBlock{
				{kind: promptBlockKindSystem, messages: []provider.Message{provider.NewSystemMessage("s")}},
				{kind: promptBlockKindUser, messages: []provider.Message{provider.UserText("u")}},
			},
		},
		{
			name: "user_tool_user_merges_into_one_user_block",
			prompt: []provider.Message{
				provider.UserText("a"),
				provider.NewToolMessage(provider.ToolResultPart("c1", "t", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r"})),
				provider.UserText("b"),
			},
			want: []promptBlock{{kind: promptBlockKindUser, messages: []provider.Message{
				provider.UserText("a"),
				provider.NewToolMessage(provider.ToolResultPart("c1", "t", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "r"})),
				provider.UserText("b"),
			}}},
		},
		{
			name: "assistant_user_assistant_alternates",
			prompt: []provider.Message{
				provider.AssistantText("a1"),
				provider.UserText("u1"),
				provider.AssistantText("a2"),
			},
			want: []promptBlock{
				{kind: promptBlockKindAssistant, messages: []provider.Message{provider.AssistantText("a1")}},
				{kind: promptBlockKindUser, messages: []provider.Message{provider.UserText("u1")}},
				{kind: promptBlockKindAssistant, messages: []provider.Message{provider.AssistantText("a2")}},
			},
		},
		{
			name: "consecutive_systems_merge",
			prompt: []provider.Message{
				provider.NewSystemMessage("s1"),
				provider.NewSystemMessage("s2"),
			},
			want: []promptBlock{{kind: promptBlockKindSystem, messages: []provider.Message{
				provider.NewSystemMessage("s1"),
				provider.NewSystemMessage("s2"),
			}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := groupIntoBlocks(tc.prompt)
			require.Len(t, got, len(tc.want), "block count mismatch")
			for i := range got {
				assert.Equal(t, tc.want[i].kind, got[i].kind, "block %d kind", i)
				assert.Equal(t, tc.want[i].messages, got[i].messages, "block %d messages", i)
			}
		})
	}
}

func ptrInt(i int) *int { return &i }

func TestHasWebTool20260209WithoutCodeExecution(t *testing.T) {
	tests := []struct {
		name  string
		tools []provider.Tool
		want  bool
	}{
		{
			name:  "nil tools",
			tools: nil,
			want:  false,
		},
		{
			name: "web_fetch_20260209 without code_execution",
			tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20260209", Name: "web_fetch"},
			},
			want: true,
		},
		{
			name: "web_search_20260209 without code_execution",
			tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20260209", Name: "web_search"},
			},
			want: true,
		},
		{
			name: "web_fetch_20260209 with code_execution_20250522",
			tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20260209", Name: "web_fetch"},
				{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "code_execution"},
			},
			want: false,
		},
		{
			name: "web_fetch_20260209 with code_execution_20260120",
			tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20260209", Name: "web_fetch"},
				{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20260120", Name: "code_execution"},
			},
			want: false,
		},
		{
			name: "only older web_search (not 20260209)",
			tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305", Name: "web_search"},
			},
			want: false,
		},
		{
			name: "function tool only",
			tools: []provider.Tool{
				{Type: provider.ToolTypeFunction, Name: "lookup"},
			},
			want: false,
		},
		{
			name: "web_fetch_20260209 alongside function tool",
			tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20260209", Name: "web_fetch"},
				{Type: provider.ToolTypeFunction, Name: "lookup"},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasWebTool20260209WithoutCodeExecution(tc.tools))
		})
	}
}

func TestBuildParams_MarkCodeExecutionDynamic(t *testing.T) {
	t.Run("set when web_fetch_20260209 without code_execution", func(t *testing.T) {
		opts := provider.CallOptions{
			Tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20260209", Name: "web_fetch"},
			},
		}
		_, _, _, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		assert.True(t, br.markCodeExecutionDynamic)
	})

	t.Run("unset when both web_fetch_20260209 and code_execution configured", func(t *testing.T) {
		opts := provider.CallOptions{
			Tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20260209", Name: "web_fetch"},
				{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20260120", Name: "code_execution"},
			},
		}
		_, _, _, br, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)
		assert.False(t, br.markCodeExecutionDynamic)
	})
}

// TestConvertResponse_CodeExecutionDynamic exercises the upstream
// hasWebTool20260209WithoutCodeExecution mark on non-streaming responses.
// Mirrors upstream anthropic-language-model.ts:1043-1047.
func TestConvertResponse_CodeExecutionDynamic(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
		"content": [
			{"type": "server_tool_use", "id": "stu_1", "name": "code_execution", "input": {"code": "print('hi')"}}
		],
		"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	t.Run("dynamic when markCodeExecutionDynamic=true", func(t *testing.T) {
		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", true)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)
		part := result.Content[0]
		require.NotNil(t, part.Dynamic, "Dynamic must be set when markCodeExecutionDynamic=true")
		assert.True(t, *part.Dynamic)
		assert.True(t, part.ProviderExecuted)
	})

	t.Run("not dynamic when markCodeExecutionDynamic=false", func(t *testing.T) {
		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)
		assert.Nil(t, result.Content[0].Dynamic)
	})

	t.Run("dynamic also applies to bash_code_execution wire name", func(t *testing.T) {
		bash := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "server_tool_use", "id": "stu_1", "name": "bash_code_execution", "input": {"code": "ls"}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)
		result, err := convertResponse(bash, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", true)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)
		require.NotNil(t, result.Content[0].Dynamic)
		assert.True(t, *result.Content[0].Dynamic)
	})

	t.Run("does not mark non-code_execution server tool", func(t *testing.T) {
		web := unmarshalMessage(t, `{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
			"content": [
				{"type": "server_tool_use", "id": "stu_1", "name": "web_fetch", "input": {"url": "https://example.com"}}
			],
			"stop_reason": "end_turn", "usage": {"input_tokens": 10, "output_tokens": 5}
		}`)
		result, err := convertResponse(web, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", true)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)
		assert.Nil(t, result.Content[0].Dynamic)
	})
}
