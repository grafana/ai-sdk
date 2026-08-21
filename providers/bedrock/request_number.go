package bedrock

import (
	"errors"
	"math"

	"github.com/grafana/ai-sdk/provider"
)

func addBedrockRequestNumber(number provider.LanguageModelNumber, delta int64) (provider.LanguageModelNumber, error) {
	if integer, ok := number.Int64(); ok {
		if delta > 0 && integer > math.MaxInt64-delta || delta < 0 && integer < math.MinInt64-delta {
			return provider.LanguageModelNumber{}, errors.New("bedrock: max output tokens overflow")
		}
		return provider.LanguageModelNumberFromInt64(integer + delta), nil
	}
	floating, ok := number.Float64()
	if !ok {
		return provider.LanguageModelNumber{}, errors.New("bedrock: invalid language model number")
	}
	return provider.LanguageModelNumberFromFloat64(floating + float64(delta))
}

func validBedrockRequestNumber(number provider.LanguageModelNumber) bool {
	if _, ok := number.Int64(); ok {
		return true
	}
	_, ok := number.Float64()
	return ok
}
