package openai

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_GPT6ReasoningEffortCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		model    string
		effort   string
		fallback provider.ReasoningEffort
		want     string
		warn     bool
		force    bool
	}{
		{name: "supported option", model: "gpt-6-astra", effort: "max", want: "max"},
		{name: "unsupported option", model: "gpt-6-astra", effort: "none", warn: true},
		{name: "unsupported fallback", model: "gpt-7", fallback: provider.ReasoningMinimal, warn: true},
		{name: "supported fallback", model: "gpt-6", fallback: provider.ReasoningHigh, want: "high"},
		{name: "option precedes fallback", model: "gpt-6", effort: "minimal", fallback: provider.ReasoningHigh, warn: true},
		{name: "earlier model remains unchanged", model: "gpt-5.6", effort: "none", want: "none"},
		{name: "unrecognized ID forced reasoning", model: "gpt-6chat", effort: "minimal", want: "minimal", force: true},
		{name: "mantle override", model: "openai.gpt-6-astra", effort: "none", want: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := OpenAIResponsesOptions{ReasoningEffort: tc.effort}
			if tc.force {
				force := true
				options.ForceReasoning = &force
			}
			body, warnings := buildBody(t, tc.model, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.fallback,
				ProviderOptions: withOpenAIOptions(options),
			})
			assert.Equal(t, tc.effort, options.ReasoningEffort)
			if tc.want == "" {
				if reasoning, ok := body["reasoning"].(map[string]any); ok {
					assert.NotContains(t, reasoning, "effort")
				}
			} else {
				reasoning, ok := body["reasoning"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.want, reasoning["effort"])
			}
			if tc.warn {
				require.Contains(t, warningFeatures(warnings), "reasoningEffort")
				for _, warning := range warnings {
					if warning.Feature == "reasoningEffort" {
						assert.Equal(t, provider.WarnUnsupported, warning.Type)
						assert.Contains(t, warning.Details, "low, medium, high, xhigh, max")
						break
					}
				}
			} else {
				assert.NotContains(t, warningFeatures(warnings), "reasoningEffort")
			}
		})
	}
}

