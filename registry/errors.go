package registry

import (
	"errors"
	"fmt"
)

var (
	ErrNoSuchModel    = errors.New("registry: no such model")
	ErrNoSuchProvider = fmt.Errorf("registry: no such provider: %w", ErrNoSuchModel)
	ErrInvalidModelID = fmt.Errorf("registry: invalid model ID: %w", ErrNoSuchModel)
)
