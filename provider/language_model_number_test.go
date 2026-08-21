package provider

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLanguageModelNumber_ConstructorsAndAccessors(t *testing.T) {
	tests := []struct {
		name        string
		construct   func() (LanguageModelNumber, error)
		wantInt     int64
		wantIntOK   bool
		wantFloat   float64
		wantFloatOK bool
	}{
		{name: "int", construct: func() (LanguageModelNumber, error) { return LanguageModelNumberFromInt(42), nil }, wantInt: 42, wantIntOK: true, wantFloat: 42, wantFloatOK: true},
		{name: "large int", construct: func() (LanguageModelNumber, error) { return LanguageModelNumberFromInt64(9007199254740993), nil }, wantInt: 9007199254740993, wantIntOK: true},
		{name: "exact large float-compatible int", construct: func() (LanguageModelNumber, error) { return LanguageModelNumberFromInt64(9007199254740992), nil }, wantInt: 9007199254740992, wantIntOK: true, wantFloat: 9007199254740992, wantFloatOK: true},
		{name: "fraction", construct: func() (LanguageModelNumber, error) { return LanguageModelNumberFromFloat64(1.5) }, wantFloat: 1.5, wantFloatOK: true},
		{name: "integral float", construct: func() (LanguageModelNumber, error) { return LanguageModelNumberFromFloat64(42) }, wantInt: 42, wantIntOK: true, wantFloat: 42, wantFloatOK: true},
		{name: "negative zero", construct: func() (LanguageModelNumber, error) { return LanguageModelNumberFromFloat64(math.Copysign(0, -1)) }, wantInt: 0, wantIntOK: true, wantFloat: 0, wantFloatOK: true},
		{name: "outside int64", construct: func() (LanguageModelNumber, error) { return LanguageModelNumberFromFloat64(math.Ldexp(1, 63)) }, wantFloat: math.Ldexp(1, 63), wantFloatOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			number, err := tc.construct()
			require.NoError(t, err)
			gotInt, gotIntOK := number.Int64()
			assert.Equal(t, tc.wantIntOK, gotIntOK)
			if tc.wantIntOK {
				assert.Equal(t, tc.wantInt, gotInt)
			}
			gotFloat, gotFloatOK := number.Float64()
			assert.Equal(t, tc.wantFloatOK, gotFloatOK)
			if tc.wantFloatOK {
				assert.Equal(t, tc.wantFloat, gotFloat)
			}
		})
	}
}

func TestLanguageModelNumber_RejectsInvalidValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := LanguageModelNumberFromFloat64(value)
		require.Error(t, err)
	}

	_, err := json.Marshal(LanguageModelNumber{})
	require.Error(t, err)
}

func TestLanguageModelNumber_JSONCompatibility(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantJSON    string
		wantInt     int64
		wantIntOK   bool
		wantFloat   float64
		wantFloatOK bool
	}{
		{name: "large integer", input: "9007199254740993", wantJSON: "9007199254740993", wantInt: 9007199254740993, wantIntOK: true},
		{name: "fraction", input: "2.5", wantJSON: "2.5", wantFloat: 2.5, wantFloatOK: true},
		{name: "exponent canonicalizes", input: "1e3", wantJSON: "1000", wantInt: 1000, wantIntOK: true, wantFloat: 1000, wantFloatOK: true},
		{name: "outside int64", input: "9223372036854775808", wantJSON: "9223372036854776000", wantFloat: math.Ldexp(1, 63), wantFloatOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var number LanguageModelNumber
			require.NoError(t, json.Unmarshal([]byte(tc.input), &number))
			encoded, err := json.Marshal(number)
			require.NoError(t, err)
			assert.Equal(t, tc.wantJSON, string(encoded))
			gotInt, gotIntOK := number.Int64()
			assert.Equal(t, tc.wantIntOK, gotIntOK)
			if tc.wantIntOK {
				assert.Equal(t, tc.wantInt, gotInt)
			}
			gotFloat, gotFloatOK := number.Float64()
			assert.Equal(t, tc.wantFloatOK, gotFloatOK)
			if tc.wantFloatOK {
				assert.Equal(t, tc.wantFloat, gotFloat)
			}
		})
	}
}

func TestLanguageModelNumber_JSONCompatibilityRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"null", `"1"`, "true", "{}", "[]", "1e400", "1 2", ""} {
		t.Run(input, func(t *testing.T) {
			var number LanguageModelNumber
			require.Error(t, json.Unmarshal([]byte(input), &number))
			assert.Equal(t, LanguageModelNumber{}, number)
		})
	}
}

func languageModelIntPointer(value int) *LanguageModelNumber {
	number := LanguageModelNumberFromInt(value)
	return &number
}

func stringPtr(value string) *string { return &value }