func TestBuildParams_ConfigurationUpdateAndCompactionTrigger(t *testing.T) {
	truth := true
	for _, tc := range []struct {
		name        string
		model       string
		options     OpenAIResponsesOptions
		wantUpdate  bool
		wantTrigger bool
		wantWarning bool
	}{
		{name: "supported update and trigger", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffort: "low", ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, CompactionTrigger: &truth, PreviousResponseID: "resp_1"}, wantUpdate: true, wantTrigger: true},
		{name: "old model still triggers", model: "gpt-5.6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, CompactionTrigger: &truth}, wantTrigger: true, wantWarning: true},
		{name: "pro rejects update", model: "gpt-6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, ReasoningMode: "pro"}, wantWarning: true},
		{name: "empty context management rejects update", model: "gpt-6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, ContextManagement: []ContextManagementEntry{}}, wantWarning: true},
		{name: "auto truncation rejects update", model: "gpt-6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, Truncation: "auto"}, wantWarning: true},
		{name: "mantle requires endpoint verification", model: "openai.gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh}, wantWarning: true},
		{name: "default request unchanged", model: "gpt-6", options: OpenAIResponsesOptions{}},
		{name: "explicit false has no trigger", model: "gpt-5.6", options: OpenAIResponsesOptions{CompactionTrigger: new(bool)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt := []provider.Message{provider.UserText("hi")}
			before, err := json.Marshal(prompt)
			require.NoError(t, err)
			body, warnings := buildBody(t, tc.model, provider.CallOptions{Prompt: prompt, ProviderOptions: withOpenAIOptions(tc.options)})
			after, err := json.Marshal(prompt)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			input := body["input"].([]any)
			if tc.wantUpdate {
				assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
				assert.Equal(t, "high", input[0].(map[string]any)["reasoning"].(map[string]any)["effort"])
				input = input[1:]
			}
			assert.Equal(t, "hi", input[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
			if tc.wantTrigger {
				assert.Equal(t, "compaction_trigger", input[len(input)-1].(map[string]any)["type"])
				input = input[:len(input)-1]
			}
			require.Len(t, input, 1)
			if tc.wantWarning {
				assert.Contains(t, warningFeatures(warnings), "reasoningEffortUpdate")
			} else {
				assert.NotContains(t, warningFeatures(warnings), "reasoningEffortUpdate")
			}
			if tc.options.PreviousResponseID != "" {
				assert.Equal(t, tc.options.PreviousResponseID, body["previous_response_id"])
				assert.Equal(t, "low", body["reasoning"].(map[string]any)["effort"])
			}
		})
	}
}

func TestBuildParams_AzureUsesOpenAIModelCapabilities(t *testing.T) {
	body, warnings, _, err := buildParamsForProvider("gpt-6-astra", provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ProviderOptions: withAzureOptions(t, map[string]any{"reasoningEffortUpdate": "high"}),
	}, "azure")
	require.NoError(t, err)
	assert.Empty(t, warnings)
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	var request map[string]any
	require.NoError(t, json.Unmarshal(encoded, &request))
	input := request["input"].([]any)
	assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
}

func TestBuildParams_InvalidReasoningEffortUpdate(t *testing.T) {
	_, _, _, err := buildParams("gpt-6", provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{ReasoningEffortUpdate: "minimal"}),
	})
	require.ErrorContains(t, err, "invalid reasoningEffortUpdate")
}

func TestBuildParams_CompactionTriggerFollowsHistory(t *testing.T) {
	trigger := true
	encrypted := "encrypted"
	for _, store := range []bool{true, false} {
		t.Run(map[bool]string{true: "stored", false: "stateless"}[store], func(t *testing.T) {
			compaction := provider.CustomPart("openai.compaction")
			compaction.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "cmp_1", EncryptedContent: &encrypted})
			body, warnings := buildBody(t, "gpt-6", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(compaction), provider.UserText("continue")},
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
					Store: &store, ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, CompactionTrigger: &trigger,
				}),
			})
			assert.Empty(t, warnings)
			input := body["input"].([]any)
			require.Len(t, input, 4)
			assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
			if store {
				assert.Equal(t, "item_reference", input[1].(map[string]any)["type"])
				assert.Equal(t, "cmp_1", input[1].(map[string]any)["id"])
			} else {
				assert.Equal(t, "compaction", input[1].(map[string]any)["type"])
				assert.Equal(t, "cmp_1", input[1].(map[string]any)["id"])
				assert.Equal(t, encrypted, input[1].(map[string]any)["encrypted_content"])
			}
			assert.Equal(t, "continue", input[2].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
			assert.Equal(t, "compaction_trigger", input[3].(map[string]any)["type"])
		})
	}
}

