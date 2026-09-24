package v4

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type reasoningTextPart struct {
	Type     provider.GenerateContentType `json:"type"`
	Text     string                       `json:"text"`
	Metadata *provider.ProviderMetadata   `json:"providerMetadata,omitempty"`
}

type reasoningFilePart struct {
	Type      string                     `json:"type"`
	MediaType string                     `json:"mediaType"`
	Data      reasoningWireFileData      `json:"data"`
	Metadata  *provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

type reasoningWireFileData struct {
	Type string  `json:"type"`
	Data *string `json:"data,omitempty"`
	URL  *string `json:"url,omitempty"`
}

// Called only after bounded preflight and validation. Do not expose provider
// marshalers at the strict service boundary.
func projectReasoningFile(data *provider.StreamFileData) reasoningWireFileData {
	if data.Type == provider.StreamFileDataTypeURL || (data.Type == "" && data.URL != "") {
		return reasoningWireFileData{Type: "url", URL: &data.URL}
	}
	encoded := data.Base64
	if data.Bytes != nil {
		encoded = base64.StdEncoding.EncodeToString(data.Bytes)
	}
	return reasoningWireFileData{Type: "data", Data: &encoded}
}

func reasoningMetadataFits(metadata provider.ProviderMetadata, remaining *int64) bool {
	if int64(len(metadata)) > *remaining/2 {
		return false
	}
	for key, raw := range metadata {
		cost := int64(len(key)) + int64(len(raw))
		if cost > *remaining {
			return false
		}
		*remaining -= cost
	}
	return true
}

func projectReasoningMetadata(metadata provider.ProviderMetadata, limit int64) (*provider.ProviderMetadata, error) {
	if metadata == nil {
		return nil, nil
	}
	if !reasoningMetadataFits(metadata, &limit) {
		return nil, errInvalidUnarySuccess
	}
	projected := make(provider.ProviderMetadata)
	for namespace, raw := range metadata {
		if !utf8.ValidString(namespace) || !utf8.Valid(raw) {
			return nil, errInvalidUnarySuccess
		}
		if namespace != "anthropic" && namespace != "bedrock" && namespace != "amazonBedrock" && namespace != "openai" {
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return nil, errInvalidUnarySuccess
		}
		selected := make(map[string]json.RawMessage)
		for key, value := range fields {
			allowed := (namespace == "anthropic" && (key == "signature" || key == "redactedData")) ||
				((namespace == "bedrock" || namespace == "amazonBedrock") && (key == "signature" || key == "redactedData" || key == "redactedContent")) ||
				(namespace == "openai" && (key == "itemId" || key == "reasoningEncryptedContent"))
			if !allowed {
				continue
			}
			var text *string
			if json.Unmarshal(value, &text) != nil || (text == nil && (namespace != "openai" || key != "reasoningEncryptedContent")) {
				return nil, errInvalidUnarySuccess
			}
			selected[key] = value
		}
		encoded, err := json.Marshal(selected)
		if err != nil {
			return nil, errInvalidUnarySuccess
		}
		projected[namespace] = encoded
	}
	return &projected, nil
}

func reasoningFileFits(data *provider.StreamFileData, mediaType string, remaining *int64) bool {
	if data == nil {
		return false
	}
	for _, size := range []int{len(mediaType), len(data.Base64), len(data.URL)} {
		if int64(size) > *remaining {
			return false
		}
		*remaining -= int64(size)
	}
	if int64(len(data.Bytes)) > (*remaining/4)*3 {
		return false
	}
	encoded := (int64(len(data.Bytes)) + 2) / 3 * 4
	if encoded > *remaining {
		return false
	}
	*remaining -= encoded
	return true
}

func unaryReasoningFile(data *provider.DataContent) *provider.StreamFileData {
	if data == nil || data.Validate() != nil {
		return nil
	}
	if data.IsData() {
		return &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: data.Bytes, Base64: data.Base64}
	}
	if data.IsURL() {
		return &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: data.URL}
	}
	return nil
}

func validReasoningFile(data *provider.StreamFileData, mediaType string) bool {
	return data != nil && data.Validate() == nil && utf8.ValidString(mediaType) && utf8.ValidString(data.Base64) && utf8.ValidString(data.URL)
}

func (h *handler) processReasoningPart(w http.ResponseWriter, state *streamState, part provider.StreamPart) streamPartResult {
	event := streamEvent{typeName: part.Type, id: part.ID, delta: part.Delta, reasoningMetadata: part.ProviderMetadata, mediaType: part.MediaType, fileData: part.Data}
	switch part.Type {
	case provider.PartReasoningStart:
		if part.ID == "" {
			return streamPartAdapterFailure
		}
		if _, exists := state.usedReasoningIDs[part.ID]; exists {
			return streamPartAdapterFailure
		}
	case provider.PartReasoningDelta, provider.PartReasoningEnd:
		if _, active := state.reasoningIDs[part.ID]; !active {
			return streamPartAdapterFailure
		}
	}
	switch h.emitStreamEvent(w, event) {
	case streamWriteEncodingFailure:
		return streamPartAdapterFailure
	case streamWriteWriterFailure:
		return streamPartWriterFailure
	}
	state.textStarted = true
	if part.Type == provider.PartReasoningStart {
		state.reasoningIDs[part.ID] = struct{}{}
		state.usedReasoningIDs[part.ID] = struct{}{}
	}
	if part.Type == provider.PartReasoningEnd {
		delete(state.reasoningIDs, part.ID)
	}
	return streamPartContinue
}
