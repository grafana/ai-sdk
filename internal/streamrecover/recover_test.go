package streamrecover

import (
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecover(t *testing.T) {
	t.Run("panic calls onPanic and discards the value", func(t *testing.T) {
		called := false
		func() {
			defer Recover(func() { called = true })
			panic("secret-value")
		}()
		assert.True(t, called)
	})

	t.Run("no panic skips onPanic", func(t *testing.T) {
		called := false
		func() {
			defer Recover(func() { called = true })
		}()
		assert.False(t, called)
	})
}

func TestPart(t *testing.T) {
	part := Part()
	assert.Equal(t, provider.PartError, part.Type)
	require.NotNil(t, part.APICallError)
	assert.True(t, errors.Is(part.APICallError, ErrPanic))
	assert.False(t, part.APICallError.IsRetryable)
	assert.Zero(t, part.APICallError.StatusCode)
	assert.NotSame(t, part.APICallError, Part().APICallError)
}
