package agentobservability

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerationID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, GenerationIDFromContext(ctx), "zero value should be empty string")

	ctx = WithGenerationID(ctx, "gen-123")
	assert.Equal(t, "gen-123", GenerationIDFromContext(ctx))
}

func TestGenerationID_NilContext(t *testing.T) {
	// Deliberately exercising the nil-context guard. SA1012 fires because
	// the standard library frowns on this, but the helpers are documented
	// to tolerate it gracefully.
	assert.Empty(t, GenerationIDFromContext(nil), "nil context returns empty string")   //nolint:staticcheck
	assert.Nil(t, ParentGenerationIDsFromContext(nil), "nil context returns nil slice") //nolint:staticcheck
}

func TestNewGenerationID(t *testing.T) {
	id := NewGenerationID()
	require.Len(t, id, generationIDByteLength*2, "ID is hex-encoded 16 bytes = 32 chars")
	decoded, err := hex.DecodeString(id)
	require.NoError(t, err)
	assert.Len(t, decoded, generationIDByteLength)

	// Repeated calls produce different IDs.
	other := NewGenerationID()
	assert.NotEqual(t, id, other)
}

func TestParentGenerationIDs_AppendNotReplace(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, ParentGenerationIDsFromContext(ctx))

	ctx = WithParentGenerationIDs(ctx, "p1", "p2")
	assert.Equal(t, []string{"p1", "p2"}, ParentGenerationIDsFromContext(ctx))

	// Second call appends, doesn't replace.
	ctx = WithParentGenerationIDs(ctx, "p3")
	assert.Equal(t, []string{"p1", "p2", "p3"}, ParentGenerationIDsFromContext(ctx))

	// Third call with multiple IDs appends each in order.
	ctx = WithParentGenerationIDs(ctx, "p4", "p5")
	assert.Equal(t, []string{"p1", "p2", "p3", "p4", "p5"}, ParentGenerationIDsFromContext(ctx))
}

func TestParentGenerationIDs_EmptyInputs(t *testing.T) {
	ctx := context.Background()
	ctx = WithParentGenerationIDs(ctx)
	assert.Nil(t, ParentGenerationIDsFromContext(ctx), "no args -> no change")

	ctx = WithParentGenerationIDs(ctx, "", "")
	assert.Nil(t, ParentGenerationIDsFromContext(ctx), "only empty strings -> no change")

	ctx = WithParentGenerationIDs(ctx, "", "p1", "")
	assert.Equal(t, []string{"p1"}, ParentGenerationIDsFromContext(ctx), "empty strings filtered out")
}

func TestParentGenerationIDs_DefensiveCopy(t *testing.T) {
	ctx := WithParentGenerationIDs(context.Background(), "p1", "p2")
	got := ParentGenerationIDsFromContext(ctx)
	got[0] = "mutated"

	// The next read should still see the original values; the previous return
	// must be a defensive copy so caller mutations don't bleed into the
	// context.
	again := ParentGenerationIDsFromContext(ctx)
	assert.Equal(t, []string{"p1", "p2"}, again)
}

func TestWithLinkedGenerationID(t *testing.T) {
	ctx := WithLinkedGenerationID(context.Background(), "link-1")
	assert.Equal(t, []string{"link-1"}, ParentGenerationIDsFromContext(ctx),
		"linked ID is stored alongside parents")

	// Empty input is a no-op.
	ctx2 := WithLinkedGenerationID(context.Background(), "")
	assert.Nil(t, ParentGenerationIDsFromContext(ctx2))
}
