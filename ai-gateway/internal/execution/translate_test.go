package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

// optionsModel records the options each attempt receives and fails
// retryably when fail is set.
type optionsModel struct {
	provider.LanguageModel
	modelID string
	fail    bool
	mu      sync.Mutex
	seen    []provider.CallOptions
}

func (m *optionsModel) SpecificationVersion() string { return "v4" }
func (m *optionsModel) Provider() string             { return "native" }
func (m *optionsModel) ModelID() string              { return m.modelID }

func (m *optionsModel) record(options provider.CallOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = append(m.seen, options)
	if m.fail {
		return provider.NewAPICallError(provider.APICallErrorOptions{Message: "unavailable", StatusCode: 503})
	}
	return nil
}

func (m *optionsModel) DoGenerate(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if err := m.record(options); err != nil {
		return nil, err
	}
	return &provider.GenerateResult{}, nil
}

func (m *optionsModel) DoStream(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if err := m.record(options); err != nil {
		return nil, err
	}
	parts := make(chan provider.StreamPart, 1)
	parts <- provider.StreamPart{Type: provider.PartFinish}
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}

func (m *optionsModel) attempts() []provider.CallOptions {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]provider.CallOptions(nil), m.seen...)
}

// markTarget writes the target type and model into its own namespace.
func markTarget(target Target, options provider.CallOptions) (provider.CallOptions, error) {
	if options.ProviderOptions == nil {
		options.ProviderOptions = provider.ProviderOptions{}
	}
	raw := fmt.Sprintf(`{"model":%q}`, target.ModelID)
	options.ProviderOptions[string(target.Type)] = provider.RawProviderOption{Key: string(target.Type), Raw: json.RawMessage(raw)}
	return options, nil
}

func namespaces(options provider.CallOptions) []string {
	names := make([]string, 0, len(options.ProviderOptions))
	for name := range options.ProviderOptions {
		names = append(names, name)
	}
	return names
}

func callWithTranslator(t *testing.T, model provider.LanguageModel, translator Translator, options provider.CallOptions, streaming bool) error {
	t.Helper()
	call, err := newRuntime(t, directSelector(model), testLimits()).Call(context.Background(), "request/model", options, WithTranslator(translator))
	require.NoError(t, err)
	defer call.Close()
	if streaming {
		stream, err := call.Stream()
		if err == nil {
			stream.Close()
		}
		return err
	}
	_, err = call.Generate()
	return err
}

func TestTranslate_WithoutTranslatorPassesOptionsThrough(t *testing.T) {
	options := provider.CallOptions{ProviderOptions: rawOptions(map[string]string{"openai": `{"store":false}`}), Headers: map[string]string{"X-Trace": "1"}}
	for name, callOptions := range map[string][]CallOption{"no option": nil, "nil translator": {WithTranslator(nil)}} {
		t.Run(name, func(t *testing.T) {
			inner := &optionsModel{modelID: "native"}
			runtime := newRuntime(t, directSelector(Translate(inner, Target{Type: TargetOpenAI, ModelID: "native"})), testLimits())
			call, err := runtime.Call(context.Background(), "request/model", options, callOptions...)
			require.NoError(t, err)
			defer call.Close()
			_, err = call.Generate()
			require.NoError(t, err)
			require.Len(t, inner.attempts(), 1)
			assert.Equal(t, options, inner.attempts()[0])
		})
	}
}

