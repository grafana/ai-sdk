package v4

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

type wireFileDataType string

const (
	wireFileDataData      wireFileDataType = "data"
	wireFileDataURL       wireFileDataType = "url"
	wireFileDataReference wireFileDataType = "reference"
	wireFileDataText      wireFileDataType = "text"
)

func mapWireFileData(raw json.RawMessage) (provider.DataContent, *requestFailure) {
	var file struct {
		Type      wireFileDataType `json:"type"`
		Data      string           `json:"data"`
		URL       string           `json:"url"`
		Reference json.RawMessage  `json:"reference"`
		Text      string           `json:"text"`
	}
	if json.Unmarshal(raw, &file) != nil {
		return provider.DataContent{}, invalidMappingFailure()
	}
	var data provider.DataContent
	switch file.Type {
	case wireFileDataData:
		data = provider.Base64DataContent(file.Data)
	case wireFileDataURL:
		data = provider.URLDataContent(file.URL)
	case wireFileDataReference:
		data = provider.ReferenceDataContent(file.Reference)
	case wireFileDataText:
		data = provider.TextDataContent(file.Text)
	default:
		return provider.DataContent{}, invalidMappingFailure()
	}
	if data.Validate() != nil {
		return provider.DataContent{}, invalidMappingFailure()
	}
	return data, nil
}
