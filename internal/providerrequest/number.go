package providerrequest

import (
	"errors"
	"math"

	"github.com/grafana/ai-sdk/provider"
)

var (
	// ErrLanguageModelNumberOverflow indicates that exact integer addition overflowed int64.
	ErrLanguageModelNumberOverflow = errors.New("providerrequest: language model number overflow")
	// ErrInvalidLanguageModelNumber indicates that a request number has no valid variant.
	ErrInvalidLanguageModelNumber = errors.New("providerrequest: invalid language model number")
)

// AddInt64 adds an integer delta while preserving the number's exact variant.
func AddInt64(number provider.LanguageModelNumber, delta int64) (provider.LanguageModelNumber, error) {
	if integer, ok := number.Int64(); ok {
		if delta > 0 && integer > math.MaxInt64-delta || delta < 0 && integer < math.MinInt64-delta {
			return provider.LanguageModelNumber{}, ErrLanguageModelNumberOverflow
		}
		return provider.LanguageModelNumberFromInt64(integer + delta), nil
	}
	floating, ok := number.Float64()
	if !ok {
		return provider.LanguageModelNumber{}, ErrInvalidLanguageModelNumber
	}
	return provider.LanguageModelNumberFromFloat64(floating + float64(delta))
}
