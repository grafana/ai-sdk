package v4

import "net/http"

// HostErrorCategory identifies a closed error response available to host code.
type HostErrorCategory uint8

const (
	// HostErrorAuthentication reports a host authentication failure.
	HostErrorAuthentication HostErrorCategory = iota + 1
	// HostErrorPermission reports a host permission failure.
	HostErrorPermission
	// HostErrorInternal reports a host internal failure.
	HostErrorInternal
	HostErrorBYOKDiscovery
)

// HostErrorWriter writes fixed ProviderWire V4 documents for host failures.
type HostErrorWriter struct{}

// NewHostErrorWriter constructs a fixed-document host error writer.
func NewHostErrorWriter() *HostErrorWriter { return &HostErrorWriter{} }

// Write writes the fixed error document for category.
func (*HostErrorWriter) Write(w http.ResponseWriter, category HostErrorCategory) {
	value := safeError{category: safeInternal}
	switch category {
	case HostErrorAuthentication:
		value.category = safeAuthentication
	case HostErrorPermission:
		value.category = safePermission
	case HostErrorBYOKDiscovery:
		value.category = safeBYOKDiscovery
	}
	status, body := encodeHTTPError(value, nil, maxErrorResponseBytes)
	writeErrorResponse(w, status, body)
}
