package chatcompletions

import (
	"context"
	"encoding/json"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// Backend identifies the native translation used by a configured candidate.
type Backend string

const (
	BackendResponses  Backend = "openai"
	BackendAnthropic  Backend = "anthropic"
	BackendCompatible Backend = "openai-compatible"
)

type defaultsKey struct{}
type requestDefaults struct {
	strict   bool
	parallel *bool
}
type defaultsModel struct {
	provider.LanguageModel
	backend   Backend
	namespace string
}

// WithChatDefaults translates Chat defaults at the candidate boundary. Other
// protocols invoke the underlying model with unchanged options.
func WithChatDefaults(model provider.LanguageModel, backend Backend, namespace string) provider.LanguageModel {
	return &defaultsModel{LanguageModel: model, backend: backend, namespace: namespace}
}

func (m *defaultsModel) options(ctx context.Context, options provider.CallOptions) (provider.CallOptions, error) {
	defaults, ok := ctx.Value(defaultsKey{}).(requestDefaults)
	if !ok {
		return options, nil
	}
	namespace := m.namespace
	values := map[string]any{}
	switch m.backend {
	case BackendResponses:
		namespace = "openai"
		values["store"] = false
		values["strictJsonSchema"] = defaults.strict
		if defaults.parallel != nil {
			values["parallelToolCalls"] = *defaults.parallel
		}
	case BackendAnthropic:
		namespace = "anthropic"
		if defaults.parallel != nil {
			values["disableParallelToolUse"] = !*defaults.parallel
		}
	case BackendCompatible:
		namespace, _, _ = strings.Cut(namespace, ".")
		namespace = strings.TrimSpace(namespace)
		if namespace == "" {
			namespace = "openai-compatible"
		}
		values["store"] = false
		values["strictJsonSchema"] = defaults.strict
		if defaults.parallel != nil {
			values["parallel_tool_calls"] = *defaults.parallel
		}
	}
	if len(values) == 0 {
		return options, nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return options, err
	}
	options.ProviderOptions = maps.Clone(options.ProviderOptions)
	if options.ProviderOptions == nil {
		options.ProviderOptions = provider.ProviderOptions{}
	}
	options.ProviderOptions[namespace] = provider.RawProviderOption{Key: namespace, Raw: raw}
	return options, nil
}
func (m *defaultsModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	options, err := m.options(ctx, options)
	if err != nil {
		return nil, err
	}
	return m.LanguageModel.DoGenerate(ctx, options)
}
func (m *defaultsModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	options, err := m.options(ctx, options)
	if err != nil {
		return nil, err
	}
	return m.LanguageModel.DoStream(ctx, options)
}
