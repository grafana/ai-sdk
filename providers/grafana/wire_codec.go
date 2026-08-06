package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
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

func codecForMode(mode ProviderWireMode) wireCodec {
	if mode == ProviderWireStrict {
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

func (legacyWireCodec) newStreamReader(reader io.Reader, limit int64) (streamPartReader, error) {
	payloadReader, err := newSSEPayloadReader(reader, limit)
	if err != nil {
		return nil, err
	}
	return &legacyStreamReader{payloadReader: payloadReader}, nil
}

type legacyStreamReader struct {
	payloadReader *ssePayloadReader
}

func (reader *legacyStreamReader) Next() (provider.StreamPart, error) {
	payload, err := reader.payloadReader.next()
	if err != nil {
		return provider.StreamPart{}, err
	}
	var part provider.StreamPart
	err = json.Unmarshal(payload, &part)
	return part, err
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
	strictReader, err := providerwirev4.NewSSEReader(reader, limit)
	if err != nil {
		return nil, err
	}
	return &strictStreamReader{reader: strictReader}, nil
}

type strictStreamReader struct {
	reader *providerwirev4.SSEReader
}

func (reader *strictStreamReader) Next() (provider.StreamPart, error) {
	part, err := reader.reader.Next()
	if err == nil || errors.Is(err, io.EOF) {
		return part, err
	}
	if errors.Is(err, providerwirev4.ErrSSEEventTooLarge) {
		return provider.StreamPart{}, errors.Join(errProtocolResponse, errSSEEventTooLarge, err)
	}
	return provider.StreamPart{}, errors.Join(errProtocolResponse, err)
}

func (strictWireCodec) decodeErrorResponse(response *http.Response, data []byte) (*provider.APICallError, error) {
	return providerwirev4.DecodeErrorResponse(data, response.StatusCode)
}
