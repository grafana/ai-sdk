package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type defaultSettingsTestOption struct {
	Thinking bool   `json:"thinking,omitempty"`
	Effort   string `json:"effort,omitempty"`
}

func (defaultSettingsTestOption) ProviderKey() string { return "anthropic" }

type unmarshalableOption struct {
	Value func() `json:"value"`
}

func (unmarshalableOption) ProviderKey() string { return "anthropic" }

func applyDefaultSettings(t *testing.T, settings DefaultSettingsOptions, params provider.CallOptions) provider.CallOptions {
	t.Helper()
	var received provider.CallOptions
	model := &mockModel{
		doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
			received = params
			return &provider.GenerateResult{}, nil
		},
	}
	_, err := WrapLanguageModel(model, DefaultSettings(settings)).DoGenerate(context.Background(), params)
	require.NoError(t, err)
	return received
}

func rawOptions(t *testing.T, data string) provider.ProviderOptions {
	t.Helper()
	var options provider.ProviderOptions
	require.NoError(t, json.Unmarshal([]byte(data), &options))
	return options
}

func optionsJSON(t *testing.T, options provider.ProviderOptions) string {
	t.Helper()
	data, err := json.Marshal(options)
	require.NoError(t, err)
	return string(data)
}

func TestDefaultSettings(t *testing.T) {
	t.Run("AppliesDefaultSettings", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{Temperature: ptr(0.7), MaxOutputTokens: ptr(1024)}, provider.CallOptions{})
		require.NotNil(t, received.Temperature)
		assert.Equal(t, 0.7, *received.Temperature)
		require.NotNil(t, received.MaxOutputTokens)
		assert.Equal(t, 1024, *received.MaxOutputTokens)
	})

	t.Run("CallerValueTakesPrecedence", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{Temperature: ptr(0.7)}, provider.CallOptions{Temperature: ptr(0.5)})
		require.NotNil(t, received.Temperature)
		assert.Equal(t, 0.5, *received.Temperature)
	})

	t.Run("MultipleDefaults_PartialOverride", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{
			Temperature:     ptr(0.7),
			MaxOutputTokens: ptr(1024),
			StopSequences:   []string{"STOP"},
		}, provider.CallOptions{Temperature: ptr(0.3)})
		assert.Equal(t, 0.3, *received.Temperature)
		assert.Equal(t, 1024, *received.MaxOutputTokens)
		assert.Equal(t, []string{"STOP"}, received.StopSequences)
	})

	t.Run("NoDefaultsOrCaller", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{}, provider.CallOptions{})
		assert.Nil(t, received.Headers)
		assert.Nil(t, received.ProviderOptions)
		assert.Nil(t, received.ResponseFormat)
		assert.Equal(t, provider.ReasoningProviderDefault, received.Reasoning)
	})
}

func TestDefaultSettings_Temperature(t *testing.T) {
	tests := []struct {
		name     string
		settings *float64
		caller   *float64
		want     *float64
	}{
		{name: "KeepsZeroWithoutDefault", caller: ptr(0.0), want: ptr(0.0)},
		{name: "KeepsZeroOverDefault", settings: ptr(0.7), caller: ptr(0.0), want: ptr(0.0)},
		{name: "DefaultWhenUnset", settings: ptr(0.7), want: ptr(0.7)},
		{name: "CallerByDefault", settings: ptr(0.7), caller: ptr(0.9), want: ptr(0.9)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			received := applyDefaultSettings(t, DefaultSettingsOptions{Temperature: tc.settings}, provider.CallOptions{Temperature: tc.caller})
			assert.Equal(t, tc.want, received.Temperature)
		})
	}
}

