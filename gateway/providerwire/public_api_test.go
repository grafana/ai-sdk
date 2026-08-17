package providerwire_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ providerwire.ModelResolver = providerwire.ModelResolverFunc(nil)
	_ http.Handler               = (*providerwire.Handler)(nil)
)

func TestPublicAPI_SourceCompatibility(t *testing.T) {
	assert.Equal(t, "/language-model", providerwire.PathLanguageModel)
	assert.Equal(t, "ai-language-model-id", providerwire.HeaderModelID)
	assert.Equal(t, "ai-language-model-streaming", providerwire.HeaderStreaming)
	assert.Equal(t, "ai-language-model-specification-version", providerwire.HeaderSpecVersion)
	assert.Equal(t, "4", providerwire.SpecVersionV4)
	assert.Equal(t, "application/json", providerwire.MIMEJSON)
	assert.Equal(t, "text/event-stream", providerwire.MIMESSE)
	assert.Equal(t, 120*time.Second, providerwire.DefaultTotalTimeout)
	assert.Equal(t, 60*time.Second, providerwire.DefaultIdleTimeout)
	assert.Equal(t, int64(8<<20), providerwire.DefaultMaxRequestBodyBytes)
	require.NotNil(t, providerwire.ErrTotalTimeout)
	require.NotNil(t, providerwire.ErrIdleTimeout)

	resolver := providerwire.ModelResolverFunc(func(*http.Request, string) (provider.LanguageModel, error) {
		return nil, nil
	})
	handler, err := providerwire.NewHandler(
		resolver,
		providerwire.WithTotalTimeout(time.Second),
		providerwire.WithIdleTimeout(time.Second),
		providerwire.WithMaxRequestBodyBytes(1),
	)
	require.NoError(t, err)
	require.NotNil(t, handler)

	assert.NotNil(t, providerwire.EncodeCallOptions)
	assert.NotNil(t, providerwire.DecodeCallOptions)
	assert.NotNil(t, providerwire.EncodeGenerateResult)
	assert.NotNil(t, providerwire.DecodeGenerateResult)
	assert.NotNil(t, providerwire.WriteSSEStreamPart)
	assert.NotNil(t, providerwire.WriteSSEStreamPartTo)
	assert.NotNil(t, providerwire.NewSSEReader)
	assert.NotNil(t, providerwire.EncodeAPICallError)
	assert.NotNil(t, providerwire.DecodeAPICallError)
	assert.NotNil(t, providerwire.WriteErrorResponse)
	assert.NotNil(t, providerwire.DecodeErrorResponse)
	var reader *providerwire.SSEReader
	assert.Nil(t, reader)
}
