package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	compactionTestBeta    = "compact-2026-09-04"
	compactionTestSummary = "Résumé  \n"
	compactionTestBlock   = `{"type":"compaction","content":"Résumé  \n","signature":"opaque+/="}`
	compactionTestWire    = `{"type":"compaction","content":"Résumé  \n","signature":"opaque+/=","Content":"other","Signature":"not a signature"}`
	compactionTestMeta    = `{"type":"compaction","signature":"opaque+/="}`
	compactionTestStart   = `{"type":"message_start","message":{"id":"msg_summary","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}`
	compactionTestDelta   = `{"type":"message_delta","delta":{"stop_reason":"compaction"},"usage":{"output_tokens":0}}`
	compactionTestStop    = `{"type":"message_stop"}`
)

func TestModel_CompactionRequest(t *testing.T) {
	limit := 64
	cases := []struct {
		name string
		opts provider.ProviderOptions
		want string
		beta string
		err  string
	}{
		{name: "omitted"},
		{name: "typed without instructions", opts: provider.BuildProviderOptions(AnthropicOptions{Compaction: &CompactionConfig{Type: CompactionSummarize}}), want: `{"type":"summarize"}`, beta: compactionTestBeta},
		{name: "invalid typed config", opts: provider.BuildProviderOptions(AnthropicOptions{Compaction: &CompactionConfig{}}), err: "compaction.type"},
		{name: "with instructions", opts: makeProviderOpts(`{"compaction":{"type":"summarize","instructions":"Keep decisions."}}`), want: `{"type":"summarize","instructions":"Keep decisions."}`, beta: compactionTestBeta},
		{name: "empty instructions", opts: makeProviderOpts(`{"compaction":{"type":"summarize","instructions":""},"betas":["compact-2026-09-04","other-beta"]}`), want: `{"type":"summarize","instructions":""}`, beta: compactionTestBeta + ",other-beta"},
		{name: "invalid type", opts: makeProviderOpts(`{"compaction":{"type":"threshold"}}`), err: "compaction.type"},
		{name: "missing type", opts: makeProviderOpts(`{"compaction":{}}`), err: "compaction.type"},
		{name: "null compaction", opts: makeProviderOpts(`{"compaction":null}`), err: "compaction"},
		{name: "null instructions", opts: makeProviderOpts(`{"compaction":{"type":"summarize","instructions":null}}`), err: "compaction.instructions"},
		{name: "invalid instructions", opts: makeProviderOpts(`{"compaction":{"type":"summarize","instructions":42}}`), err: "instructions"},
		{name: "context edit conflict", opts: makeProviderOpts(`{"compaction":{"type":"summarize"},"contextManagement":{"edits":[{"type":"clear_tool_uses_20250919"}]}}`), err: "`compaction` and `contextManagement` cannot be used together"},
		{name: "empty context conflict", opts: makeProviderOpts(`{"compaction":{"type":"summarize"},"contextManagement":{"edits":[]}}`), err: "`compaction` and `contextManagement` cannot be used together"},
		{name: "context option is not independently consumed", opts: makeProviderOpts(`{"contextManagement":{"edits":[]}}`)},
	}

	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					model, requests := compactionTestModel(t, func(call int) (string, string) {
						if mode == "stream" {
							return "text/event-stream", transportSSE([]string{compactionTestStart, compactionTestDelta, compactionTestStop})
						}
						return "application/json", compactionUnary("")
					})
					opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("summarize")}, ProviderOptions: tc.opts, MaxOutputTokens: &limit}
					var err error
					if mode == "generate" {
						_, err = model.DoGenerate(t.Context(), opts)
					} else {
						var result *provider.StreamResult
						result, err = model.DoStream(t.Context(), opts)
						if result != nil {
							for part := range result.Stream {
								assert.NotEqual(t, provider.PartError, part.Type)
							}
						}
					}
					if tc.err != "" {
						require.ErrorContains(t, err, tc.err)
						assert.Empty(t, *requests)
						return
					}
					require.NoError(t, err)
					require.Len(t, *requests, 1)
					req := (*requests)[0]
					assert.NotContains(t, req.body, "context_management")
					assert.Equal(t, tc.beta, req.beta)
					if tc.want == "" {
						assert.NotContains(t, req.body, "compaction")
					} else {
						assert.JSONEq(t, tc.want, string(req.body["compaction"]))
					}
				})
			}
		})
	}
}

