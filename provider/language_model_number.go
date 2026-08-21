package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

type languageModelNumberKind uint8

const (
	languageModelNumberInvalid languageModelNumberKind = iota
	languageModelNumberInteger
	languageModelNumberFloat
)

// LanguageModelNumber preserves either an exact signed integer or a finite
// floating-point language-model request setting.
type LanguageModelNumber struct {
	kind    languageModelNumberKind
	integer int64
	float   float64
}

// LanguageModelNumberFromInt constructs an exact integer request number.
func LanguageModelNumberFromInt(value int) LanguageModelNumber {
	return LanguageModelNumberFromInt64(int64(value))
}

// LanguageModelNumberFromInt64 constructs an exact integer request number.
func LanguageModelNumberFromInt64(value int64) LanguageModelNumber {
	return LanguageModelNumber{kind: languageModelNumberInteger, integer: value}
}

// LanguageModelNumberFromFloat64 constructs a finite request number.
func LanguageModelNumberFromFloat64(value float64) (LanguageModelNumber, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return LanguageModelNumber{}, errors.New("provider: language model number must be finite")
	}
	if value == math.Trunc(value) && value >= -math.Ldexp(1, 63) && value < math.Ldexp(1, 63) {
		integer := int64(value)
		if float64(integer) == value {
			return LanguageModelNumberFromInt64(integer), nil
		}
	}
	return LanguageModelNumber{kind: languageModelNumberFloat, float: value}, nil
}

// Int64 returns the exact integer when the number uses the integer variant.
func (n LanguageModelNumber) Int64() (int64, bool) {
	if n.kind != languageModelNumberInteger {
		return 0, false
	}
	return n.integer, true
}

// Float64 returns a lossless float when one is available.
func (n LanguageModelNumber) Float64() (float64, bool) {
	switch n.kind {
	case languageModelNumberFloat:
		return n.float, true
	case languageModelNumberInteger:
		value := float64(n.integer)
		if value >= math.Ldexp(1, 63) || value < -math.Ldexp(1, 63) || int64(value) != n.integer {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

// MarshalJSON preserves exact integers and finite floating-point values for
// generic provider JSON compatibility.
func (n LanguageModelNumber) MarshalJSON() ([]byte, error) {
	switch n.kind {
	case languageModelNumberInteger:
		return strconv.AppendInt(nil, n.integer, 10), nil
	case languageModelNumberFloat:
		if math.IsNaN(n.float) || math.IsInf(n.float, 0) {
			return nil, errors.New("provider: marshaling non-finite language model number")
		}
		return json.Marshal(n.float)
	default:
		return nil, errors.New("provider: marshaling invalid language model number")
	}
}

// UnmarshalJSON decodes generic provider JSON compatibility numbers.
func (n *LanguageModelNumber) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return errors.New("provider: language model number must be a JSON number")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("provider: decoding language model number: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("provider: decoding language model number: trailing data")
	}
	number, ok := value.(json.Number)
	if !ok {
		return errors.New("provider: language model number must be a JSON number")
	}
	token := number.String()
	if isPlainDecimalInteger(token) {
		if integer, err := strconv.ParseInt(token, 10, 64); err == nil {
			*n = LanguageModelNumberFromInt64(integer)
			return nil
		}
	}
	floating, err := strconv.ParseFloat(token, 64)
	if err != nil || math.IsNaN(floating) || math.IsInf(floating, 0) {
		return fmt.Errorf("provider: decoding finite language model number %q", token)
	}
	decoded, err := LanguageModelNumberFromFloat64(floating)
	if err != nil {
		return err
	}
	*n = decoded
	return nil
}

func isPlainDecimalInteger(value string) bool {
	if value == "0" || value == "-0" {
		return true
	}
	start := 0
	if len(value) > 0 && value[0] == '-' {
		start = 1
	}
	if start == len(value) || value[start] < '1' || value[start] > '9' {
		return false
	}
	for i := start + 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
