package calldefaults

import (
	"encoding/json"
	"errors"
	"math/big"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/middleware"
)

var errSettingsConfig = errors.New("calldefaults: the default-settings configuration does not decode")

type settingsConfig struct {
	Temperature      *float64     `json:"temperature"`
	TopP             *float64     `json:"topP"`
	TopK             *json.Number `json:"topK"`
	MaxOutputTokens  *json.Number `json:"maxOutputTokens"`
	StopSequences    []string     `json:"stopSequences"`
	Seed             *json.Number `json:"seed"`
	PresencePenalty  *float64     `json:"presencePenalty"`
	FrequencyPenalty *float64     `json:"frequencyPenalty"`
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
	options := middleware.DefaultSettingsOptions{
		Temperature:      decoded.Temperature,
		TopP:             decoded.TopP,
		PresencePenalty:  decoded.PresencePenalty,
		FrequencyPenalty: decoded.FrequencyPenalty,
		StopSequences:    slices.Clip(decoded.StopSequences),
	}
	var err error
	if options.TopK, err = intOf(decoded.TopK); err != nil {
		return nil, err
	}
	if options.MaxOutputTokens, err = intOf(decoded.MaxOutputTokens); err != nil {
		return nil, err
	}
	if options.Seed, err = intOf(decoded.Seed); err != nil {
		return nil, err
	}
	return transformer{middleware: middleware.DefaultSettings(options)}, nil
}

// intOf converts an integer the schema accepted, which may be written with a
// fraction or an exponent, such as 1.0 or 4e1.
func intOf(number *json.Number) (*int, error) {
	if number == nil {
		return nil, nil
	}
	exact, ok := new(big.Rat).SetString(number.String())
	if !ok || !exact.IsInt() || !exact.Num().IsInt64() {
		return nil, errSettingsConfig
	}
	value := int(exact.Num().Int64())
	return &value, nil
}