func TestModel_CompactionRoundTrip(t *testing.T) {
	limit := 64
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			model, requests := compactionTestModel(t, func(call int) (string, string) {
				if call == 2 {
					return "application/json", compactionUnary(`{"type":"text","text":"continued"}`)
				}
				if mode == "generate" {
					return "application/json", compactionUnary(compactionTestWire)
				}
				return "text/event-stream", transportSSE([]string{
					compactionTestStart,
					`{"type":"content_block_start","index":0,"content_block":` + compactionTestWire + `}`,
					`{"type":"content_block_delta","index":0,"delta":{"type":"compaction_delta","content":"Résumé  \n"}}`,
					`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"should not be appended"}}`,
					`{"type":"content_block_stop","index":0}`,
					compactionTestDelta, compactionTestStop,
				})
			})
			options := makeProviderOpts(`{"compaction":{"type":"summarize"}}`)
			user := provider.UserText("summarize")
			var history []provider.Message
			if mode == "generate" {
				result, err := model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{user}, ProviderOptions: options, MaxOutputTokens: &limit})
				require.NoError(t, err)
				require.Len(t, result.Content, 1)
				assert.Equal(t, provider.ContentText, result.Content[0].Type)
				assert.Equal(t, compactionTestSummary, result.Content[0].Text)
				assert.JSONEq(t, compactionTestMeta, string(result.Content[0].ProviderMetadata["anthropic"]))
				assert.Equal(t, provider.FinishReasonOther, result.FinishReason.Unified)
				part := provider.TextPart(result.Content[0].Text)
				part.ProviderOptions = makeProviderOpts(string(result.Content[0].ProviderMetadata["anthropic"]))
				history = aisdk.ToResponseMessages([]provider.ContentPart{part})
			} else {
				result := aisdk.StreamText(t.Context(), model, aisdk.WithModelMessages(user), aisdk.WithProviderOptions(options["anthropic"]))
				var deltas []string
				for part := range result.FullStream() {
					switch part := part.(type) {
					case aisdk.StreamTextStart:
						assert.JSONEq(t, compactionTestMeta, string(part.ProviderMetadata["anthropic"]))
					case aisdk.StreamTextDelta:
						deltas = append(deltas, part.Text)
					}
				}
				require.NoError(t, result.Err())
				assert.Equal(t, []string{compactionTestSummary}, deltas)
				assert.Equal(t, provider.FinishReasonOther, result.FinishReason().Unified)
				history = result.ResponseMessages()
			}
			require.Len(t, history, 1)
			result, err := model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{user, history[0], provider.UserText("continue")}, MaxOutputTokens: &limit})
			require.NoError(t, err)
			require.Len(t, result.Content, 1)
			assert.Equal(t, "continued", result.Content[0].Text)
			require.Len(t, *requests, 2)
			assert.JSONEq(t, `{"type":"summarize"}`, string((*requests)[0].body["compaction"]))
			assert.Equal(t, compactionTestBeta, (*requests)[0].beta)
			req := (*requests)[1]
			assert.Equal(t, compactionTestBeta, req.beta)
			assert.NotContains(t, req.body, "compaction")
			assert.JSONEq(t, `[{"role":"user","content":[{"type":"text","text":"summarize"}]},
				{"role":"assistant","content":[`+compactionTestBlock+`]},
				{"role":"user","content":[{"type":"text","text":"continue"}]}]`, string(req.body["messages"]))
		})
	}
}