func TestTranslate_EachAttemptGetsItsOwnTranslation(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			anthropic := &optionsModel{modelID: "claude", fail: true}
			openai := &optionsModel{modelID: "gpt"}
			chain, err := fallback.New(
				Translate(anthropic, Target{Type: TargetAnthropic, ModelID: "claude"}),
				Translate(openai, Target{Type: TargetOpenAI, ModelID: "gpt"}),
			)
			require.NoError(t, err)
			chain.WithDecider(FallbackDecider)
			original := provider.CallOptions{ProviderOptions: rawOptions(map[string]string{"shared": `{}`})}
			require.NoError(t, callWithTranslator(t, chain, markTarget, original, streaming))

			require.Len(t, anthropic.attempts(), 1)
			require.Len(t, openai.attempts(), 1)
			assert.ElementsMatch(t, []string{"shared", "anthropic"}, namespaces(anthropic.attempts()[0]))
			assert.ElementsMatch(t, []string{"shared", "openai"}, namespaces(openai.attempts()[0]))
			assert.JSONEq(t, `{"model":"gpt"}`, string(openai.attempts()[0].ProviderOptions["openai"].(provider.RawProviderOption).Raw))
			assert.ElementsMatch(t, []string{"shared"}, namespaces(original))
		})
	}
}

func TestTranslate_AttemptsDoNotShareTopLevelCollections(t *testing.T) {
	first := &optionsModel{modelID: "first", fail: true}
	second := &optionsModel{modelID: "second"}
	chain, err := fallback.New(Translate(first, Target{Type: TargetAnthropic, ModelID: "first"}), Translate(second, Target{Type: TargetOpenAI, ModelID: "second"}))
	require.NoError(t, err)
	chain.WithDecider(FallbackDecider)
	original := provider.CallOptions{
		Prompt:        []provider.Message{{Role: provider.RoleUser}},
		Tools:         []provider.Tool{{Name: "original"}},
		StopSequences: []string{"original"},
	}
	translator := func(target Target, options provider.CallOptions) (provider.CallOptions, error) {
		options.Prompt[0] = provider.Message{Role: provider.RoleSystem}
		options.Tools[0] = provider.Tool{Name: target.ModelID}
		options.StopSequences[0] = target.ModelID
		return options, nil
	}
	require.NoError(t, callWithTranslator(t, chain, translator, original, false))
	require.Len(t, second.attempts(), 1)
	assert.Equal(t, "second", second.attempts()[0].Tools[0].Name)
	assert.Equal(t, []string{"second"}, second.attempts()[0].StopSequences)
	assert.Equal(t, "first", first.attempts()[0].Tools[0].Name)
	assert.Equal(t, provider.RoleUser, original.Prompt[0].Role)
	assert.Equal(t, "original", original.Tools[0].Name)
	assert.Equal(t, []string{"original"}, original.StopSequences)
}

func TestTranslate_FailuresBeforeProviderContact(t *testing.T) {
	const secret = "private-header-value"
	inject := func(mutate func(*provider.CallOptions)) Translator {
		return func(_ Target, options provider.CallOptions) (provider.CallOptions, error) {
			mutate(&options)
			return options, nil
		}
	}
	for _, tc := range []struct {
		name       string
		translator Translator
	}{
		{"panic", func(Target, provider.CallOptions) (provider.CallOptions, error) { panic(secret) }},
		{"control namespace", inject(func(options *provider.CallOptions) {
			options.ProviderOptions = rawOptions(map[string]string{ControlNamespace: `{"byok":{}}`})
		})},
		{"protected header", inject(func(options *provider.CallOptions) {
			options.Headers = map[string]string{"AUTHORIZATION": "Bearer " + secret}
		})},
		{"protected header in another case", inject(func(options *provider.CallOptions) {
			options.Headers = map[string]string{"x-Api-Key": secret}
		})},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				first := &optionsModel{modelID: "first"}
				second := &optionsModel{modelID: "second"}
				chain, err := fallback.New(Translate(first, Target{Type: TargetAnthropic, ModelID: "first"}), Translate(second, Target{Type: TargetOpenAI, ModelID: "second"}))
				require.NoError(t, err)
				chain.WithDecider(FallbackDecider)
				err = callWithTranslator(t, chain, tc.translator, provider.CallOptions{}, streaming)
				require.ErrorIs(t, err, ErrTranslation)
				assert.Equal(t, CategoryInternal, ClassifyProvider(err))
				assert.NotContains(t, err.Error(), secret)
				assert.NotContains(t, strings.ToLower(err.Error()), "authorization")
				assert.Empty(t, first.attempts())
				assert.Empty(t, second.attempts())
			})
		}
	}
}

