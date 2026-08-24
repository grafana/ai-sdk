package v4

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"github.com/grafana/ai-sdk/schema"
)

// HostErrorCategory identifies a closed error response available to host code.
type HostErrorCategory uint8

const (
	// HostErrorAuthentication reports a host authentication failure.
	HostErrorAuthentication HostErrorCategory = iota + 1
	// HostErrorInternal reports a host internal failure.
	HostErrorInternal
)

// HostErrorWriter writes bounded ProviderWire V4 errors for host failures.
type HostErrorWriter struct {
	errorResponseBytes int64
	errorSchema        *schema.CompiledSchema
}

// NewHostErrorWriter constructs a bounded host error writer.
func NewHostErrorWriter(errorResponseBytes int64) (*HostErrorWriter, error) {
	if err := validateErrorResponseBytes(errorResponseBytes); err != nil {
		return nil, err
	}
	errorSchema, err := schema.CompileSchema(errorSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("providerwire v4: compiling error schema: %w", err)
	}
	if err := errorSchema.Validate(json.RawMessage(canonicalInternalError)); err != nil {
		return nil, fmt.Errorf("providerwire v4: validating canonical internal error: %w", err)
	}
	return &HostErrorWriter{errorResponseBytes: errorResponseBytes, errorSchema: errorSchema}, nil
}

// Write writes the fixed error document for category.
func (w *HostErrorWriter) Write(dst http.ResponseWriter, category HostErrorCategory) {
	value := safeError{}
	switch category {
	case HostErrorAuthentication:
		value.category = safeAuthentication
	case HostErrorInternal:
		value.category = safeInternal
	}
	writeSafeError(dst, value, w.errorResponseBytes, w.errorSchema)
}

func validateErrorResponseBytes(limit int64) error {
	if limit <= 0 {
		return fmt.Errorf("providerwire v4: error response bytes must be positive")
	}
	if limit == math.MaxInt64 {
		return fmt.Errorf("providerwire v4: error response bytes cannot safely use limit+1")
	}
	if int64(len(canonicalInternalError)) > limit {
		return fmt.Errorf("providerwire v4: error response bytes cannot contain canonical internal error")
	}
	return nil
}

func writeSafeError(w http.ResponseWriter, value safeError, limit int64, errorSchema *schema.CompiledSchema) {
	body, status, ok := encodeSafeError(value, limit)
	if ok && errorSchema.Validate(json.RawMessage(body)) == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(canonicalInternalError)
}
