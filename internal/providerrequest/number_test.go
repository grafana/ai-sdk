package providerrequest

import (
	"math"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddInt64(t *testing.T) {
	fraction, err := provider.LanguageModelNumberFromFloat64(100.5)
	require.NoError(t, err)
	tests := []struct {
		name      string
		number    provider.LanguageModelNumber
		delta     int64
		wantInt   *int64
		wantFloat *float64
		wantError error
	}{
		{name: "integer", number: provider.LanguageModelNumberFromInt64(9007199254740993), delta: 1024, wantInt: int64Pointer(9007199254742017)},
		{name: "fraction", number: fraction, delta: 1024, wantFloat: float64Pointer(1124.5)},
		{name: "positive overflow", number: provider.LanguageModelNumberFromInt64(math.MaxInt64), delta: 1, wantError: ErrLanguageModelNumberOverflow},
		{name: "negative overflow", number: provider.LanguageModelNumberFromInt64(math.MinInt64), delta: -1, wantError: ErrLanguageModelNumberOverflow},
		{name: "invalid", number: provider.LanguageModelNumber{}, delta: 1, wantError: ErrInvalidLanguageModelNumber},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AddInt64(tc.number, tc.delta)
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			if tc.wantInt != nil {
				value, ok := got.Int64()
				require.True(t, ok)
				assert.Equal(t, *tc.wantInt, value)
			}
			if tc.wantFloat != nil {
				value, ok := got.Float64()
				require.True(t, ok)
				assert.Equal(t, *tc.wantFloat, value)
			}
		})
	}
}

func int64Pointer(value int64) *int64       { return &value }
func float64Pointer(value float64) *float64 { return &value }
