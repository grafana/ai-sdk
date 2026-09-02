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
)

// HostErrorWriter writes fixed ProviderWire V4 documents for host failures.
type HostErrorWriter struct{}

// NewHostErrorWriter constructs a fixed-document host error writer.
func NewHostErrorWriter() *HostErrorWriter { return &HostErrorWriter{} }

// Write writes the fixed error document for category.
func (*HostErrorWriter) Write(w http.ResponseWriter, category HostErrorCategory) {
	document := safeErrorDocument{status: http.StatusInternalServerError, body: canonicalInternalError}
	switch category {
	case HostErrorAuthentication:
		document = safeErrorDocument{status: http.StatusUnauthorized, body: canonicalAuthenticationError}
	case HostErrorPermission:
		document = safeErrorDocument{status: http.StatusForbidden, body: canonicalPermissionError}
	case HostErrorInternal:
	}
	writeSafeErrorDocument(w, document)
}
