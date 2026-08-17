package providerwire_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type externalModelResolver struct{}

func (externalModelResolver) ResolveLanguageModel(*http.Request, string) (provider.LanguageModel, error) {
	return nil, nil
}

var (
	_ providerwire.ModelResolver     = providerwire.ModelResolverFunc(nil)
	_ providerwire.ModelResolver     = externalModelResolver{}
	_ providerwire.ModelResolverFunc = func(*http.Request, string) (provider.LanguageModel, error) {
		return nil, nil
	}
	_ http.Handler = (*providerwire.Handler)(nil)

	_ string        = providerwire.PathLanguageModel
	_ string        = providerwire.HeaderModelID
	_ string        = providerwire.HeaderStreaming
	_ string        = providerwire.HeaderSpecVersion
	_ string        = providerwire.SpecVersionV4
	_ string        = providerwire.MIMEJSON
	_ string        = providerwire.MIMESSE
	_ time.Duration = providerwire.DefaultTotalTimeout
	_ time.Duration = providerwire.DefaultIdleTimeout
	_ int64         = providerwire.DefaultMaxRequestBodyBytes
	_ error         = providerwire.ErrTotalTimeout
	_ error         = providerwire.ErrIdleTimeout

	_ func(providerwire.ModelResolver, ...providerwire.Option) (*providerwire.Handler, error) = providerwire.NewHandler
	_ func(time.Duration) providerwire.Option                                                 = providerwire.WithTotalTimeout
	_ func(time.Duration) providerwire.Option                                                 = providerwire.WithIdleTimeout
	_ func(int64) providerwire.Option                                                         = providerwire.WithMaxRequestBodyBytes
	_ func(provider.CallOptions) ([]byte, error)                                              = providerwire.EncodeCallOptions
	_ func([]byte) (provider.CallOptions, error)                                              = providerwire.DecodeCallOptions
	_ func(*provider.GenerateResult) ([]byte, error)                                          = providerwire.EncodeGenerateResult
	_ func([]byte) (*provider.GenerateResult, error)                                          = providerwire.DecodeGenerateResult
	_ func(io.Writer, provider.StreamPart) error                                              = providerwire.WriteSSEStreamPart
	_ func(http.ResponseWriter, provider.StreamPart) error                                    = providerwire.WriteSSEStreamPartTo
	_ func(io.Reader) *providerwire.SSEReader                                                 = providerwire.NewSSEReader
	_ func(*providerwire.SSEReader) (provider.StreamPart, error)                              = (*providerwire.SSEReader).Next
	_ func(*provider.APICallError) ([]byte, error)                                            = providerwire.EncodeAPICallError
	_ func([]byte) (*provider.APICallError, error)                                            = providerwire.DecodeAPICallError
	_ func(http.ResponseWriter, *provider.APICallError) error                                 = providerwire.WriteErrorResponse
	_ func(*http.Response) (*provider.APICallError, error)                                    = providerwire.DecodeErrorResponse
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

	handler, err := providerwire.NewHandler(
		externalModelResolver{},
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
