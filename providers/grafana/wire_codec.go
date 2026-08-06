package grafana

import (
	"bytes"
	"io"
	"net/http"

	"github.com/grafana/ai-sdk/gateway/providerwire"
	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

type wireCodec interface {
	encodeCallOptions(provider.CallOptions) ([]byte, error)
	decodeGenerateResult([]byte) (*provider.GenerateResult, error)
	newStreamReader(io.Reader, int64) (streamPartReader, error)
	decodeErrorResponse(*http.Response, []byte) (*provider.APICallError, error)
}

type streamPartReader interface {
	Next() (provider.StreamPart, error)
}

type legacyWireCodec struct{}
type strictWireCodec struct{}

func codecForStrictMode(strict bool) wireCodec {
	if strict {
		return strictWireCodec{}
	}
	return legacyWireCodec{}
}

func (legacyWireCodec) encodeCallOptions(options provider.CallOptions) ([]byte, error) {
	return providerwire.EncodeCallOptions(options)
}

func (legacyWireCodec) decodeGenerateResult(data []byte) (*provider.GenerateResult, error) {
	return providerwire.DecodeGenerateResult(data)
}

func (legacyWireCodec) newStreamReader(reader io.Reader, _ int64) (streamPartReader, error) {
	return providerwire.NewSSEReader(reader), nil
}

func (legacyWireCodec) decodeErrorResponse(response *http.Response, data []byte) (*provider.APICallError, error) {
	clone := *response
	clone.Body = io.NopCloser(bytes.NewReader(data))
	return providerwire.DecodeErrorResponse(&clone)
}

func (strictWireCodec) encodeCallOptions(options provider.CallOptions) ([]byte, error) {
	return providerwirev4.EncodeCallOptions(options)
}

func (strictWireCodec) decodeGenerateResult(data []byte) (*provider.GenerateResult, error) {
	return providerwirev4.DecodeGenerateResult(data)
}

func (strictWireCodec) newStreamReader(reader io.Reader, limit int64) (streamPartReader, error) {
	return providerwirev4.NewSSEReader(reader, limit)
}

func (strictWireCodec) decodeErrorResponse(response *http.Response, data []byte) (*provider.APICallError, error) {
	return providerwirev4.DecodeErrorResponse(data, response.StatusCode)
}
