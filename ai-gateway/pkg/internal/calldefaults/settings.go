package calldefaults

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

var (
	errSettingsConfig = errors.New("calldefaults: the default-settings configuration does not decode")
	errSettingsCall   = errors.New("calldefaults: the call carries no default-settings configuration")
)

type settingsConfig struct {
	Temperature      *float64 `json:"temperature"`
	TopP             *float64 `json:"topP"`
	TopK             *int     `json:"topK"`
	MaxOutputTokens  *int     `json:"maxOutputTokens"`
	StopSequences    []string `json:"stopSequences"`
	Seed             *int     `json:"seed"`
	PresencePenalty  *float64 `json:"presencePenalty"`
	FrequencyPenalty *float64 `json:"frequencyPenalty"`
}

func settingsDefinition() extension.Definition {
	return tenanttype.DefaultSettings().Bind(tenanttype.Implementation{
		FailurePolicy: extension.FailSkip,
		New:           func(json.RawMessage, extension.Dependencies) (any, error) { return settingsTransformer{}, nil },
		Decode:        decodeSettings,
	})
}

// decodeSettings decodes a namespace's default settings into the middleware's
// options. Stop sequences are clipped, so a later transform's append never
// writes into the slice every call of the namespace shares.
func decodeSettings(config json.RawMessage) (any, error) {
	var decoded settingsConfig
	if err := json.Unmarshal(config, &decoded); err != nil {
		return nil, errSettingsConfig
	}
	return middleware.DefaultSettingsOptions{
		Temperature:      decoded.Temperature,
		TopP:             decoded.TopP,
		TopK:             decoded.TopK,
		MaxOutputTokens:  decoded.MaxOutputTokens,
		StopSequences:    slices.Clip(decoded.StopSequences),
		Seed:             decoded.Seed,
		PresencePenalty:  decoded.PresencePenalty,
		FrequencyPenalty: decoded.FrequencyPenalty,
	}, nil
}

// settingsTransformer applies each call's default settings.
type settingsTransformer struct{}

func (settingsTransformer) TransformRequest(ctx context.Context, call *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
	options, ok := call.Config.(middleware.DefaultSettingsOptions)
	if !ok {
		return params, errSettingsCall
	}
	return middleware.DefaultSettings(options).TransformParams(ctx, middleware.TransformParamsInput{Params: params})
}
