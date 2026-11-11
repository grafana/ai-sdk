package calldefaults

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/provider"
)

var _ extension.RequestTransformer = transformer{}

func ptr[T any](value T) *T { return &value }

func newTransformer(t *testing.T, definition extension.Definition, config string) extension.RequestTransformer {
	t.Helper()
	instance, err := definition.New(json.RawMessage(config), extension.Dependencies{})
	require.NoError(t, err)
	require.IsType(t, transformer{}, instance, "only a request transform")
	return instance.(transformer)
}

func transform(t *testing.T, transformer extension.RequestTransformer, params provider.CallOptions) provider.CallOptions {
	t.Helper()
	result, err := transformer.TransformRequest(context.Background(), &extension.Call{}, params)
	require.NoError(t, err)
	return result
}

func TestDefinitions(t *testing.T) {
	definitions := Definitions()
	require.Len(t, definitions, 2)
	for i, tenantType := range []tenanttype.Type{tenanttype.DefaultInstructions(), tenanttype.DefaultSettings()} {
		definition := definitions[i]
		assert.Equal(t, tenantType.Name, definition.Name)
		assert.Equal(t, extension.ScopeTenant, definition.Scope)
		assert.Nil(t, definition.Keys, "operator-owned and customer-owned calls")
		assert.Nil(t, definition.Sources, "every source")
		assert.Equal(t, extension.FailSkip, definition.FailurePolicy)
		assert.False(t, definition.EnabledByDefault)
		assert.Equal(t, tenantType.ConfigSchema.JSON(), definition.ConfigSchema.JSON())
		assert.Empty(t, definition.Secrets)
		newTransformer(t, definition, string(tenantType.Example))
	}
}

func TestInstructions(t *testing.T) {
	user := provider.NewUserMessage(provider.TextPart("hello"))
	assistant := provider.Message{Role: provider.RoleAssistant, Content: []provider.ContentPart{provider.TextPart("hi")}}
	client := provider.NewSystemMessage("client instructions")
	defaults := []provider.Message{provider.NewSystemMessage("first"), provider.NewSystemMessage("second")}
	instructions := newTransformer(t, instructionsDefinition(), `{"messages":[{"content":"first"},{"content":"second"}]}`)

	for _, tc := range []struct {
		name   string
		prompt []provider.Message
		want   []provider.Message
	}{
		{name: "no system message", prompt: []provider.Message{user, assistant}, want: append(append([]provider.Message(nil), defaults...), user, assistant)},
		{name: "system message first", prompt: []provider.Message{client, user}, want: []provider.Message{client, user}},
		{name: "system message later", prompt: []provider.Message{user, client}, want: []provider.Message{user, client}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt := append([]provider.Message(nil), tc.prompt...)
			result := transform(t, instructions, provider.CallOptions{Prompt: prompt, Temperature: ptr(0.5)})
			assert.Equal(t, tc.want, result.Prompt)
			assert.Equal(t, ptr(0.5), result.Temperature, "other parameters are kept")
			assert.Equal(t, tc.prompt, prompt, "the call's prompt is not mutated")
		})
	}
}

