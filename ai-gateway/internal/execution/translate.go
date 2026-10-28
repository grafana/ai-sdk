package execution

import (
	"context"
	"errors"
	"maps"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

// TargetType is the native protocol of one physical candidate.
type TargetType string

const (
	// TargetAnthropic is an Anthropic Messages candidate.
	TargetAnthropic TargetType = "anthropic"
	// TargetOpenAI is an OpenAI Responses candidate.
	TargetOpenAI TargetType = "openai"
	// TargetOpenAICompatible is an OpenAI-compatible Chat Completions candidate.
	TargetOpenAICompatible TargetType = "openai-compatible"
)

// Target identifies the physical candidate an attempt invokes.
type Target struct {
	Type TargetType
	// ProviderName is the configured provider name of an OpenAI-compatible
	// candidate, which names its native option namespace. It is empty for
	// other types.
	ProviderName string
	ModelID      string
}

// Translator maps a call's options to one target's native options. It runs
// once per physical attempt, on the attempt's own copy of the top-level
// provider-option and header maps; it must not mutate values nested inside
// them. Translated options are re-checked by host policy and then by the
// target's native option validation.
type Translator func(Target, provider.CallOptions) (provider.CallOptions, error)

// ErrTranslation reports a translator that panicked or produced options that
// host policy refuses. It never carries option or header values, is classified
// as internal, and Gateway fallback chains do not retry it.
var ErrTranslation = errors.New("execution: option translation failed")

// FallbackDecider is the decider for Gateway fallback chains. It stops on
// [ErrTranslation] and otherwise applies [fallback.DefaultDecider].
func FallbackDecider(err error) bool {
	return !errors.Is(err, ErrTranslation) && fallback.DefaultDecider(err)
}

// CallOption configures one [Runtime.Call].
type CallOption func(*callConfig)

type callConfig struct {
	translator Translator
}

// WithTranslator makes translator available to every physical attempt of the
// call. A nil translator leaves options unchanged.
func WithTranslator(translator Translator) CallOption {
	return func(config *callConfig) { config.translator = translator }
}

type translatorKey struct{}

func withTranslator(ctx context.Context, translator Translator) context.Context {
	if translator == nil {
		return ctx
	}
	return context.WithValue(ctx, translatorKey{}, translator)
}

// Translate wraps one physical candidate so each attempt receives its call's
// translation for target. Without a translator in the call, options pass
// through unchanged. Hosts wrap every candidate once, at construction,
// directly outside its native option validation and inside fallback.
func Translate(model provider.LanguageModel, target Target) provider.LanguageModel {
	return translatedModel{LanguageModel: model, target: target}
}

type translatedModel struct {
	provider.LanguageModel
	target Target
}

func (m translatedModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	options, err := translate(ctx, m.target, options)
	if err != nil {
		return nil, err
	}
	return m.LanguageModel.DoGenerate(ctx, options)
}

func (m translatedModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	options, err := translate(ctx, m.target, options)
	if err != nil {
		return nil, err
	}
	return m.LanguageModel.DoStream(ctx, options)
}

func translate(ctx context.Context, target Target, options provider.CallOptions) (provider.CallOptions, error) {
	translator, ok := ctx.Value(translatorKey{}).(Translator)
	if !ok {
		return options, nil
	}
	options.ProviderOptions = maps.Clone(options.ProviderOptions)
	options.Headers = maps.Clone(options.Headers)
	translated, err := safeTranslate(translator, target, options)
	if err != nil {
		return provider.CallOptions{}, err
	}
	if !allowedTranslation(translated) {
		return provider.CallOptions{}, ErrTranslation
	}
	return translated, nil
}

func safeTranslate(translator Translator, target Target, options provider.CallOptions) (translated provider.CallOptions, err error) {
	defer func() {
		if recover() != nil {
			translated, err = provider.CallOptions{}, ErrTranslation
		}
	}()
	return translator(target, options)
}