func TestResponsesCapabilityControls_GenerateAndStreamRequests(t *testing.T) {
	trigger := true
	logprobs := int64(5)
	for _, tc := range []struct {
		name, model              string
		options                  OpenAIResponsesOptions
		wantUpdate               bool
		wantNoLogprobs           bool
		wantPromptCacheRetention string
		wantWarning              string
	}{
		{name: "supported", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffort: "high", ReasoningEffortUpdate: OpenAIReasoningEffortUpdateLow, CompactionTrigger: &trigger}, wantUpdate: true},
		{name: "unsupported effort and update", model: "gpt-5.6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateLow}, wantWarning: "reasoningEffortUpdate"},
		{name: "invalid gpt6 effort", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffort: "none"}, wantWarning: "reasoningEffort"},
		{name: "gpt6 omits logprobs", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffort: "low", Logprobs: &LogprobsOption{Int: &logprobs}, Include: []string{"message.output_text.logprobs"}}, wantNoLogprobs: true, wantWarning: "logprobs"},
		{name: "gpt6 omits legacy prompt cache retention", model: "gpt-6-astra", options: OpenAIResponsesOptions{PromptCacheRetention: "24h"}, wantWarning: "promptCacheRetention"},
		{name: "gpt5 retains legacy prompt cache retention", model: "gpt-5.6", options: OpenAIResponsesOptions{PromptCacheRetention: "24h"}, wantPromptCacheRetention: "24h"},
		{name: "mantle retains legacy prompt cache retention", model: "openai.gpt-6-astra", options: OpenAIResponsesOptions{PromptCacheRetention: "24h"}, wantPromptCacheRetention: "24h"},
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
			model := NewResponses("test-key", tc.model, WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: withOpenAIOptions(tc.options)}
			generated, err := model.DoGenerate(t.Context(), options)
			require.NoError(t, err)
			streamed, err := model.DoStream(t.Context(), options)
			require.NoError(t, err)
			var streamWarnings []provider.Warning
			for part := range streamed.Stream {
				if part.Type == provider.PartStreamStart {
					streamWarnings = part.Warnings
				}
			}
			require.Len(t, bodies, 2)
			assert.Equal(t, generated.Warnings, streamWarnings)
			for _, body := range bodies {
				input := body["input"].([]any)
				if tc.wantUpdate {
					assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
					assert.Equal(t, "compaction_trigger", input[len(input)-1].(map[string]any)["type"])
					assert.Equal(t, "high", body["reasoning"].(map[string]any)["effort"])
				} else {
					assert.Contains(t, input[0].(map[string]any), "content")
					if tc.wantNoLogprobs {
						assert.NotContains(t, body, "top_logprobs")
						assert.NotContains(t, body, "include")
					}
					if tc.wantPromptCacheRetention == "" {
						assert.NotContains(t, body, "prompt_cache_retention")
					} else {
						assert.Equal(t, tc.wantPromptCacheRetention, body["prompt_cache_retention"])
					}
					if tc.wantWarning == "reasoningEffort" {
						if reasoning, ok := body["reasoning"].(map[string]any); ok {
							assert.NotContains(t, reasoning, "effort")
						}
					}
				}
			}
			if tc.wantWarning != "" {
				assert.Contains(t, warningFeatures(generated.Warnings), tc.wantWarning)
			} else {
				assert.NotContains(t, warningFeatures(generated.Warnings), "reasoningEffortUpdate")
			}
		})
	}
}

func TestBuildParams_CompactionReplayPresence(t *testing.T) {
	encrypted := "encrypted"
	raw := func(value string) provider.ProviderOptions {
		return provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(value)}}
	}
	tests := []struct {
		name         string
		options      provider.ProviderOptions
		store        bool
		conversation string
		want         []map[string]any
	}{
		{
			name:    "stateless with encrypted content",
			options: raw(`{"itemId":"cmp_1","encryptedContent":"encrypted"}`),
			want:    []map[string]any{{"type": "compaction", "id": "cmp_1", "encrypted_content": encrypted}},
		},
		{
			name:    "stateless without encrypted content omits the key",
			options: raw(`{"itemId":"cmp_1"}`),
			want:    []map[string]any{{"type": "compaction", "id": "cmp_1"}},
		},
		{
			name:    "stored empty item ID is present",
			options: raw(`{"itemId":""}`),
			store:   true,
			want:    []map[string]any{{"type": "item_reference", "id": ""}},
		},
		{
			name:         "conversation skips the part",
			options:      raw(`{"itemId":"cmp_1","encryptedContent":"encrypted"}`),
			store:        true,
			conversation: "conv_1",
		},
		{name: "null item ID is absent", options: raw(`{"itemId":null,"encryptedContent":"encrypted"}`)},
		{name: "missing item ID is absent", options: raw(`{"encryptedContent":"encrypted"}`)},
		{name: "no options is absent"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compaction := provider.CustomPart("openai.compaction")
			compaction.ProviderOptions = tc.options
			store := tc.store
			options := OpenAIResponsesOptions{Store: &store}
			if tc.conversation != "" {
				options.Conversation = tc.conversation
			}
			body, warnings := buildBody(t, "gpt-6", provider.CallOptions{
				Prompt:          []provider.Message{provider.NewAssistantMessage(compaction), provider.UserText("continue")},
				ProviderOptions: withOpenAIOptions(options),
			})
			assert.Empty(t, warnings)

			input := body["input"].([]any)
			require.Len(t, input, len(tc.want)+1)
			for i, want := range tc.want {
				assert.Equal(t, want, input[i])
			}
			assert.Equal(t, "user", input[len(tc.want)].(map[string]any)["role"])
		})
	}
}

