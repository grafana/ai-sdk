package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const astraEfforts = "gpt-6-astra only supports the following reasoning efforts: low, medium, high, xhigh, max"

var (
	effortUser     = provider.UserText("Question")
	wireEffortUser = map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Question"}}}
)

func effortUpdate(effort OpenAIReasoningEffortUpdate) provider.Message {
	msg := provider.NewSystemMessage("")
	msg.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ReasoningEffortUpdate: effort})
	return msg
}

func wireEffortUpdate(effort string) map[string]any {
	return map[string]any{"type": "configuration_update", "reasoning": map[string]any{"effort": effort}}
}

func storedReasoning() provider.Message {
	part := provider.ReasoningPart("Earlier reasoning")
	part.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "rs_previous"})
	return provider.NewAssistantMessage(part)
}

func buildEffortUpdateBody(model string, prompt []provider.Message, providerOptions provider.ProviderOptions, namespace string) (map[string]any, []provider.Warning, error) {
	params, warnings, _, err := buildParamsForProvider(model, provider.CallOptions{Prompt: prompt, ProviderOptions: providerOptions}, namespace)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, nil, err
	}
	var body map[string]any
	return body, warnings, json.Unmarshal(encoded, &body)
}

func TestBuildParams_PositionedReasoningEffortUpdates(t *testing.T) {
	high, low, none := OpenAIReasoningEffortUpdateHigh, OpenAIReasoningEffortUpdateLow, OpenAIReasoningEffortUpdateNone
	nullSummary, conciseSummary := json.RawMessage(`null`), json.RawMessage(`"concise"`)
	store := false
	trigger := true
	removed := provider.NewSystemMessage("Removed")
	removedWarning := provider.Warning{Type: provider.WarnOther, Feature: "system", Message: "system messages are removed for this model"}
	malformedPartOptions := provider.NewSystemMessage("")
	malformedPartOptions.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"promptCacheBreakpoint":"x","itemId":1}`)}}

	for _, tc := range []struct {
		name          string
		model         string
		prompt        []provider.Message
		options       OpenAIResponsesOptions
		wantInput     []any
		wantReasoning map[string]any
		wantWarnings  []provider.Warning
	}{
		{name: "astra retains multiple updates", model: "gpt-6-astra", prompt: []provider.Message{effortUser, provider.NewAssistantMessage(provider.TextPart("Answer")), effortUpdate(high), effortUser, effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "medium", Store: &store}, wantInput: []any{wireEffortUser, map[string]any{"role": "assistant", "content": "Answer"}, wireEffortUpdate("high"), wireEffortUser, wireEffortUpdate("low"), wireEffortUser}, wantReasoning: map[string]any{"effort": "medium", "summary": "detailed"}},
		{name: "sol retains multiple updates", model: "gpt-6-sol", prompt: []provider.Message{effortUser, effortUpdate(high), effortUser, effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "medium"}, wantInput: []any{wireEffortUser, wireEffortUpdate("high"), wireEffortUser, wireEffortUpdate("low"), wireEffortUser}, wantReasoning: map[string]any{"effort": "medium", "summary": "detailed"}},
		{name: "luna retains multiple updates", model: "gpt-6-luna", prompt: []provider.Message{effortUser, effortUpdate(high), effortUser, effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "medium"}, wantInput: []any{wireEffortUser, wireEffortUpdate("high"), wireEffortUser, wireEffortUpdate("low"), wireEffortUser}, wantReasoning: map[string]any{"effort": "medium", "summary": "detailed"}},
		{name: "luna preserves positioned none updates", model: "gpt-6-luna", prompt: []provider.Message{effortUser, effortUpdate(none), effortUser, effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "low", ReasoningSummary: nullSummary}, wantInput: []any{wireEffortUser, wireEffortUpdate("none"), wireEffortUser, wireEffortUpdate("low"), wireEffortUser}, wantReasoning: map[string]any{"effort": "low"}},
		{name: "sol prepends request-level none update", model: "gpt-6-sol", prompt: []provider.Message{effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "low", ReasoningEffortUpdate: none, ReasoningSummary: nullSummary}, wantInput: []any{wireEffortUpdate("none"), wireEffortUser}, wantReasoning: map[string]any{"effort": "low"}},
		{name: "luna deduplicates matching first none update", model: "gpt-6-luna", prompt: []provider.Message{effortUpdate(none), effortUser}, options: OpenAIResponsesOptions{ReasoningEffortUpdate: none}, wantInput: []any{wireEffortUpdate("none"), wireEffortUser}},
		{name: "astra omits request-level none update and keeps history", model: "gpt-6-astra", prompt: []provider.Message{effortUser, effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "medium", ReasoningEffortUpdate: none, ReasoningSummary: nullSummary}, wantInput: []any{wireEffortUser, wireEffortUpdate("low"), wireEffortUser}, wantReasoning: map[string]any{"effort": "medium"}, wantWarnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: "reasoningEffortUpdate", Details: astraEfforts}}},
		{name: "request-level update prepends before positioned history", model: "gpt-6-astra", prompt: []provider.Message{effortUser, effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "low", ReasoningEffortUpdate: OpenAIReasoningEffortUpdateMedium, ReasoningSummary: nullSummary}, wantInput: []any{wireEffortUpdate("medium"), wireEffortUser, wireEffortUpdate("high"), wireEffortUser}, wantReasoning: map[string]any{"effort": "low"}},
		{name: "identical first update is not duplicated", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "low", ReasoningSummary: conciseSummary, ReasoningEffortUpdate: high}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}, wantReasoning: map[string]any{"effort": "low", "summary": "concise"}},
		{name: "first item is checked after system text removal", model: "gpt-6-astra", prompt: []provider.Message{removed, effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{ReasoningEffortUpdate: high, SystemMessageMode: "remove"}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}, wantWarnings: []provider.Warning{removedWarning}},
		{name: "identical later update still prepends", model: "gpt-6-astra", prompt: []provider.Message{effortUser, effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{ReasoningEffortUpdate: high}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser, wireEffortUpdate("high"), wireEffortUser}},
		{name: "request-level continuation", model: "gpt-6-astra", prompt: []provider.Message{effortUser}, options: OpenAIResponsesOptions{PreviousResponseID: "resp_previous", ReasoningEffort: "low", ReasoningEffortUpdate: high, ReasoningSummary: nullSummary}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}, wantReasoning: map[string]any{"effort": "low"}},
		{name: "update kept after stored reasoning filtered by previous response", model: "gpt-6-astra", prompt: []provider.Message{storedReasoning(), effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{PreviousResponseID: "resp_previous", ReasoningEffort: "low", ReasoningSummary: nullSummary}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}, wantReasoning: map[string]any{"effort": "low"}},
		{name: "update kept after stored reasoning filtered by conversation", model: "gpt-6-astra", prompt: []provider.Message{storedReasoning(), effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{Conversation: "conv_test", ReasoningEffort: "low", ReasoningSummary: nullSummary}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}, wantReasoning: map[string]any{"effort": "low"}},
		{name: "system mode system", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{SystemMessageMode: "system"}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}},
		{name: "system mode developer", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{SystemMessageMode: "developer"}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}},
		{name: "system mode remove", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{SystemMessageMode: "remove"}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser}},
		{name: "unrelated malformed system part options are ignored", model: "gpt-6-astra", prompt: []provider.Message{malformedPartOptions, effortUser}, wantInput: []any{map[string]any{"role": "developer", "content": ""}, wireEffortUser}},
		{name: "effort warning precedes update warning", model: "gpt-6-astra", prompt: []provider.Message{effortUser}, options: OpenAIResponsesOptions{ReasoningEffort: "none", ReasoningEffortUpdate: none}, wantInput: []any{wireEffortUser}, wantWarnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: "reasoningEffort", Details: astraEfforts}, {Type: provider.WarnUnsupported, Feature: "reasoningEffortUpdate", Details: astraEfforts}}},
		{name: "ordinary empty system message", model: "gpt-6-astra", prompt: []provider.Message{provider.NewSystemMessage(""), effortUser}, wantInput: []any{map[string]any{"role": "developer", "content": ""}, wireEffortUser}},
		{name: "explicit compaction with standard mode and disabled truncation", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{ReasoningMode: "standard", Truncation: "disabled", CompactionTrigger: &trigger}, wantInput: []any{wireEffortUpdate("high"), wireEffortUser, map[string]any{"type": "compaction_trigger"}}, wantReasoning: map[string]any{"mode": "standard"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, warnings, err := buildEffortUpdateBody(tc.model, tc.prompt, withOpenAIOptions(tc.options), "")
			require.NoError(t, err)
			assert.Equal(t, tc.wantInput, body["input"])
			if tc.wantReasoning == nil {
				assert.NotContains(t, body, "reasoning")
			} else {
				assert.Equal(t, tc.wantReasoning, body["reasoning"])
			}
			assert.Equal(t, tc.wantWarnings, warnings)
		})
	}
}

func TestBuildParams_ReasoningEffortUpdateSampling(t *testing.T) {
	low, none := OpenAIReasoningEffortUpdateLow, OpenAIReasoningEffortUpdateNone
	logprobs := int64(2)
	temperature, topP := 0.0, 0.9
	restrictedWarnings := []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: "temperature", Details: "temperature is not supported for reasoning models"},
		{Type: provider.WarnUnsupported, Feature: "topP", Details: "topP is not supported for reasoning models"},
		{Type: provider.WarnUnsupported, Feature: "logprobs", Details: "logprobs is not supported for reasoning models"},
	}

	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		for _, tc := range []struct {
			name         string
			prompt       []provider.Message
			effort       string
			update       OpenAIReasoningEffortUpdate
			wantSampling bool
		}{
			{name: "request update enables reasoning", prompt: []provider.Message{effortUser}, effort: "none", update: low},
			{name: "request update disables reasoning", prompt: []provider.Message{effortUser}, effort: "low", update: none, wantSampling: true},
			{name: "positioned update enables reasoning", prompt: []provider.Message{effortUser, effortUpdate(low), effortUser}, effort: "none"},
			{name: "last positioned update disables reasoning", prompt: []provider.Message{effortUser, effortUpdate(low), effortUser, effortUpdate(none), effortUser}, effort: "none", wantSampling: true},
			{name: "positioned update overrides prepended request update", prompt: []provider.Message{effortUser, effortUpdate(low), effortUser}, effort: "low", update: none},
			{name: "positioned update disables default reasoning", prompt: []provider.Message{effortUpdate(none), effortUser}, wantSampling: true},
		} {
			t.Run(model+"/"+tc.name, func(t *testing.T) {
				body, warnings, _, err := buildParams(model, provider.CallOptions{
					Prompt: tc.prompt, Temperature: &temperature, TopP: &topP,
					ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
						ReasoningEffort: tc.effort, ReasoningEffortUpdate: tc.update, ReasoningSummary: json.RawMessage(`null`),
						Logprobs: &LogprobsOption{Int: &logprobs}, Include: []string{"message.output_text.logprobs", "reasoning.encrypted_content"},
					}),
				})
				require.NoError(t, err)
				assert.Equal(t, tc.effort, string(body.Reasoning.Effort))
				if tc.wantSampling {
					assert.Equal(t, temperature, body.Temperature.Value)
					assert.Equal(t, topP, body.TopP.Value)
					assert.Equal(t, logprobs, body.TopLogprobs.Value)
					assert.ElementsMatch(t, []string{"message.output_text.logprobs", "reasoning.encrypted_content"}, includeStrings(body.Include))
					assert.Empty(t, warnings)
					return
				}
				assert.False(t, body.Temperature.Valid())
				assert.False(t, body.TopP.Valid())
				assert.False(t, body.TopLogprobs.Valid())
				assert.Equal(t, []string{"reasoning.encrypted_content"}, includeStrings(body.Include))
				assert.Equal(t, restrictedWarnings, warnings)
			})
		}
	}
}

func TestBuildParams_RejectedReasoningEffortUpdates(t *testing.T) {
	high, low, none := OpenAIReasoningEffortUpdateHigh, OpenAIReasoningEffortUpdateLow, OpenAIReasoningEffortUpdateNone
	compactThreshold := int64(1000)
	withText := func(content string) provider.Message {
		msg := provider.NewSystemMessage(content)
		msg.ProviderOptions = effortUpdate(high).ProviderOptions
		return msg
	}
	removed := provider.NewSystemMessage("Removed")

	type rejection struct {
		name    string
		model   string
		prompt  []provider.Message
		options OpenAIResponsesOptions
		wantErr string
	}
	cases := []rejection{
		{name: "astra historical none update", model: "gpt-6-astra", prompt: []provider.Message{effortUser, effortUpdate(none), effortUser}, wantErr: astraEfforts},
		{name: "mixed text", model: "gpt-6-astra", prompt: []provider.Message{withText("Instructions"), effortUser}, wantErr: configurationUpdateNonEmptySystemMessage},
		{name: "whitespace text", model: "gpt-6-astra", prompt: []provider.Message{withText(" "), effortUser}, wantErr: configurationUpdateNonEmptySystemMessage},
		{name: "mantle sol", model: "openai.gpt-6-sol", prompt: []provider.Message{effortUpdate(high), effortUser}, wantErr: configurationUpdateModelUnsupported},
		{name: "adjacent positioned updates", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUpdate(low), effortUser}, wantErr: configurationUpdateAdjacent},
		{name: "adjacent identical positioned updates", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUpdate(high), effortUser}, wantErr: configurationUpdateAdjacent},
		{name: "adjacent identical positioned updates with request update", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{ReasoningEffortUpdate: high}, wantErr: configurationUpdateAdjacent},
		{name: "request update adjacent to first positioned update", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{ReasoningEffortUpdate: low}, wantErr: configurationUpdateAdjacent},
		{name: "adjacent after system text removal", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), removed, effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{SystemMessageMode: "remove"}, wantErr: configurationUpdateAdjacent},
		{name: "adjacent identical after system text removal", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), removed, effortUpdate(high), effortUser}, options: OpenAIResponsesOptions{SystemMessageMode: "remove"}, wantErr: configurationUpdateAdjacent},
		{name: "adjacent after previous response filters stored reasoning", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), storedReasoning(), effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{PreviousResponseID: "resp_previous"}, wantErr: configurationUpdateAdjacent},
		{name: "adjacent after conversation filters stored reasoning", model: "gpt-6-astra", prompt: []provider.Message{effortUpdate(high), storedReasoning(), effortUpdate(low), effortUser}, options: OpenAIResponsesOptions{Conversation: "conv_test"}, wantErr: configurationUpdateAdjacent},
	}
	force := true
	for _, unsupported := range []struct {
		name    string
		model   string
		options OpenAIResponsesOptions
		reason  string
	}{
		{name: "gpt-5.6", model: "gpt-5.6", reason: configurationUpdateModelUnsupported},
		{name: "forced custom model", model: "custom-model", options: OpenAIResponsesOptions{ForceReasoning: &force}, reason: configurationUpdateModelUnsupported},
		{name: "pro mode", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningMode: "pro"}, reason: configurationUpdateConfigurationUnsupported},
		{name: "auto truncation", model: "gpt-6-astra", options: OpenAIResponsesOptions{Truncation: "auto"}, reason: configurationUpdateConfigurationUnsupported},
		{name: "automatic compaction", model: "gpt-6-astra", options: OpenAIResponsesOptions{ContextManagement: []ContextManagementEntry{{Type: "compaction", CompactThreshold: &compactThreshold}}}, reason: configurationUpdateConfigurationUnsupported},
		{name: "empty context management", model: "gpt-6-astra", options: OpenAIResponsesOptions{ContextManagement: []ContextManagementEntry{}}, reason: configurationUpdateConfigurationUnsupported},
	} {
		cases = append(cases, rejection{name: "unsupported configuration " + unsupported.name, model: unsupported.model, prompt: []provider.Message{effortUpdate(high), effortUser}, options: unsupported.options, wantErr: unsupported.reason})

		t.Run("request-level warning "+unsupported.name, func(t *testing.T) {
			options := unsupported.options
			options.ReasoningEffortUpdate = low
			body, warnings, err := buildEffortUpdateBody(unsupported.model, []provider.Message{effortUser}, withOpenAIOptions(options), "")
			require.NoError(t, err)
			assert.Equal(t, []any{wireEffortUser}, body["input"])
			assert.Contains(t, warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "reasoningEffortUpdate", Details: unsupported.reason})
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := buildEffortUpdateBody(tc.model, tc.prompt, withOpenAIOptions(tc.options), "")
			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}

	t.Run("non-string message-level update is invalid", func(t *testing.T) {
		msg := provider.NewSystemMessage("")
		msg.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"reasoningEffortUpdate":1}`)}}
		_, _, err := buildEffortUpdateBody("gpt-6-luna", []provider.Message{msg, effortUser}, nil, "")
		require.Error(t, err)
		assert.NotErrorIs(t, err, provider.ErrUnsupportedFunctionality)
		assert.ErrorContains(t, err, "system message options")
	})

	t.Run("minimal message-level update is invalid", func(t *testing.T) {
		_, _, err := buildEffortUpdateBody("gpt-6-luna", []provider.Message{effortUpdate("minimal"), effortUser}, nil, "")
		require.Error(t, err)
		assert.NotErrorIs(t, err, provider.ErrUnsupportedFunctionality)
		assert.ErrorContains(t, err, `invalid message-level reasoningEffortUpdate "minimal"`)
	})
}

