package catalog

import (
	"errors"
	"fmt"
)

// ErrUnknownModel identifies a public model ID absent from a catalog.
var ErrUnknownModel = errors.New("catalog: unknown model")

// UnknownModelError reports the requested public model ID.
type UnknownModelError struct {
	// ModelID is the exact public ID supplied to model resolution.
	ModelID string
}

// Error implements error.
func (e *UnknownModelError) Error() string {
	return fmt.Sprintf("catalog: unknown model %q", e.ModelID)
}

// Unwrap makes UnknownModelError match ErrUnknownModel.
func (e *UnknownModelError) Unwrap() error { return ErrUnknownModel }
