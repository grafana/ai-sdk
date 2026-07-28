package agentobservability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Context-key types are unexported empty structs so external packages cannot
// collide with these keys. Lookups are O(1) by type identity.
type (
	generationIDContextKey        struct{}
	parentGenerationIDsContextKey struct{}
)

// generationIDByteLength is the entropy budget for NewGenerationID. 16 bytes
// of crypto/rand encoded as 32 hex characters provides 128 bits of entropy,
// which is the same order of magnitude as UUIDv4. The format is documented as
// "opaque string" so callers must not assume any structure.
const generationIDByteLength = 16

// WithGenerationID stores the current generation ID in ctx. RecordingMiddleware
// reads it via GenerationIDFromContext and uses it as GenerationStart.ID when
// non-empty. Generation IDs SHOULD be unique per call; a fresh ID can be
// generated with NewGenerationID.
func WithGenerationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, generationIDContextKey{}, id)
}

// GenerationIDFromContext returns the generation ID stored by WithGenerationID,
// or the empty string when none is set.
func GenerationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(generationIDContextKey{}).(string); ok {
		return id
	}
	return ""
}

// NewGenerationID returns a fresh, opaque generation ID suitable for use as
// the argument to WithGenerationID. The format is 32 hex characters from
// crypto/rand; callers MUST treat the value as opaque (no parsing).
func NewGenerationID() string {
	buf := make([]byte, generationIDByteLength)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand on supported platforms never fails; surface the error
		// as an empty string so callers can defensively handle the edge case
		// without a panic in instrumentation code.
		return ""
	}
	return hex.EncodeToString(buf)
}

// WithParentGenerationIDs appends the given upstream generation IDs to the
// parent set in ctx. The Agent Observability parent → child DAG is built by repeatedly
// calling this helper as a request walks across agents: each `With...` call
// SHALL append, not replace, so a leaf call sees the union of all upstream
// generation IDs.
//
// Empty strings are skipped; duplicate IDs are preserved in order because the
// Agent Observability service treats the list as a sequence, not a set.
func WithParentGenerationIDs(ctx context.Context, ids ...string) context.Context {
	if len(ids) == 0 {
		return ctx
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		return ctx
	}
	existing := ParentGenerationIDsFromContext(ctx)
	combined := make([]string, 0, len(existing)+len(filtered))
	combined = append(combined, existing...)
	combined = append(combined, filtered...)
	return context.WithValue(ctx, parentGenerationIDsContextKey{}, combined)
}

// ParentGenerationIDsFromContext returns the parent generation IDs stored by
// WithParentGenerationIDs (in the order they were added), or nil when none
// are set.
//
// The returned slice is a defensive copy; callers MAY mutate it freely.
func ParentGenerationIDsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	ids, ok := ctx.Value(parentGenerationIDsContextKey{}).([]string)
	if !ok || len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	copy(out, ids)
	return out
}

// WithLinkedGenerationID adds a sibling/peer linkage ID to the parent set.
//
// Semantically a "linked" generation is a sibling of the current call — e.g.
// an evaluation generation that complements a primary generation — rather
// than a true ancestor. The underlying SDK wire schema represents this with the same
// ParentGenerationIDs list, so this helper is currently an alias for
// WithParentGenerationIDs(ctx, id). The dedicated entry point exists so
// callers express intent at the call site and so the semantics can split
// later without a wire-level migration.
func WithLinkedGenerationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return WithParentGenerationIDs(ctx, id)
}