func TestBuildParams_AzureMessageReasoningEffortUpdates(t *testing.T) {
	azureUpdate := func(providerOptions provider.ProviderOptions) []provider.Message {
		msg := provider.NewSystemMessage("")
		msg.ProviderOptions = providerOptions
		return []provider.Message{effortUser, msg, effortUser}
	}
	openAIHigh := OpenAIPartOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh}

	for _, tc := range []struct {
		name           string
		azure          map[string]any
		openAIFallback bool
		wantInput      any
	}{
		{name: "azure options", azure: map[string]any{"reasoningEffortUpdate": "high"}, wantInput: wireEffortUpdate("high")},
		{name: "openai options fallback", openAIFallback: true, wantInput: wireEffortUpdate("high")},
		{name: "azure options take precedence", azure: map[string]any{"reasoningEffortUpdate": "low"}, openAIFallback: true, wantInput: wireEffortUpdate("low")},
		{name: "explicit empty azure options", azure: map[string]any{}, openAIFallback: true, wantInput: map[string]any{"role": "developer", "content": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			providerOptions := provider.ProviderOptions{}
			if tc.azure != nil {
				providerOptions = withAzureOptions(t, tc.azure)
			}
			if tc.openAIFallback {
				providerOptions["openai"] = openAIHigh
			}
			body, _, err := buildEffortUpdateBody("gpt-6-astra", azureUpdate(providerOptions), nil, "azure")
			require.NoError(t, err)
			assert.Equal(t, []any{wireEffortUser, tc.wantInput, wireEffortUser}, body["input"])
		})
	}

	t.Run("unsupported azure configuration with openai message options", func(t *testing.T) {
		_, _, err := buildEffortUpdateBody("gpt-6-astra", azureUpdate(provider.BuildProviderOptions(openAIHigh)), withAzureOptions(t, map[string]any{"truncation": "auto"}), "azure")
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
		assert.ErrorContains(t, err, configurationUpdateConfigurationUnsupported)
	})
}

