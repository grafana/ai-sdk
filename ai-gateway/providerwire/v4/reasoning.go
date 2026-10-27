package v4

import (
	"encoding/base64"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type reasoningTextPart struct {
	Type     provider.GenerateContentType `json:"type"`
	Text     string                       `json:"text"`
	Metadata provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
}

type reasoningFilePart struct {
	Type      string                    `json:"type"`
	MediaType string                    `json:"mediaType"`
	Data      reasoningWireFileData     `json:"data"`
	Metadata  provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

type reasoningWireFileData struct {
	Type        string  `json:"type"`
	Data        *string `json:"data,omitempty"`
	URL         *string `json:"url,omitempty"`
	OriginalURL *string `json:"originalUrl,omitempty"`
}

// Called only after validation. Do not expose provider
// marshalers at the strict service boundary.
func projectReasoningFile(data *provider.StreamFileData) reasoningWireFileData {
	if data.Type == provider.StreamFileDataTypeURL || (data.Type == "" && data.URL != "") {
		return reasoningWireFileData{Type: "url", URL: &data.URL, OriginalURL: data.OriginalURL}
	}
	encoded := data.Base64
	if data.Bytes != nil {
		encoded = base64.StdEncoding.EncodeToString(data.Bytes)
	}
	return reasoningWireFileData{Type: "data", Data: &encoded}
}

func unaryReasoningFile(data *provider.DataContent) *provider.StreamFileData {
	if data == nil || data.Validate() != nil {
		return nil
	}
	if data.IsData() {
		return &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: data.Bytes, Base64: data.Base64}
	}
	if data.IsURL() {
		return &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: data.URL, OriginalURL: data.OriginalURL}
	}
	return nil
}

func validReasoningFile(data *provider.StreamFileData, mediaType string) bool {
	return data != nil && data.Validate() == nil && utf8.ValidString(mediaType) && utf8.ValidString(data.Base64) && utf8.ValidString(data.URL) && (data.OriginalURL == nil || utf8.ValidString(*data.OriginalURL))
}

func (h *handler) processReasoningPart(w http.ResponseWriter, state *streamState, part provider.StreamPart) streamPartResult {
	event := streamEvent{typeName: part.Type, id: part.ID, delta: part.Delta, metadata: part.ProviderMetadata, mediaType: part.MediaType, fileData: part.Data}
	switch part.Type {
	case provider.PartReasoningStart:
		if part.ID == "" {
			return streamPartAdapterFailure
		}
		if _, exists := state.segmentReasoningIDs[part.ID]; exists {
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
	if part.Type == provider.PartReasoningStart {
		state.reasoningIDs[part.ID] = struct{}{}
		state.segmentReasoningIDs[part.ID] = struct{}{}
	}
	if part.Type == provider.PartReasoningEnd {
		delete(state.reasoningIDs, part.ID)
	}
	return streamPartContinue
}