func TestDefaultSettings_OtherSettings(t *testing.T) {
	t.Run("DefaultMaxOutputTokens", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{MaxOutputTokens: ptr(100)}, provider.CallOptions{})
		assert.Equal(t, ptr(100), received.MaxOutputTokens)
	})

	t.Run("CallerMaxOutputTokens", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{MaxOutputTokens: ptr(100)}, provider.CallOptions{MaxOutputTokens: ptr(50)})
		assert.Equal(t, ptr(50), received.MaxOutputTokens)
	})

	t.Run("DefaultStopSequences", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{StopSequences: []string{"stop"}}, provider.CallOptions{})
		assert.Equal(t, []string{"stop"}, received.StopSequences)
	})

	t.Run("CallerStopSequences", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{StopSequences: []string{"stop"}}, provider.CallOptions{StopSequences: []string{"end"}})
		assert.Equal(t, []string{"end"}, received.StopSequences)
	})

	t.Run("DefaultTopP", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{TopP: ptr(0.9)}, provider.CallOptions{})
		assert.Equal(t, ptr(0.9), received.TopP)
	})

	t.Run("CallerTopP", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{TopP: ptr(0.9)}, provider.CallOptions{TopP: ptr(0.5)})
		assert.Equal(t, ptr(0.5), received.TopP)
	})

	t.Run("AllOptionalDefaults", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{
			TopK:             ptr(40),
			PresencePenalty:  ptr(0.1),
			FrequencyPenalty: ptr(0.2),
			Seed:             ptr(42),
		}, provider.CallOptions{})
		assert.Equal(t, ptr(40), received.TopK)
		assert.Equal(t, ptr(0.1), received.PresencePenalty)
		assert.Equal(t, ptr(0.2), received.FrequencyPenalty)
		assert.Equal(t, ptr(42), received.Seed)
	})

	t.Run("EmptyCallerSlicesKept", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{
			StopSequences: []string{"stop"},
			Tools:         []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup"}},
		}, provider.CallOptions{StopSequences: []string{}, Tools: []provider.Tool{}})
		assert.Equal(t, []string{}, received.StopSequences)
		assert.Equal(t, []provider.Tool{}, received.Tools)
	})

	t.Run("DefaultTools", func(t *testing.T) {
		tools := []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup"}}
		received := applyDefaultSettings(t, DefaultSettingsOptions{Tools: tools}, provider.CallOptions{})
		assert.Equal(t, tools, received.Tools)
	})
}

func TestDefaultSettings_ToolChoice(t *testing.T) {
	named := &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}

	t.Run("DefaultWhenUnset", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{ToolChoice: named}, provider.CallOptions{})
		assert.Equal(t, named, received.ToolChoice)
	})

	t.Run("AutomaticCallerOverNamedDefault", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{ToolChoice: named}, provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto}})
		assert.Equal(t, &provider.ToolChoice{Type: provider.ToolChoiceAuto}, received.ToolChoice)
	})
}

func TestDefaultSettings_Reasoning(t *testing.T) {
	t.Run("DefaultWhenZero", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{Reasoning: provider.ReasoningMedium}, provider.CallOptions{})
		assert.Equal(t, provider.ReasoningMedium, received.Reasoning)
	})

	t.Run("CallerTakesPrecedence", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{Reasoning: provider.ReasoningMedium}, provider.CallOptions{Reasoning: provider.ReasoningHigh})
		assert.Equal(t, provider.ReasoningHigh, received.Reasoning)
	})
}

func TestDefaultSettings_Headers(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		caller   map[string]string
		want     map[string]string
	}{
		{
			name:     "MergesHeaders",
			settings: map[string]string{"X-Custom-Header": "test", "X-Another-Header": "test2"},
			caller:   map[string]string{"X-Custom-Header": "test2"},
			want:     map[string]string{"X-Custom-Header": "test2", "X-Another-Header": "test2"},
		},
		{
			name:     "EmptyDefaultHeaders",
			settings: map[string]string{},
			caller:   map[string]string{"X-Param-Header": "param"},
			want:     map[string]string{"X-Param-Header": "param"},
		},
		{
			name:     "EmptyCallerHeaders",
			settings: map[string]string{"X-Default-Header": "default"},
			caller:   map[string]string{},
			want:     map[string]string{"X-Default-Header": "default"},
		},
		{name: "BothUnset"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			received := applyDefaultSettings(t, DefaultSettingsOptions{Headers: tc.settings}, provider.CallOptions{Headers: tc.caller})
			assert.Equal(t, tc.want, received.Headers)
		})
	}
}