func TestResponsesPositionedReasoningEffortUpdates_GenerateAndStream(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prompt    []provider.Message
		wantInput []any
		wantErr   bool
	}{
		{name: "sends positioned updates", prompt: []provider.Message{effortUser, effortUpdate(OpenAIReasoningEffortUpdateNone), effortUser}, wantInput: []any{wireEffortUser, wireEffortUpdate("none"), wireEffortUser}},
		{name: "rejects before sending", prompt: []provider.Message{effortUpdate(OpenAIReasoningEffortUpdateHigh), effortUpdate(OpenAIReasoningEffortUpdateLow), effortUser}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies []map[string]any
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				data, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, json.Unmarshal(data, &body))
				bodies = append(bodies, body)
				contentType, payload := "application/json", `{"id":"resp_1","status":"completed","output":[]}`
				if body["stream"] == true {
					contentType = "text/event-stream"
					payload = "event: response.completed\n" + `data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_1","status":"completed","output":[]}}` + "\n\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
			})}
			model := NewResponses("test-key", "gpt-6-luna", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			options := provider.CallOptions{Prompt: tc.prompt}

			_, generateErr := model.DoGenerate(t.Context(), options)
			streamed, streamErr := model.DoStream(t.Context(), options)
			if tc.wantErr {
				require.ErrorIs(t, generateErr, provider.ErrUnsupportedFunctionality)
				require.ErrorIs(t, streamErr, provider.ErrUnsupportedFunctionality)
				assert.Empty(t, bodies)
				return
			}
			require.NoError(t, generateErr)
			require.NoError(t, streamErr)
			for range streamed.Stream {
			}
			require.Len(t, bodies, 2)
			for _, body := range bodies {
				assert.Equal(t, tc.wantInput, body["input"])
			}
		})
	}
}

func includeStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}