func TestBuildParams_NonReasoningControlsByModel(t *testing.T) {
	const logprobsInclude = "message.output_text.logprobs"
	temperature, topP, logprobs := 0.5, 0.7, int64(5)
	sampling := map[string]any{"temperature": temperature, "top_p": topP}
	logprobsFields := map[string]any{"top_logprobs": float64(logprobs), "include": []any{logprobsInclude}}
	samplingWarnings := []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: "temperature", Details: "temperature is not supported for reasoning models"},
		{Type: provider.WarnUnsupported, Feature: "topP", Details: "topP is not supported for reasoning models"},
	}
	strippedWarnings := append(slices.Clone(samplingWarnings), provider.Warning{Type: provider.WarnUnsupported, Feature: "logprobs", Details: "logprobs is not supported for reasoning models"})
	unsupportedEffortWarnings := func(model, supported string) []provider.Warning {
		effort := provider.Warning{Type: provider.WarnUnsupported, Feature: "reasoningEffort", Details: model + " only supports the following reasoning efforts: " + supported}
		return append([]provider.Warning{effort}, strippedWarnings...)
	}
	const gpt6Efforts = "low, medium, high, xhigh, max"
	reasoning := func(effort string) map[string]any {
		block := map[string]any{"effort": effort}
		if effort != "none" {
			block["summary"] = "detailed"
		}
		return map[string]any{"reasoning": block}
	}

	for _, tc := range []struct {
		name, model   string
		azure         bool
		effort        string
		fallback      provider.ReasoningEffort
		update        OpenAIReasoningEffortUpdate
		reasoningMode string
		wantUpdate    bool
		wantFields    []map[string]any
		wantWarnings  []provider.Warning
	}{
		{name: "sol none retains controls", model: "gpt-6-sol", effort: "none", wantFields: []map[string]any{sampling, logprobsFields, reasoning("none")}},
		{name: "luna none retains controls", model: "gpt-6-luna", effort: "none", wantFields: []map[string]any{sampling, logprobsFields, reasoning("none")}},
		{name: "azure sol none retains controls", model: "gpt-6-sol", azure: true, effort: "none", wantFields: []map[string]any{sampling, logprobsFields, reasoning("none")}},
		{name: "sol positive effort strips controls", model: "gpt-6-sol", effort: "low", wantFields: []map[string]any{reasoning("low")}, wantWarnings: strippedWarnings},
		{name: "luna default effort strips controls", model: "gpt-6-luna", wantWarnings: strippedWarnings},
		{name: "luna core none fallback retains controls", model: "gpt-6-luna", fallback: provider.ReasoningNone, wantFields: []map[string]any{sampling, logprobsFields, reasoning("none")}},
		{name: "sol minimal is unsupported", model: "gpt-6-sol", effort: "minimal", wantWarnings: unsupportedEffortWarnings("gpt-6-sol", "none, "+gpt6Efforts)},
		{name: "sol blocked update keeps none effort", model: "gpt-6-sol", effort: "none", update: OpenAIReasoningEffortUpdateHigh, reasoningMode: "pro", wantFields: []map[string]any{sampling, logprobsFields, {"reasoning": map[string]any{"effort": "none", "mode": "pro"}}}, wantWarnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: "reasoningEffortUpdate", Details: "reasoningEffortUpdate requires standard reasoning mode without automatic compaction or automatic truncation"}}},
		{name: "sol positive update strips controls", model: "gpt-6-sol", effort: "none", update: OpenAIReasoningEffortUpdateHigh, wantUpdate: true, wantFields: []map[string]any{reasoning("none")}, wantWarnings: strippedWarnings},
		{name: "astra none is unsupported", model: "gpt-6-astra", effort: "none", wantWarnings: unsupportedEffortWarnings("gpt-6-astra", gpt6Efforts)},
		{name: "versioned sol none is unsupported", model: "gpt-6.1-sol", effort: "none", wantWarnings: unsupportedEffortWarnings("gpt-6.1-sol", gpt6Efforts)},
		{name: "fast luna none is unsupported", model: "gpt-6-luna-fast", effort: "none", wantWarnings: unsupportedEffortWarnings("gpt-6-luna-fast", gpt6Efforts)},
		{name: "gpt-5.1 none retains controls", model: "gpt-5.1", effort: "none", wantFields: []map[string]any{sampling, logprobsFields, reasoning("none")}},
		{name: "gpt-5.1 positive effort strips sampling", model: "gpt-5.1", effort: "low", wantFields: []map[string]any{logprobsFields, reasoning("low")}, wantWarnings: samplingWarnings},
		{name: "gpt-5.6-luna none retains controls", model: "gpt-5.6-luna", effort: "none", wantFields: []map[string]any{sampling, logprobsFields, reasoning("none")}},
		{name: "gpt-99 none is unsupported", model: "gpt-99", effort: "none", wantWarnings: unsupportedEffortWarnings("gpt-99", gpt6Efforts)},
		{name: "gpt-5 none strips sampling", model: "gpt-5", effort: "none", wantFields: []map[string]any{logprobsFields, reasoning("none")}, wantWarnings: samplingWarnings},
		{name: "non-reasoning model retains controls", model: "gpt-4o", wantFields: []map[string]any{sampling, logprobsFields}},
		{name: "mantle sol keeps endpoint restrictions", model: "openai.gpt-6-sol", effort: "none", wantFields: []map[string]any{logprobsFields, reasoning("none")}, wantWarnings: samplingWarnings},
		{name: "mantle astra keeps endpoint restrictions", model: "openai.gpt-6-astra", effort: "none", wantFields: []map[string]any{logprobsFields, reasoning("none")}, wantWarnings: samplingWarnings},
		{name: "mantle gpt-5.6-luna keeps endpoint restrictions", model: "openai.gpt-5.6-luna", effort: "none", wantFields: []map[string]any{logprobsFields, reasoning("none")}, wantWarnings: samplingWarnings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := OpenAIResponsesOptions{ReasoningEffort: tc.effort, ReasoningEffortUpdate: tc.update, ReasoningMode: tc.reasoningMode, Logprobs: &LogprobsOption{Int: &logprobs}, Include: []string{logprobsInclude}}
			providerOptions := withOpenAIOptions(options)
			if tc.azure {
				providerOptions = withAzureOptions(t, options)
			}
			body, warnings := buildBody(t, tc.model, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Temperature: &temperature, TopP: &topP, Reasoning: tc.fallback,
				ProviderOptions: providerOptions,
			})

			input := body["input"].([]any)
			if tc.wantUpdate {
				assert.Equal(t, map[string]any{"type": "configuration_update", "reasoning": map[string]any{"effort": string(tc.update)}}, input[0])
			} else {
				assert.Contains(t, input[0].(map[string]any), "content")
			}
			delete(body, "input")
			want := map[string]any{"model": tc.model}
			for _, fields := range tc.wantFields {
				maps.Copy(want, fields)
			}
			assert.Equal(t, want, body)
			assert.Equal(t, tc.wantWarnings, warnings)
		})
	}
}
