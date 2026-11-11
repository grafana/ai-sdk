package host

import "net/http"

// ErrorCategory is a closed host failure that an adapter encodes in its own
// error format.
type ErrorCategory uint8

const (
	// ErrorPermission reports a caller without account access.
	ErrorPermission ErrorCategory = iota + 1
	// ErrorInternal reports a host failure.
	ErrorInternal
	// ErrorBYOKDiscovery reports that catalog discovery is unsupported for BYOK.
	ErrorBYOKDiscovery
)

// ErrorWriter writes an adapter's fixed document for a host failure. A
// category outside the closed set is written as [ErrorInternal].
type ErrorWriter interface {
	WriteHostError(w http.ResponseWriter, category ErrorCategory)
}
