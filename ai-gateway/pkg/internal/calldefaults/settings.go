package calldefaults

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/middleware"
)

var errSettingsConfig = errors.New("calldefaults: the default-settings configuration does not decode")

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
		New:           newSettings,
	})
}

func newSettings(config json.RawMessage, _ extension.Dependencies) (any, error) {
	var decoded settingsConfig
	if err := json.Unmarshal(config, &decoded); err != nil {
		return nil, errSettingsConfig
	}
	return transformer{middleware: middleware.DefaultSettings(middleware.DefaultSettingsOptions{
		MaxOutputTokens:  decoded.MaxOutputTokens,
		Temperature:      decoded.Temperature,
		TopP:             decoded.TopP,
		TopK:             decoded.TopK,
		PresencePenalty:  decoded.PresencePenalty,
		FrequencyPenalty: decoded.FrequencyPenalty,
		StopSequences:    slices.Clip(decoded.StopSequences),
		Seed:             decoded.Seed,
	})}, nil
}
