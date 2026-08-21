package anthropic

import (
	"encoding/json"
	"errors"
	"math"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/grafana/ai-sdk/provider"
)

func addRequestNumber(number provider.LanguageModelNumber, delta int64) (provider.LanguageModelNumber, error) {
	if integer, ok := number.Int64(); ok {
		if delta > 0 && integer > math.MaxInt64-delta || delta < 0 && integer < math.MinInt64-delta {
			return provider.LanguageModelNumber{}, errors.New("anthropic: max output tokens overflow")
		}
		return provider.LanguageModelNumberFromInt64(integer + delta), nil
	}
	floating, ok := number.Float64()
	if !ok {
		return provider.LanguageModelNumber{}, errors.New("anthropic: invalid language model number")
	}
	return provider.LanguageModelNumberFromFloat64(floating + float64(delta))
}

func requestNumberGreaterThan(number provider.LanguageModelNumber, limit int64) (bool, error) {
	if integer, ok := number.Int64(); ok {
		return integer > limit, nil
	}
	floating, ok := number.Float64()
	if !ok {
		return false, errors.New("anthropic: invalid language model number")
	}
	return floating > float64(limit), nil
}

func requestNumberString(number provider.LanguageModelNumber) string {
	encoded, err := json.Marshal(number)
	if err != nil {
		return "invalid"
	}
	return string(encoded)
}

func setMessageExtraField(params *anthropicsdk.BetaMessageNewParams, key string, value any) {
	extra := make(map[string]any, len(params.ExtraFields())+1)
	for name, existing := range params.ExtraFields() {
		extra[name] = existing
	}
	extra[key] = value
	params.SetExtraFields(extra)
}

func clearMessageExtraField(params *anthropicsdk.BetaMessageNewParams, key string) {
	existing := params.ExtraFields()
	if len(existing) == 0 {
		return
	}
	extra := make(map[string]any, len(existing))
	for name, value := range existing {
		if name != key {
			extra[name] = value
		}
	}
	params.SetExtraFields(extra)
}

func applyMaxTokens(params *anthropicsdk.BetaMessageNewParams, number provider.LanguageModelNumber) error {
	if integer, ok := number.Int64(); ok {
		params.MaxTokens = integer
		clearMessageExtraField(params, "max_tokens")
		return nil
	}
	floating, ok := number.Float64()
	if !ok {
		return errors.New("anthropic: invalid maxOutputTokens")
	}
	params.MaxTokens = 0
	setMessageExtraField(params, "max_tokens", floating)
	return nil
}

func applyTopK(params *anthropicsdk.BetaMessageNewParams, number provider.LanguageModelNumber) error {
	if integer, ok := number.Int64(); ok {
		params.TopK = anthropicsdk.Int(integer)
		clearMessageExtraField(params, "top_k")
		return nil
	}
	floating, ok := number.Float64()
	if !ok {
		return errors.New("anthropic: invalid topK")
	}
	params.TopK = param.Opt[int64]{}
	setMessageExtraField(params, "top_k", floating)
	return nil
}

func clearTopK(params *anthropicsdk.BetaMessageNewParams) {
	params.TopK = param.Opt[int64]{}
	clearMessageExtraField(params, "top_k")
}
