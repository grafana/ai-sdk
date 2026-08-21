package ptr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPointerOperations(t *testing.T) {
	t.Run("to", func(t *testing.T) {
		value := To("value")
		require.NotNil(t, value)
		assert.Equal(t, "value", *value)
	})

	t.Run("clone", func(t *testing.T) {
		original := To("value")
		cloned := Clone(original)
		require.NotNil(t, cloned)
		assert.Equal(t, "value", *cloned)
		assert.NotSame(t, original, cloned)
		assert.Nil(t, Clone[string](nil))
	})

	t.Run("deref", func(t *testing.T) {
		assert.Equal(t, "value", Deref(To("value"), "fallback"))
		assert.Equal(t, "fallback", Deref(nil, "fallback"))
		assert.False(t, Deref(To(false), true))
	})
}