func TestTranslate_TranslatorErrorIsAnOrdinaryAttemptFailure(t *testing.T) {
	refusal := errors.New("translator refusal")
	first := &optionsModel{modelID: "first"}
	second := &optionsModel{modelID: "second"}
	chain, err := fallback.New(Translate(first, Target{Type: TargetAnthropic, ModelID: "first"}), Translate(second, Target{Type: TargetOpenAI, ModelID: "second"}))
	require.NoError(t, err)
	chain.WithDecider(FallbackDecider)
	translator := func(target Target, options provider.CallOptions) (provider.CallOptions, error) {
		if target.ModelID == "first" {
			return provider.CallOptions{}, refusal
		}
		return markTarget(target, options)
	}
	require.NoError(t, callWithTranslator(t, chain, translator, provider.CallOptions{}, false))
	assert.Empty(t, first.attempts())
	require.Len(t, second.attempts(), 1)
	assert.ElementsMatch(t, []string{"openai"}, namespaces(second.attempts()[0]))

	alone := &optionsModel{modelID: "alone"}
	err = callWithTranslator(t, Translate(alone, Target{Type: TargetOpenAI, ModelID: "alone"}), func(Target, provider.CallOptions) (provider.CallOptions, error) {
		return provider.CallOptions{}, refusal
	}, provider.CallOptions{}, false)
	assert.Same(t, refusal, err)
	assert.Empty(t, alone.attempts())
}

func TestTranslate_ConcurrentCallsAreIsolated(t *testing.T) {
	inner := &optionsModel{modelID: "native"}
	runtime := newRuntime(t, directSelector(Translate(inner, Target{Type: TargetOpenAI, ModelID: "native"})), testLimits())
	const calls = 32
	var group sync.WaitGroup
	for index := range calls {
		group.Go(func() {
			var callOptions []CallOption
			if index%2 == 0 {
				marker := fmt.Sprintf("call-%d", index)
				callOptions = append(callOptions, WithTranslator(func(_ Target, options provider.CallOptions) (provider.CallOptions, error) {
					options.Headers["X-Marker"] = marker
					return options, nil
				}))
			}
			options := provider.CallOptions{Headers: map[string]string{"X-Call": fmt.Sprintf("call-%d", index)}}
			call, err := runtime.Call(context.Background(), "request/model", options, callOptions...)
			if !assert.NoError(t, err) {
				return
			}
			defer call.Close()
			_, err = call.Generate()
			assert.NoError(t, err)
		})
	}
	group.Wait()
	attempts := inner.attempts()
	require.Len(t, attempts, calls)
	for _, options := range attempts {
		marker, translated := options.Headers["X-Marker"]
		var index int
		_, err := fmt.Sscanf(options.Headers["X-Call"], "call-%d", &index)
		require.NoError(t, err)
		if index%2 == 0 {
			assert.True(t, translated)
			assert.Equal(t, options.Headers["X-Call"], marker)
		} else {
			assert.False(t, translated)
		}
	}
}

func TestFallbackDecider(t *testing.T) {
	assert.False(t, FallbackDecider(ErrTranslation))
	assert.False(t, FallbackDecider(fmt.Errorf("attempt: %w", ErrTranslation)))
	retryable := provider.NewAPICallError(provider.APICallErrorOptions{Message: "unavailable", StatusCode: 503})
	assert.True(t, FallbackDecider(retryable))
	assert.Equal(t, fallback.DefaultDecider(errors.New("unknown")), FallbackDecider(errors.New("unknown")))
	terminal := provider.NewAPICallError(provider.APICallErrorOptions{Message: "bad request", StatusCode: 400})
	assert.False(t, FallbackDecider(terminal))
}
