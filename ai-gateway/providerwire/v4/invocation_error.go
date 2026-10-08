package v4

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

const maxErrorResponseBytes = 64 << 10

type errorType string
type errorCode string

type invocationErrorDocument struct {
	Type     provider.StreamPartType   `json:"type,omitempty"`
	Error    invocationErrorFields     `json:"error"`
	Metadata provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

type invocationErrorFields struct {
	Message    string               `json:"message"`
	Type       errorType            `json:"type"`
	Param      json.RawMessage      `json:"param"`
	Code       errorCode            `json:"code"`
	StatusCode int                  `json:"statusCode,omitempty"`
	Retryable  *bool                `json:"retryable,omitempty"`
	Data       *invocationErrorData `json:"data,omitempty"`
}

type invocationErrorData struct {
	Metadata    provider.ProviderMetadata `json:"providerMetadata,omitzero"`
	NativeError *execution.Failure        `json:"nativeError,omitempty"`
}

func enrichErrorDocument(original []byte, streaming bool, value safeError, limit int64) []byte {
	if value.invocation == nil || int64(len(original)) > limit {
		return original
	}
	call := value.invocation
	var envelope invocationErrorDocument
	if json.Unmarshal(original, &envelope) != nil {
		return original
	}
	var current *execution.Failure
	if streaming {
		current = call.current(value.nativeError)
	}
	if call.overview == nil && current == nil {
		return original
	}
	if streaming {
		envelope.Error.Data = &invocationErrorData{NativeError: current}
	}
	setMetadata := func(metadata provider.ProviderMetadata) {
		if streaming {
			envelope.Error.Data.Metadata = metadata
		} else {
			envelope.Metadata = metadata
		}
	}
	metadata := execution.Metadata(call.overview, nil, func(metadata provider.ProviderMetadata) bool {
		setMetadata(metadata)
		body, err := json.Marshal(envelope)
		return err == nil && int64(len(body)) <= limit
	})
	setMetadata(metadata)
	if len(metadata) == 0 && current == nil {
		return original
	}
	body, err := json.Marshal(envelope)
	if err != nil || int64(len(body)) > limit {
		return original
	}
	return body
}
