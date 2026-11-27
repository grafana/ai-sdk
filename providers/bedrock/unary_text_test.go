package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SYNTHETIC, not recorded or imported fixtures. The citation-only and signed
// reasoning cases adapt the inline mocks in @ai-sdk/amazon-bedrock@5.0.146
// (3dad57f204), amazon-bedrock-chat-language-model.test.ts:4321 and :7568.
func TestDoGenerate_SyntheticTextProjection(t *testing.T) {
	const modelID = "anthropic.claude-sonnet-5-5"
	text := func(s string) provider.GenerateContentPart {
		return provider.GenerateContentPart{Type: provider.ContentText, Text: s}
	}
	reasoning := func(s, signature string) provider.GenerateContentPart {
		metadata := json.RawMessage(`{"signature":"` + signature + `"}`)
		return provider.GenerateContentPart{
			Type: provider.ContentReasoning,
			Text: s,
			ProviderMetadata: provider.ProviderMetadata{
				"amazonBedrock": metadata,
				"bedrock":       metadata,
			},
		}
	}

	for _, tc := range []struct {
		name            string
		content         string
		jsonInstruction bool
		want            []provider.GenerateContentPart
	}{
		{
			name:    "citation_only",
			content: `[{"citationsContent":{"content":[{"text":"Citation "},{"text":"response"}],"citations":[]}}]`,
			want:    []provider.GenerateContentPart{text("Citation "), text("response")},
		},
		{
			name: "empty_text_between_signed_reasoning",
			content: `[
				{"reasoningContent":{"reasoningText":{"text":"thinking...","signature":"sig-1"}}},
				{"text":""},
				{"reasoningContent":{"reasoningText":{"text":"more thinking...","signature":"sig-2"}}}
			]`,
			want: []provider.GenerateContentPart{reasoning("thinking...", "sig-1"), text(""), reasoning("more thinking...", "sig-2")},
		},
		{
			name: "direct_text_wins_even_when_empty",
			content: `[
				{"text":"Canonical response","citationsContent":{"content":[{"text":"Duplicate response"}]}},
				{"text":"","citationsContent":{"content":[{"text":"Not a fallback"}]}}
			]`,
			want: []provider.GenerateContentPart{text("Canonical response"), text("")},
		},
		{
			name: "null_falls_back_and_only_present_fragments_emit",
			content: `[
				{"text":null,"citationsContent":{"content":[{}, {"text":"first"}, {"text":null}, {"text":""}, {"text":"last"}]}},
				{"citationsContent":{"content":null}},
				{"citationsContent":{}},
				{"citationsContent":null},
				{"text":null}
			]`,
			want: []provider.GenerateContentPart{text("first"), text(""), text("last")},
		},
		{
			name: "json_instruction_shares_extractor_across_direct_text_and_citations",
			content: `[
				{"text":"Intro: "},
				{"citationsContent":{"content":[{"text":"{\"value\":\""},{"text":"ok }"}]}},
				{"text":" still a string\"} trailing prose"}
			]`,
			jsonInstruction: true,
			want:            []provider.GenerateContentPart{text(""), text(`{"value":"`), text("ok }"), text(` still a string"}`)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"output":{"message":{"role":"assistant","content":` + tc.content + `}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":2,"totalTokens":3}}`
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/model/"+modelID+"/converse", r.URL.Path)
				_ = r.Body.Close()
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			model := New(modelID, WithRegion("us-east-1"), WithBaseURL("https://bedrock.invalid"), WithBearerToken("dummy"), WithHTTPClient(client))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Answer")}}
			if tc.jsonInstruction {
				opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}
				opts.ProviderOptions = provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: StructuredOutputModeJSONTool})
			}
			result, err := model.DoGenerate(context.Background(), opts)
			require.NoError(t, err)
			if tc.jsonInstruction {
				require.Contains(t, string(result.Request.Body), "You MUST answer with only a JSON object")
			}
			require.Len(t, result.Content, len(tc.want))
			assert.Equal(t, tc.want, result.Content)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