func TestDefaultSettings_ProviderOptions(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		caller   string
		want     string
	}{
		{
			name:     "MergesWithDefaultSettings",
			settings: `{"anthropic":{"cacheControl":{"type":"ephemeral"}}}`,
			want:     `{"anthropic":{"cacheControl":{"type":"ephemeral"}}}`,
		},
		{
			name:     "MergesComplexProviderMetadata",
			settings: `{"anthropic":{"cacheControl":{"type":"ephemeral"},"feature":{"enabled":true}},"openai":{"logit_bias":{"50256":-100}}}`,
			caller:   `{"anthropic":{"feature":{"enabled":false},"otherSetting":"value"}}`,
			want:     `{"anthropic":{"cacheControl":{"type":"ephemeral"},"feature":{"enabled":false},"otherSetting":"value"},"openai":{"logit_bias":{"50256":-100}}}`,
		},
		{
			name:     "MergesNestedProviderMetadata",
			settings: `{"anthropic":{"tools":{"retrieval":{"enabled":true},"math":{"enabled":true}}}}`,
			caller:   `{"anthropic":{"tools":{"retrieval":{"enabled":false},"code":{"enabled":true}}}}`,
			want:     `{"anthropic":{"tools":{"retrieval":{"enabled":false},"math":{"enabled":true},"code":{"enabled":true}}}}`,
		},
		{
			name:     "EmptyDefaultProviderOptions",
			settings: `{}`,
			caller:   `{"openai":{"user":"param-user"}}`,
			want:     `{"openai":{"user":"param-user"}}`,
		},
		{
			name:     "EmptyCallerProviderOptions",
			settings: `{"anthropic":{"user":"default-user"}}`,
			caller:   `{}`,
			want:     `{"anthropic":{"user":"default-user"}}`,
		},
		{
			name:     "ArraysAndNullReplace",
			settings: `{"anthropic":{"list":[1,2],"object":{"a":1}}}`,
			caller:   `{"anthropic":{"list":[3],"object":null}}`,
			want:     `{"anthropic":{"list":[3],"object":null}}`,
		},
		{
			name:     "PrototypeKeysIgnored",
			settings: `{"anthropic":{"user":"alice"}}`,
			caller:   `{"anthropic":{"__proto__":{"polluted":true},"constructor":{"a":1},"prototype":{"b":2},"role":"admin"}}`,
			want:     `{"anthropic":{"user":"alice","role":"admin"}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var settings, caller provider.ProviderOptions
			if tc.settings != "" {
				settings = rawOptions(t, tc.settings)
			}
			if tc.caller != "" {
				caller = rawOptions(t, tc.caller)
			}

			received := applyDefaultSettings(t, DefaultSettingsOptions{ProviderOptions: settings}, provider.CallOptions{ProviderOptions: caller})

			assert.JSONEq(t, tc.want, optionsJSON(t, received.ProviderOptions))
		})
	}

	t.Run("BothUnset", func(t *testing.T) {
		received := applyDefaultSettings(t, DefaultSettingsOptions{}, provider.CallOptions{})
		assert.Nil(t, received.ProviderOptions)
	})

	t.Run("TypedDefaultMergedWithRawCaller", func(t *testing.T) {
		settings := provider.BuildProviderOptions(defaultSettingsTestOption{Thinking: true})
		caller := provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"effort":"high"}`)},
			"openai":    provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"logprobs":true}`)},
		}

		received := applyDefaultSettings(t, DefaultSettingsOptions{ProviderOptions: settings}, provider.CallOptions{ProviderOptions: caller})

		resolved, ok, err := provider.ResolveOption[defaultSettingsTestOption](received.ProviderOptions, "anthropic")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, defaultSettingsTestOption{Thinking: true, Effort: "high"}, resolved)
		assert.Equal(t, caller["openai"], received.ProviderOptions["openai"])
		assert.Equal(t, defaultSettingsTestOption{Thinking: true}, settings["anthropic"])
		assert.JSONEq(t, `{"effort":"high"}`, string(caller["anthropic"].(provider.RawProviderOption).Raw))
	})

	t.Run("OneSidedNamespacesKeepTypedValues", func(t *testing.T) {
		typed := defaultSettingsTestOption{Effort: "low"}
		settings := provider.ProviderOptions{"anthropic": typed}
		caller := rawOptions(t, `{"openai":{"user":"u"}}`)

		received := applyDefaultSettings(t, DefaultSettingsOptions{ProviderOptions: settings}, provider.CallOptions{ProviderOptions: caller})

		assert.Equal(t, typed, received.ProviderOptions["anthropic"])
		assert.Equal(t, caller["openai"], received.ProviderOptions["openai"])
	})

	t.Run("DefaultsNotSharedAcrossCalls", func(t *testing.T) {
		settings := rawOptions(t, `{"anthropic":{"user":"default-user"}}`)
		mw := DefaultSettings(DefaultSettingsOptions{ProviderOptions: settings})

		params, err := mw.TransformParams(context.Background(), TransformParamsInput{})
		require.NoError(t, err)
		params.ProviderOptions["openai"] = provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{}`)}

		assert.Len(t, settings, 1)
	})

	t.Run("MarshalErrorReturned", func(t *testing.T) {
		mw := DefaultSettings(DefaultSettingsOptions{ProviderOptions: provider.BuildProviderOptions(unmarshalableOption{})})
		_, err := mw.TransformParams(context.Background(), TransformParamsInput{
			Params: provider.CallOptions{ProviderOptions: rawOptions(t, `{"anthropic":{}}`)},
		})
		require.Error(t, err)
	})
}

