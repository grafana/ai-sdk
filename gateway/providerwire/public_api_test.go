package providerwire_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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

	callData, err := providerwire.EncodeCallOptions(provider.CallOptions{})
	require.NoError(t, err)
	_, err = providerwire.DecodeCallOptions(callData)
	require.NoError(t, err)

	resultData, err := providerwire.EncodeGenerateResult(&provider.GenerateResult{})
	require.NoError(t, err)
	_, err = providerwire.DecodeGenerateResult(resultData)
	require.NoError(t, err)

	var stream bytes.Buffer
	part := provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: ""}
	require.NoError(t, providerwire.WriteSSEStreamPart(&stream, part))
	reader := providerwire.NewSSEReader(&stream)
	assert.IsType(t, (*providerwire.SSEReader)(nil), reader)
	decodedPart, err := reader.Next()
	require.NoError(t, err)
	assert.Equal(t, part, decodedPart)
	_, err = reader.Next()
	assert.ErrorIs(t, err, io.EOF)

	recorder := httptest.NewRecorder()
	require.NoError(t, providerwire.WriteSSEStreamPartTo(recorder, part))

	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     "failure",
		StatusCode:  http.StatusBadGateway,
		IsRetryable: boolPointer(true),
	})
	errorData, err := providerwire.EncodeAPICallError(apiErr)
	require.NoError(t, err)
	_, err = providerwire.DecodeAPICallError(errorData)
	require.NoError(t, err)

	errorRecorder := httptest.NewRecorder()
	require.NoError(t, providerwire.WriteErrorResponse(errorRecorder, apiErr))
	decodedError, err := providerwire.DecodeErrorResponse(&http.Response{
		StatusCode: errorRecorder.Code,
		Header:     errorRecorder.Header(),
		Body:       io.NopCloser(bytes.NewReader(errorRecorder.Body.Bytes())),
	})
	require.NoError(t, err)
	assert.Equal(t, apiErr.Message, decodedError.Message)
}

func boolPointer(value bool) *bool { return &value }