func TestModel_CompactionLegacy(t *testing.T) {
	model, _ := compactionTestModel(t, func(call int) (string, string) {
		if call == 1 {
			return "application/json", compactionUnary(`
				{"type":"compaction","content":"","signature":"empty"},
				{"type":"compaction","content":null,"signature":"null"},
				{"type":"compaction","signature":"missing"},
				{"type":"compaction","content":"legacy","Signature":"ignored"},
				{"type":"compaction","content":"empty signature","signature":""},
				{"type":"text","text":"normal"}`)
		}
		return "text/event-stream", transportSSE([]string{
			compactionTestStart,
			`{"type":"content_block_start","index":0,"content_block":{"type":"compaction","content":"not authoritative","signature":null,"Signature":"ignored"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"compaction_delta","content":"split ","Content":"ignored"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"compaction_delta","content":null}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"compaction_delta","content":"summary"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"compaction","content":"","signature":""}}`,
			`{"type":"content_block_stop","index":1}`,
			compactionTestDelta, compactionTestStop,
		})
	})
	limit := 64
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, MaxOutputTokens: &limit}
	unary, err := model.DoGenerate(t.Context(), options)
	require.NoError(t, err)
	require.Len(t, unary.Content, 3)
	assert.Equal(t, "legacy", unary.Content[0].Text)
	assert.JSONEq(t, `{"type":"compaction"}`, string(unary.Content[0].ProviderMetadata["anthropic"]))
	assert.Equal(t, "empty signature", unary.Content[1].Text)
	assert.JSONEq(t, `{"type":"compaction","signature":""}`, string(unary.Content[1].ProviderMetadata["anthropic"]))
	assert.Equal(t, "normal", unary.Content[2].Text)
	assert.Empty(t, unary.Content[2].ProviderMetadata)

	streamed, err := model.DoStream(t.Context(), options)
	require.NoError(t, err)
	var textParts []provider.StreamPart
	for part := range streamed.Stream {
		assert.NotEqual(t, provider.PartError, part.Type)
		switch part.Type {
		case provider.PartTextStart, provider.PartTextDelta, provider.PartTextEnd:
			textParts = append(textParts, part)
		}
	}
	require.Len(t, textParts, 6)
	assert.Equal(t, provider.PartTextStart, textParts[0].Type)
	assert.JSONEq(t, `{"type":"compaction"}`, string(textParts[0].ProviderMetadata["anthropic"]))
	assert.Equal(t, provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "split "}, textParts[1])
	assert.Equal(t, provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "summary"}, textParts[2])
	assert.Equal(t, provider.PartTextEnd, textParts[3].Type)
	assert.Equal(t, provider.PartTextStart, textParts[4].Type)
	assert.JSONEq(t, `{"type":"compaction","signature":""}`, string(textParts[4].ProviderMetadata["anthropic"]))
	assert.Equal(t, provider.PartTextEnd, textParts[5].Type)
}

func TestModel_CompactionRejectsMalformedNativeFields(t *testing.T) {
	const blockStart = `{"type":"content_block_start","index":0,"content_block":`
	const blockStop = `{"type":"content_block_stop","index":0}`
	limit := 64
	opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("summarize")}, MaxOutputTokens: &limit}
	for _, tc := range []struct {
		name, block, field string
	}{
		{"numeric signature", `{"type":"compaction","content":"summary","signature":42}`, "signature"},
		{"boolean signature", `{"type":"compaction","content":"summary","signature":true}`, "signature"},
		{"numeric summary", `{"type":"compaction","content":42,"signature":"opaque"}`, "content"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("generate", func(t *testing.T) {
				model, _ := compactionTestModel(t, func(int) (string, string) {
					return "application/json", compactionUnary(tc.block)
				})
				result, err := model.DoGenerate(t.Context(), opts)
				require.ErrorContains(t, err, tc.field)
				assert.Nil(t, result)
			})
			t.Run("stream", func(t *testing.T) {
				model, _ := compactionTestModel(t, func(int) (string, string) {
					return "text/event-stream", transportSSE([]string{
						compactionTestStart, blockStart + tc.block + `}`, blockStop, compactionTestDelta, compactionTestStop,
					})
				})
				assertCompactionStreamError(t, model, opts, tc.field)
			})
		})
	}

	t.Run("numeric summary delta", func(t *testing.T) {
		model, _ := compactionTestModel(t, func(int) (string, string) {
			return "text/event-stream", transportSSE([]string{
				compactionTestStart,
				blockStart + `{"type":"compaction","content":null}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"compaction_delta","content":42}}`,
				blockStop, compactionTestDelta, compactionTestStop,
			})
		})
		assertCompactionStreamError(t, model, opts, "content")
	})
}

