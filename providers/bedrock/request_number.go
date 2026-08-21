package bedrock

import (
	"errors"

	"github.com/grafana/ai-sdk/internal/providerrequest"
	"github.com/grafana/ai-sdk/provider"
)

func addBedrockRequestNumber(number provider.LanguageModelNumber, delta int64) (provider.LanguageModelNumber, error) {
	result, err := providerrequest.AddInt64(number, delta)
	switch {
	case errors.Is(err, providerrequest.ErrLanguageModelNumberOverflow):
		return provider.LanguageModelNumber{}, errors.New("bedrock: max output tokens overflow")
	case errors.Is(err, providerrequest.ErrInvalidLanguageModelNumber):
		return provider.LanguageModelNumber{}, errors.New("bedrock: invalid language model number")
	default:
		return result, err
	}
}

func validBedrockRequestNumber(number provider.LanguageModelNumber) bool {
	if _, ok := number.Int64(); ok {
		return true
	}
	_, ok := number.Float64()
	return ok
}
