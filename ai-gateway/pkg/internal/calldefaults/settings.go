package calldefaults

import (
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"strconv"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/middleware"
)

var errSettingsConfig = errors.New("calldefaults: the default-settings configuration does not decode")

type settingsConfig struct {
	Temperature      *json.Number `json:"temperature"`
	TopP             *json.Number `json:"topP"`
	TopK             *json.Number `json:"topK"`
	MaxOutputTokens  *json.Number `json:"maxOutputTokens"`
	StopSequences    []string     `json:"stopSequences"`
	Seed             *json.Number `json:"seed"`
	PresencePenalty  *json.Number `json:"presencePenalty"`
	FrequencyPenalty *json.Number `json:"frequencyPenalty"`
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
	options := middleware.DefaultSettingsOptions{StopSequences: slices.Clip(decoded.StopSequences)}
	for target, value := range map[**float64]*json.Number{
		&options.Temperature:      decoded.Temperature,
		&options.TopP:             decoded.TopP,
		&options.PresencePenalty:  decoded.PresencePenalty,
		&options.FrequencyPenalty: decoded.FrequencyPenalty,
	} {
		converted, err := floatOf(value)
		if err != nil {
			return nil, err
		}
		*target = converted
	}
	for target, value := range map[**int]*json.Number{
		&options.TopK:            decoded.TopK,
		&options.MaxOutputTokens: decoded.MaxOutputTokens,
		&options.Seed:            decoded.Seed,
	} {
		converted, err := intOf(value)
		if err != nil {
			return nil, err
		}
		*target = converted
	}
	return transformer{middleware: middleware.DefaultSettings(options)}, nil
}

func floatOf(number *json.Number) (*float64, error) {
	if number == nil {
		return nil, nil
	}
	value, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return nil, errSettingsConfig
	}
	return &value, nil
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
