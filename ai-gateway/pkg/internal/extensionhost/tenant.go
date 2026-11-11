package extensionhost

import (
	"context"
	"encoding/json"
	"errors"
)

// TenantResource is one stored Middleware resource of a namespace, as read.
type TenantResource struct {
	// Name is the resource's metadata.name, which must equal Type.
	Name string
	Type string
	// Enabled is nil when the stored resource has no enabled flag.
	Enabled *bool
	// Config is the stored spec.config as compact JSON, or an empty object
	// when it is absent or null. Numbers keep their stored literal.
	Config json.RawMessage
}

// TenantReader reads a namespace's stored tenant configuration. Read returns
// the namespace's resources sorted by name; an empty result means the
// namespace has none. Every failure is an error, never an empty result.
type TenantReader interface {
	Read(ctx context.Context, namespace string) ([]TenantResource, error)
}

// Read failures a TenantReader classifies. Their text never carries a
// response body or configuration value.
var (
	ErrReadRefused  = errors.New("tenant configuration: the platform refused the read")
	ErrReadNotFound = errors.New("tenant configuration: the platform does not serve the resources")
	ErrReadStatus   = errors.New("tenant configuration: the platform answered with an unexpected status")
	ErrReadTimeout  = errors.New("tenant configuration: the read timed out")
	ErrReadInvalid  = errors.New("tenant configuration: the platform's response is invalid")
	ErrReadFailed   = errors.New("tenant configuration: the read failed")
)
