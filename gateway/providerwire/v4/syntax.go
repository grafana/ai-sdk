package providerwirev4

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/go-json-experiment/json/jsontext"
)

func validateStrictJSON(src []byte) ([]byte, error) {
	decoder := jsontext.NewDecoder(bytes.NewReader(src))
	if _, err := decoder.ReadValue(); err != nil {
		return nil, fmt.Errorf("invalid-json-syntax: %w", err)
	}
	if _, err := decoder.ReadValue(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("invalid-json-syntax: trailing value")
		}
		return nil, fmt.Errorf("invalid-json-syntax: %w", err)
	}
	return src, nil
}