func assertCompactionStreamError(t *testing.T, model provider.LanguageModel, opts provider.CallOptions, field string) {
	t.Helper()
	result, err := model.DoStream(t.Context(), opts)
	require.NoError(t, err)
	var errs []*provider.APICallError
	for part := range result.Stream {
		switch part.Type {
		case provider.PartTextStart:
			assert.NotContains(t, string(part.ProviderMetadata["anthropic"]), `"signature"`)
		case provider.PartTextDelta, provider.PartFinish:
			assert.Fail(t, "malformed compaction emitted text or completed", "%+v", part)
		case provider.PartError:
			errs = append(errs, part.APICallError)
		}
	}
	require.Len(t, errs, 1)
	require.ErrorContains(t, errs[0], field)
}

func TestBuildParams_CompactionReplaySignaturePresence(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, text, block string
		signed                      bool
	}{
		{"unsigned", `{"type":"compaction","cacheControl":{"type":"ephemeral"}}`, "summary  \n", `{"type":"compaction","content":"summary  \n","cache_control":{"type":"ephemeral"}}`, false},
		{"null", `{"type":"compaction","signature":null}`, "summary", `{"type":"compaction","content":"summary"}`, false},
		{"nonstring", `{"type":"compaction","signature":3}`, "summary", `{"type":"compaction","content":"summary"}`, false},
		{"empty signature", `{"type":"compaction","signature":""}`, "summary", `{"type":"compaction","content":"summary","signature":""}`, true},
		{"case variant cannot override signature", `{"type":"compaction","signature":"opaque","Signature":"ignored"}`, "summary", `{"type":"compaction","content":"summary","signature":"opaque"}`, true},
		{"case variant cannot override type", `{"type":"compaction","Type":"other"}`, "summary", `{"type":"compaction","content":"summary"}`, false},
		{"case variant is not signed", `{"type":"compaction","Signature":"ignored"}`, "summary", `{"type":"compaction","content":"summary"}`, false},
		{"case variant is not compaction", `{"Type":"compaction","signature":"ignored"}`, "summary", `{"type":"text","text":"summary","citations":[]}`, false},
		{"empty summary", `{"type":"compaction","signature":"sig"}`, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.TextPart(tc.text)
			part.ProviderOptions = makeProviderOpts(tc.metadata)
			params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("summarize"), provider.NewAssistantMessage(part)},
			}, false)
			require.NoError(t, err)
			if tc.signed {
				require.Len(t, params.Betas, 1)
				assert.Equal(t, compactionTestBeta, string(params.Betas[0]))
			} else {
				assert.Empty(t, params.Betas)
			}
			if tc.block == "" {
				require.Len(t, params.Messages, 1)
				return
			}
			require.Len(t, params.Messages, 2)
			require.Len(t, params.Messages[1].Content, 1)
			encoded, err := json.Marshal(params.Messages[1].Content[0])
			require.NoError(t, err)
			assert.JSONEq(t, tc.block, string(encoded))
		})
	}
}

type compactionCapturedRequest struct {
	body map[string]json.RawMessage
	beta string
}

func compactionTestModel(t *testing.T, respond func(int) (contentType, body string)) (provider.LanguageModel, *[]compactionCapturedRequest) {
	t.Helper()
	var requests []compactionCapturedRequest
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		requests = append(requests, compactionCapturedRequest{body: body, beta: r.Header.Get("anthropic-beta")})
		contentType, response := respond(len(requests))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	return New("dummy", "claude-sonnet-4-6", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0))), &requests
}

func compactionUnary(blocks string) string {
	return `{"id":"msg_summary","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[` + blocks + `],"stop_reason":"compaction","usage":{"input_tokens":0,"output_tokens":0}}`
}
