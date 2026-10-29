package v4

import (
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
)

// HostErrorWriter writes fixed ProviderWire V4 documents for host failures.
type HostErrorWriter struct{}

// NewHostErrorWriter constructs a fixed-document host error writer.
func NewHostErrorWriter() *HostErrorWriter { return &HostErrorWriter{} }

// WriteHostError writes the fixed error document for category.
func (*HostErrorWriter) WriteHostError(w http.ResponseWriter, category host.ErrorCategory) {
	value := safeError{category: safeInternal}
	switch category {
	case host.ErrorPermission:
		value.category = safePermission
	case host.ErrorBYOKDiscovery:
		value.category = safeBYOKDiscovery
	}
	status, body := encodeHTTPError(value, nil)
	writeErrorResponse(w, status, body)
}
