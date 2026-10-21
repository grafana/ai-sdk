package streamrecover

import (
	"errors"
	"fmt"
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

type panickingIsError struct{}

func (panickingIsError) Error() string { return "panicking is" }
func (panickingIsError) Is(error) bool { panic("secret-value") }

type panickingCloser struct{}

func (panickingCloser) Close() error { panic("secret-value") }

func TestIs(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "contained panic", err: Error(), want: true},
		{name: "wrapped sentinel", err: fmt.Errorf("stream: %w", ErrPanic), want: true},
		{name: "other error", err: errors.New("other")},
		{name: "typed nil cause", err: provider.NewAPICallError(provider.APICallErrorOptions{Cause: (*provider.APICallError)(nil)})},
		{name: "panicking Is method", err: provider.NewAPICallError(provider.APICallErrorOptions{Cause: panickingIsError{}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Is(tc.err))
		})
	}
}

func TestClose(t *testing.T) {
	assert.NotPanics(t, func() { Close(panickingCloser{}) })
}
