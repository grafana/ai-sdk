package v4

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHandler_DefaultsAndLimits(t *testing.T) {
	model := &testModel{}
	resolver := resolverFor(model)
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxRequestBodyBytes, handler.maxRequestBodyBytes)
	assert.Equal(t, DefaultMaxUnaryResponseBytes, handler.maxUnaryResponseBytes)
	assert.Equal(t, DefaultMaxErrorResponseBytes, handler.maxErrorResponseBytes)
	assert.Equal(t, DefaultMaxEventBytes, handler.maxEventBytes)
	assert.Equal(t, DefaultTotalTimeout, handler.totalTimeout)
	assert.Equal(t, DefaultIdleTimeout, handler.idleTimeout)

	handler, err = NewHandler(
		resolver,
		WithMaxRequestBodyBytes(1),
		WithMaxUnaryResponseBytes(1),
		WithMaxErrorResponseBytes(int64(len(canonicalErrorBytes))),
		WithMaxEventBytes(int64(len(canonicalErrorFrame))),
		WithTotalTimeout(time.Millisecond),
		WithIdleTimeout(time.Millisecond),
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), handler.maxRequestBodyBytes)
	assert.Equal(t, int64(1), handler.maxUnaryResponseBytes)
}

func TestNewHandler_RejectsInvalidConstruction(t *testing.T) {
	var typedNil *testResolver
	var nilPolicy PolicyFunc
	tests := []struct {
		name     string
		resolver catalog.ModelResolver
		options  []Option
	}{
		{"nil resolver", nil, nil},
		{"typed nil resolver", typedNil, nil},
		{"nil option", resolverFor(&testModel{}), []Option{nil}},
		{"nil policy", resolverFor(&testModel{}), []Option{WithPolicy(nilPolicy)}},
		{"zero request", resolverFor(&testModel{}), []Option{WithMaxRequestBodyBytes(0)}},
		{"zero unary", resolverFor(&testModel{}), []Option{WithMaxUnaryResponseBytes(0)}},
		{"small error fallback", resolverFor(&testModel{}), []Option{WithMaxErrorResponseBytes(int64(len(canonicalErrorBytes) - 1))}},
		{"small event fallback", resolverFor(&testModel{}), []Option{WithMaxEventBytes(int64(len(canonicalErrorFrame) - 1))}},
		{"zero total timeout", resolverFor(&testModel{}), []Option{WithTotalTimeout(0)}},
		{"zero idle timeout", resolverFor(&testModel{}), []Option{WithIdleTimeout(0)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandler(tc.resolver, tc.options...)
			assert.Error(t, err)
			assert.Nil(t, handler)
		})
	}
}

func TestEmbeddedRequestSchema_HasRegisteredBytes(t *testing.T) {
	data, err := schemaFiles.ReadFile("schema/providerwire-v4-request.schema.json")
	require.NoError(t, err)
	assert.Equal(t, "376b2ecfdb6ab77eb4aa19cee5f94a7579f98d6a91a9cef407d7557131cd8edd", fmt.Sprintf("%x", sha256.Sum256(data)))
}
