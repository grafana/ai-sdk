package failure

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_CategoryVocabulary(t *testing.T) {
	tests := []struct {
		category  Category
		retryable bool
	}{
		{CategoryInvalidRequest, false},
		{CategoryAuthentication, false},
		{CategoryPermission, false},
		{CategoryNotFound, false},
		{CategoryRateLimit, true},
		{CategoryOverload, true},
		{CategoryFailedDependency, false},
		{CategoryUpstreamFailure, true},
		{CategoryTimeout, true},
		{CategoryCancellation, false},
		{CategoryInternalFailure, true},
	}
	for _, tc := range tests {
		t.Run(string(tc.category), func(t *testing.T) {
			got, err := New(tc.category, "safe message")
			require.NoError(t, err)
			assert.Equal(t, tc.category, got.Category())
			assert.Equal(t, "safe message", got.Message())
			assert.Equal(t, tc.retryable, got.Retryable())
			assert.True(t, got.Valid())
		})
	}
}

func TestNew_InvalidInput(t *testing.T) {
	_, err := New(Category("unknown"), "safe")
	assert.Error(t, err)
	_, err = New(CategoryInvalidRequest, "")
	assert.Error(t, err)
	assert.False(t, (Failure{}).Valid())
}

func TestFailure_PublicShapeIsCauseFree(t *testing.T) {
	typeOf := reflect.TypeOf(Failure{})
	for i := 0; i < typeOf.NumField(); i++ {
		assert.False(t, typeOf.Field(i).IsExported())
	}
	_, isError := any(Failure{}).(error)
	assert.False(t, isError)
	_, unwraps := any(Failure{}).(interface{ Unwrap() error })
	assert.False(t, unwraps)
}
