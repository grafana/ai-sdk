package anthropic

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_ForcedToolChoiceWithThinking(t *testing.T) {
	for _, tc := range []struct {
		name, modelID string
		thinking      *ThinkingConfig
		reasoning     provider.ReasoningEffort
		wantThinking  ThinkingType
		wantAuto      bool
	}{
		{name: "manual", modelID: "claude-haiku-4-5", thinking: &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: 1024}, wantThinking: ThinkingEnabled, wantAuto: true},
		{name: "portable_budget", modelID: "claude-haiku-4-5", reasoning: provider.ReasoningLow, wantThinking: ThinkingEnabled, wantAuto: true},
		{name: "adaptive", modelID: "claude-sonnet-4-6", thinking: &ThinkingConfig{Type: ThinkingAdaptive}, wantThinking: ThinkingAdaptive},
		{name: "disabled", modelID: "claude-haiku-4-5", thinking: &ThinkingConfig{Type: ThinkingDisabled}, wantThinking: ThinkingDisabled},
	} {
		for _, choice := range []provider.ToolChoice{
			{Type: provider.ToolChoiceRequired},
			{Type: provider.ToolChoiceTool, ToolName: "lookup"},
		} {
			t.Run(tc.name+"/"+string(choice.Type), func(t *testing.T) {
				p, _, warnings, _, err := buildParams(tc.modelID, provider.CallOptions{
					Prompt: []provider.Message{provider.UserText("Look up the answer.")},
					Tools: []provider.Tool{
						{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)},
						{Type: provider.ToolTypeFunction, Name: "other", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
					ToolChoice:      &choice,
					Reasoning:       tc.reasoning,
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Thinking: tc.thinking}),
				}, false)
				require.NoError(t, err)
				switch tc.wantThinking {
				case ThinkingEnabled:
					assert.NotNil(t, p.Thinking.OfEnabled)
				case ThinkingAdaptive:
					assert.NotNil(t, p.Thinking.OfAdaptive)
				case ThinkingDisabled:
					assert.NotNil(t, p.Thinking.OfDisabled)
				}

				names := make([]string, 0, len(p.Tools))
				for _, tool := range p.Tools {
					names = append(names, *tool.GetName())
				}
				if tc.wantAuto {
					assert.NotNil(t, p.ToolChoice.OfAuto)
					wantTools := []string{"lookup", "other"}
					if choice.Type == provider.ToolChoiceTool {
						wantTools = []string{"lookup"}
					}
					assert.Equal(t, wantTools, names)
					require.Len(t, warnings, 1)
					assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
					assert.Equal(t, "toolChoice", warnings[0].Feature)
					assert.Contains(t, warnings[0].Details, "not supported with this model or thinking configuration")
					if choice.Type == provider.ToolChoiceTool {
						assert.Contains(t, warnings[0].Details, "Only the 'lookup' tool is sent")
					}
				} else {
					assert.Equal(t, []string{"lookup", "other"}, names)
					assert.Empty(t, warnings)
					if choice.Type == provider.ToolChoiceRequired {
						assert.NotNil(t, p.ToolChoice.OfAny)
					} else if assert.NotNil(t, p.ToolChoice.OfTool) {
						assert.Equal(t, "lookup", p.ToolChoice.OfTool.Name)
					}
				}
			})
		}
	}
}