func TestDefaultSettings_ResponseFormat(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	callerSchema := json.RawMessage(`{"type":"array"}`)
	jsonDefault := &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema, Name: ptr("answer"), Description: "An answer."}

	tests := []struct {
		name     string
		settings *provider.ResponseFormat
		caller   *provider.ResponseFormat
		want     *provider.ResponseFormat
	}{
		{name: "DefaultWhenUnset", settings: jsonDefault, want: jsonDefault},
		{name: "CallerWithoutDefault", caller: &provider.ResponseFormat{Type: provider.ResponseFormatText}, want: &provider.ResponseFormat{Type: provider.ResponseFormatText}},
		{
			name:     "JSONCallerInheritsDefaultMembers",
			settings: jsonDefault,
			caller:   &provider.ResponseFormat{Type: provider.ResponseFormatJSON},
			want:     jsonDefault,
		},
		{
			name:     "CallerMembersWin",
			settings: jsonDefault,
			caller:   &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: callerSchema, Name: ptr(""), Description: "A list."},
			want:     &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: callerSchema, Name: ptr(""), Description: "A list."},
		},
		{
			name:     "TextCallerOverJSONDefault",
			settings: jsonDefault,
			caller:   &provider.ResponseFormat{Type: provider.ResponseFormatText},
			want:     &provider.ResponseFormat{Type: provider.ResponseFormatText},
		},
		{
			name:     "EmptyCallerTypeUsesDefaultType",
			settings: jsonDefault,
			caller:   &provider.ResponseFormat{Schema: callerSchema},
			want:     &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: callerSchema, Name: ptr("answer"), Description: "An answer."},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var original *provider.ResponseFormat
			if tc.caller != nil {
				original = ptr(*tc.caller)
			}

			received := applyDefaultSettings(t, DefaultSettingsOptions{ResponseFormat: tc.settings}, provider.CallOptions{ResponseFormat: tc.caller})

			assert.Equal(t, tc.want, received.ResponseFormat)
			assert.Equal(t, original, tc.caller)
		})
	}
}