func TestSettings(t *testing.T) {
	all := `{"temperature":0.7,"topP":0.9,"topK":40,"maxOutputTokens":1024,"stopSequences":["a","b","c"],"seed":9007199254740993,"presencePenalty":0.1,"frequencyPenalty":0.2}`
	for _, tc := range []struct {
		name   string
		config string
		params provider.CallOptions
		check  func(t *testing.T, result provider.CallOptions)
	}{
		{
			name:   "fills unset fields",
			config: all,
			check: func(t *testing.T, result provider.CallOptions) {
				assert.Equal(t, ptr(0.7), result.Temperature)
				assert.Equal(t, ptr(0.9), result.TopP)
				assert.Equal(t, ptr(40), result.TopK)
				assert.Equal(t, ptr(1024), result.MaxOutputTokens)
				assert.Equal(t, []string{"a", "b", "c"}, result.StopSequences)
				assert.Equal(t, ptr(9007199254740993), result.Seed, "integers are exact")
				assert.Equal(t, ptr(0.1), result.PresencePenalty)
				assert.Equal(t, ptr(0.2), result.FrequencyPenalty)
			},
		},
		{
			name:   "call values take precedence",
			config: all,
			params: provider.CallOptions{Temperature: ptr(0.0), TopP: ptr(0.5), TopK: ptr(1), MaxOutputTokens: ptr(16), StopSequences: []string{}, Seed: ptr(0), PresencePenalty: ptr(0.0), FrequencyPenalty: ptr(0.0)},
			check: func(t *testing.T, result provider.CallOptions) {
				assert.Equal(t, ptr(0.0), result.Temperature, "a call's 0 is kept")
				assert.Equal(t, ptr(0.5), result.TopP)
				assert.Equal(t, ptr(1), result.TopK)
				assert.Equal(t, ptr(16), result.MaxOutputTokens)
				assert.Equal(t, []string{}, result.StopSequences, "a call's empty stop sequences are kept")
				assert.Equal(t, ptr(0), result.Seed)
				assert.Equal(t, ptr(0.0), result.PresencePenalty)
				assert.Equal(t, ptr(0.0), result.FrequencyPenalty)
			},
		},
		{
			name:   "integers written with a fraction or an exponent",
			config: `{"maxOutputTokens":1.0,"topK":4e1,"seed":-9.223372036854775808e18}`,
			check: func(t *testing.T, result provider.CallOptions) {
				assert.Equal(t, ptr(1), result.MaxOutputTokens)
				assert.Equal(t, ptr(40), result.TopK)
				assert.Equal(t, ptr(-9223372036854775808), result.Seed)
			},
		},
		{
			name:   "only configured fields",
			config: `{"temperature":0.7}`,
			params: provider.CallOptions{MaxOutputTokens: ptr(16)},
			check: func(t *testing.T, result provider.CallOptions) {
				assert.Equal(t, provider.CallOptions{Temperature: ptr(0.7), MaxOutputTokens: ptr(16)}, result)
			},
		},
		{
			name:   "other parameters unchanged",
			config: all,
			params: provider.CallOptions{
				Prompt:          []provider.Message{provider.NewUserMessage(provider.TextPart("hello"))},
				Headers:         map[string]string{"x-call": "1"},
				ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"a":1}`)}},
				Tools:           []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup"}},
				ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceRequired},
				ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON},
				Reasoning:       provider.ReasoningHigh,
			},
			check: func(t *testing.T, result provider.CallOptions) {
				assert.Equal(t, []provider.Message{provider.NewUserMessage(provider.TextPart("hello"))}, result.Prompt)
				assert.Equal(t, map[string]string{"x-call": "1"}, result.Headers)
				assert.Equal(t, provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"a":1}`)}}, result.ProviderOptions)
				assert.Equal(t, []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup"}}, result.Tools)
				assert.Equal(t, &provider.ToolChoice{Type: provider.ToolChoiceRequired}, result.ToolChoice)
				assert.Equal(t, &provider.ResponseFormat{Type: provider.ResponseFormatJSON}, result.ResponseFormat)
				assert.Equal(t, provider.ReasoningHigh, result.Reasoning)
			},
		},
		{
			name:   "no call controls added",
			config: all,
			check: func(t *testing.T, result provider.CallOptions) {
				assert.Nil(t, result.Headers)
				assert.Nil(t, result.ProviderOptions)
				assert.Nil(t, result.Tools)
				assert.Nil(t, result.ToolChoice)
				assert.Nil(t, result.ResponseFormat)
				assert.Equal(t, provider.ReasoningProviderDefault, result.Reasoning)
				assert.Nil(t, result.Prompt)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, transform(t, newTransformer(t, settingsDefinition(), tc.config), tc.params))
		})
	}
}

func TestSettings_AppendingToStopSequencesLeavesOtherCallsUnchanged(t *testing.T) {
	settings := newTransformer(t, settingsDefinition(), `{"stopSequences":["a","b","c"]}`)
	first := transform(t, settings, provider.CallOptions{})
	first.StopSequences = append(first.StopSequences, "added")
	first.StopSequences[0] = "changed"
	assert.Equal(t, []string{"a", "b", "c"}, transform(t, settings, provider.CallOptions{}).StopSequences)
}

func TestFactories_RefuseUndecodableConfiguration(t *testing.T) {
	for _, tc := range []struct {
		definition extension.Definition
		config     string
	}{
		{definition: instructionsDefinition(), config: `{"messages":"e2e-sentinel"}`},
		{definition: settingsDefinition(), config: `{"topK":1e30}`},
		{definition: settingsDefinition(), config: `{"temperature":"e2e-sentinel"}`},
		{definition: settingsDefinition(), config: `{"topK":1.5}`},
		{definition: settingsDefinition(), config: `{"temperature":1e400}`},
	} {
		_, err := tc.definition.New(json.RawMessage(tc.config), extension.Dependencies{})
		require.Error(t, err, tc.config)
		assert.NotContains(t, err.Error(), "e2e-sentinel")
		assert.NotContains(t, err.Error(), "1e30")
		assert.NotContains(t, err.Error(), "1e400")
	}
}
