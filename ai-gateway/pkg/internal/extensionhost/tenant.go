package extensionhost

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
)

// TenantResource is one stored Middleware resource of a namespace, as read.
type TenantResource struct {
	// Name is the resource's metadata.name, which must equal Type.
	Name    string
	Type    string
	Enabled bool
	// Config is the type's member as canonical JSON, with numbers as stored,
	// or nil when the spec sets no member.
	Config json.RawMessage
	// Invalid reports a stored spec the Gateway cannot apply: one that can't
	// be decoded, or that sets a member other than its type's. Only Name is
	// meaningful then.
	Invalid bool
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

const (
	readSuccess = "success"
	readFailed  = "failed"
)

// readClass returns the fixed class of a read failure.
func readClass(err error) string {
	switch {
	case errors.Is(err, ErrReadRefused):
		return "refused"
	case errors.Is(err, ErrReadNotFound):
		return "not-found"
	case errors.Is(err, ErrReadStatus):
		return "unexpected-status"
	case errors.Is(err, ErrReadTimeout), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	case errors.Is(err, ErrReadInvalid):
		return "invalid-response"
	default:
		return "error"
	}
}

// recordReadFailure counts and logs one failed read without its text.
func (h *Host) recordReadFailure(namespace string, err error) {
	h.reads.WithLabelValues(readFailed).Inc()
	h.logger.Warn("gateway tenant configuration read failed", "namespace", namespace, "class", readClass(err))
}

// tenantDefinition reports whether name is a registered tenant extension.
func (h *Host) tenantDefinition(name string) bool {
	_, found := slices.BinarySearchFunc(h.tenant, name, func(definition extension.Definition, name string) int { return strings.Compare(definition.Name, name) })
	return found
}