func TestJSONToolFallback_FinalText(t *testing.T) {
	const citation = `{"type":"web_search_result_location","url":"https://example.com/report","title":"Report","cited_text":"source","encrypted_index":"opaque"}`
	const preamble = `{"type":"text","text":"Checking first."}`
	const search = `{"type":"server_tool_use","name":"web_search","id":"srvtoolu_search","input":{"query":"answer"}}`
	const result = `{"type":"web_search_tool_result","tool_use_id":"srvtoolu_search","content":[]}`
	const answer = `{"type":"text","text":"Final answer","citations":[` + citation + `]}`
	const jsonCall = `{"type":"tool_use","name":"json","id":"toolu_json","input":{"answer":42}}`

	for _, tc := range []struct {
		name, stop    string
		blocks        []string
		wantText      []string
		wantCitations bool
		finish        provider.UnifiedFinishReason
	}{
		{name: "text_only", stop: "end_turn", blocks: []string{answer}, wantText: []string{"Final answer"}, wantCitations: true, finish: provider.FinishReasonStop},
		{name: "tool_and_result_discard_preamble", stop: "end_turn", blocks: []string{preamble, search, preamble, result, answer}, wantText: []string{"Final answer"}, wantCitations: true, finish: provider.FinishReasonStop},
		{name: "json_call_suppresses_all_plain_text", stop: "end_turn", blocks: []string{preamble, jsonCall, answer}, wantText: []string{`{"answer":42}`}, finish: provider.FinishReasonStop},
		{name: "tool_use", stop: "tool_use", blocks: []string{search, answer}, finish: provider.FinishReasonToolCalls},
		{name: "pause_turn", stop: "pause_turn", blocks: []string{answer}, finish: provider.FinishReasonStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawBlocks := make([]json.RawMessage, 0, len(tc.blocks))
			for _, block := range tc.blocks {
				rawBlocks = append(rawBlocks, json.RawMessage(block))
			}
			encoded, err := json.Marshal(rawBlocks)
			require.NoError(t, err)
			msg := unmarshalMessage(t, fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-3-haiku","content":%s,"stop_reason":%q,"usage":{"input_tokens":1,"output_tokens":1}}`, encoded, tc.stop))
			generated, err := convertResponse(msg, toolNameMapping{}, true, nil, seqIDGenerator(), "anthropic", false)
			require.NoError(t, err)
			var gotText []string
			var textPart provider.GenerateContentPart
			for _, part := range generated.Content {
				if part.Type == provider.ContentText {
					gotText = append(gotText, part.Text)
					textPart = part
				}
			}
			assert.Equal(t, tc.wantText, gotText, "generate")
			assert.Equal(t, provider.FinishReason{Unified: tc.finish, Raw: tc.stop}, generated.FinishReason)
			if tc.wantCitations && assert.Len(t, textPart.Citations, 1) {
				assert.Equal(t, "https://example.com/report", textPart.Citations[0].Source.ID)
				assert.JSONEq(t, `{"citations":[`+citation+`]}`, string(textPart.ProviderMetadata["anthropic"]))
			}

			events := jsonFallbackMessageEvents(t, "msg_1", tc.stop, tc.blocks...)
			adapter := newJSONFallbackTestAdapter()
			ch := make(chan provider.StreamPart, 64)
			for _, event := range events[:len(events)-1] {
				require.NoError(t, adapter.handleEvent(unmarshalEvent(t, event), ch))
			}
			var streamText []string
			var textParts []provider.StreamPart
			drain := func() {
				for len(ch) > 0 {
					part := <-ch
					switch part.Type {
					case provider.PartTextStart, provider.PartTextDelta, provider.PartTextEnd:
						textParts = append(textParts, part)
						if part.Type == provider.PartTextDelta {
							streamText = append(streamText, part.Delta)
						}
					case provider.PartFinish:
						assert.Equal(t, &generated.FinishReason, part.FinishReason)
					}
				}
			}
			drain()
			if tc.name != "json_call_suppresses_all_plain_text" {
				assert.Empty(t, textParts, "plain text must not appear before message_stop")
			}
			require.NoError(t, adapter.handleEvent(unmarshalEvent(t, events[len(events)-1]), ch))
			drain()
			assert.Equal(t, tc.wantText, streamText, "stream")
			if tc.wantCitations {
				require.Len(t, textParts, 3)
				assert.Equal(t, provider.PartTextStart, textParts[0].Type)
				assert.Equal(t, provider.PartTextDelta, textParts[1].Type)
				assert.Equal(t, provider.PartTextEnd, textParts[2].Type)
				assert.Equal(t, textParts[0].ID, textParts[1].ID)
				assert.Equal(t, textParts[0].ID, textParts[2].ID)
				require.Len(t, textParts[2].Citations, 1)
				assert.Equal(t, textPart.Citations, textParts[2].Citations)
				assert.Equal(t, textPart.ProviderMetadata, textParts[2].ProviderMetadata)
			}
		})
	}
}

func TestJSONToolFallback_ContinuedStream(t *testing.T) {
	for _, reason := range []string{"tool_use", "pause_turn"} {
		t.Run(reason, func(t *testing.T) {
			adapter := newJSONFallbackTestAdapter()
			ch := make(chan provider.StreamPart, 64)
			for _, msg := range [][]string{
				jsonFallbackMessageEvents(t, "msg_1", reason, `{"type":"text","text":"Still working."}`),
				jsonFallbackMessageEvents(t, "msg_2", "end_turn", `{"type":"text","text":"Final answer"}`),
			} {
				for _, event := range msg {
					require.NoError(t, adapter.handleEvent(unmarshalEvent(t, event), ch))
				}
			}
			close(ch)
			var text, stops []string
			for part := range ch {
				switch part.Type {
				case provider.PartTextDelta:
					text = append(text, part.Delta)
				case provider.PartFinish:
					stops = append(stops, part.FinishReason.Raw)
				}
			}
			assert.Equal(t, []string{"Final answer"}, text)
			assert.Equal(t, []string{reason, "end_turn"}, stops)
		})
	}
}

func newJSONFallbackTestAdapter() streamAdapter {
	return streamAdapter{
		usesJsonResponseTool: true,
		blocks:               make(map[int64]*blockState),
		serverToolCalls:      make(map[string]string),
		generateID:           seqIDGenerator(),
	}
}

func jsonFallbackMessageEvents(t *testing.T, id, reason string, blocks ...string) []string {
	t.Helper()
	events := []string{fmt.Sprintf(`{"type":"message_start","message":{"id":%q,"type":"message","role":"assistant","model":"claude-3-haiku","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`, id)}
	for i, raw := range blocks {
		var block struct {
			Type      string
			Text      string
			Citations []json.RawMessage
			Input     json.RawMessage
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &block))
		start := raw
		switch block.Type {
		case "text":
			start = `{"type":"text","text":""}`
		case "tool_use":
			start = `{"type":"tool_use","name":"json","id":"toolu_json","input":{}}`
		}
		events = append(events, fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":%s}`, i, start))
		switch block.Type {
		case "text":
			mid := len(block.Text) / 2
			for _, text := range []string{block.Text[:mid], block.Text[mid:]} {
				events = append(events, fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":%q}}`, i, text))
			}
			for _, citation := range block.Citations {
				events = append(events, fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"citations_delta","citation":%s}}`, i, citation))
			}
		case "tool_use":
			events = append(events, fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%q}}`, i, string(block.Input)))
		}
		events = append(events, fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, i))
	}
	return append(events,
		fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"output_tokens":1}}`, reason),
		`{"type":"message_stop"}`,
	)
}
