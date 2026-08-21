package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataContent_SelectionAndValidation(t *testing.T) {
	tests := []struct {
		name     string
		data     DataContent
		wantType DataContentType
		wantJSON string
	}{
		{name: "empty bytes", data: BytesDataContent(nil), wantType: DataContentTypeData, wantJSON: `{"type":"data","data":""}`},
		{name: "bytes", data: BytesDataContent([]byte("value")), wantType: DataContentTypeData, wantJSON: `{"type":"data","data":"dmFsdWU="}`},
		{name: "empty base64", data: Base64DataContent(""), wantType: DataContentTypeData, wantJSON: `{"type":"data","data":""}`},
		{name: "base64", data: Base64DataContent("dmFsdWU="), wantType: DataContentTypeData, wantJSON: `{"type":"data","data":"dmFsdWU="}`},
		{name: "empty URL", data: URLDataContent(""), wantType: DataContentTypeURL, wantJSON: `{"type":"url","url":""}`},
		{name: "URL", data: URLDataContent("https://example.com"), wantType: DataContentTypeURL, wantJSON: `{"type":"url","url":"https://example.com"}`},
		{name: "empty reference", data: ReferenceDataContent(json.RawMessage(`{}`)), wantType: DataContentTypeReference, wantJSON: `{"type":"reference","reference":{}}`},
		{name: "reference", data: ReferenceDataContent(json.RawMessage(`{"openai":"file-1"}`)), wantType: DataContentTypeReference, wantJSON: `{"type":"reference","reference":{"openai":"file-1"}}`},
		{name: "empty text", data: TextDataContent(""), wantType: DataContentTypeText, wantJSON: `{"type":"text","text":""}`},
		{name: "text", data: TextDataContent("value"), wantType: DataContentTypeText, wantJSON: `{"type":"text","text":"value"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataType, ok := tc.data.DataType()
			require.True(t, ok)
			assert.Equal(t, tc.wantType, dataType)
			require.NoError(t, tc.data.Validate())
			encoded, err := json.Marshal(tc.data)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wantJSON, string(encoded))
			var decoded DataContent
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			decodedType, ok := decoded.DataType()
			require.True(t, ok)
			assert.Equal(t, tc.wantType, decodedType)
		})
	}
}

func TestDataContent_ConstructorsCopyInputs(t *testing.T) {
	bytesInput := []byte("value")
	bytesData := BytesDataContent(bytesInput)
	bytesInput[0] = 'X'
	assert.Equal(t, []byte("value"), bytesData.Bytes)

	referenceInput := json.RawMessage(`{"openai":"file-1"}`)
	referenceData := ReferenceDataContent(referenceInput)
	referenceInput[2] = 'X'
	assert.JSONEq(t, `{"openai":"file-1"}`, string(referenceData.Reference))
}

func TestDataContent_DataTypeConflicts(t *testing.T) {
	tests := []struct {
		name     string
		data     DataContent
		wantType DataContentType
		wantOK   bool
	}{
		{name: "zero value"},
		{name: "inferred conflict", data: DataContent{URL: "https://example.com", Text: "value"}, wantType: DataContentTypeURL},
		{name: "selected conflict", data: DataContent{variant: DataContentTypeText, URL: "https://example.com"}, wantType: DataContentTypeText},
		{name: "same data arm", data: DataContent{Bytes: []byte{}, Base64: "dmFsdWU="}, wantType: DataContentTypeData, wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataType, ok := tc.data.DataType()
			assert.Equal(t, tc.wantType, dataType)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestDataContent_InvalidStates(t *testing.T) {
	invalid := []DataContent{
		{},
		{Bytes: []byte{}, Base64: "dmFsdWU="},
		{URL: "https://example.com", Text: "value"},
		{variant: DataContentTypeText, URL: "https://example.com"},
		ReferenceDataContent(nil),
		ReferenceDataContent(json.RawMessage(`null`)),
		ReferenceDataContent(json.RawMessage(`[]`)),
		ReferenceDataContent(json.RawMessage(`{"count":1}`)),
	}
	for _, data := range invalid {
		require.Error(t, data.Validate())
		_, err := json.Marshal(data)
		require.Error(t, err)
	}
}

func TestDataContent_JSONRejectsInactivePayloads(t *testing.T) {
	invalid := []string{
		`{"type":"text","text":"","url":"https://example.com"}`,
		`{"type":"data","data":"","reference":{}}`,
		`{"type":"url","url":"","data":null}`,
		`{"type":"reference","reference":{},"text":""}`,
		`{"type":"data","data":"","bytes":""}`,
	}
	for _, input := range invalid {
		var data DataContent
		require.Error(t, json.Unmarshal([]byte(input), &data))
	}
}

func TestContentPart_FilenameCompatibilityJSON(t *testing.T) {
	data := TextDataContent("value")
	tests := []struct {
		name         string
		part         ContentPart
		wantJSON     string
		wantRequest  *string
		wantResponse string
	}{
		{name: "request absent", part: FilePart("text/plain", data), wantJSON: `{"type":"file","data":{"type":"text","text":"value"},"mediaType":"text/plain"}`},
		{name: "request empty", part: FilePartWithFilename("text/plain", data, ""), wantJSON: `{"type":"file","data":{"type":"text","text":"value"},"filename":"","mediaType":"text/plain"}`, wantRequest: stringPtr("")},
		{name: "request non-empty", part: FilePartWithFilename("text/plain", data, "report.txt"), wantJSON: `{"type":"file","data":{"type":"text","text":"value"},"filename":"report.txt","mediaType":"text/plain"}`, wantRequest: stringPtr("report.txt")},
		{name: "generated normalizes", part: ContentPart{Type: ContentPartTypeFile, Data: &data, MediaType: "text/plain", Filename: "generated.txt"}, wantJSON: `{"type":"file","data":{"type":"text","text":"value"},"filename":"generated.txt","mediaType":"text/plain"}`, wantRequest: stringPtr("generated.txt")},
		{name: "source remains response-owned", part: ContentPart{Type: ContentPartTypeSource, SourceType: SourceTypeDocument, Filename: "source.txt"}, wantJSON: `{"type":"source","filename":"source.txt","sourceType":"document"}`, wantResponse: "source.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.part)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wantJSON, string(encoded))
			var decoded ContentPart
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.Equal(t, tc.wantRequest, decoded.FilePartFilename)
			assert.Equal(t, tc.wantResponse, decoded.Filename)
		})
	}
}

func TestContentPart_FilenameCompatibilityRejectsMixedOwnership(t *testing.T) {
	data := TextDataContent("value")
	_, err := json.Marshal(ContentPart{
		Type: ContentPartTypeFile, Data: &data, MediaType: "text/plain",
		FilePartFilename: stringPtr("request.txt"), Filename: "response.txt",
	})
	require.Error(t, err)

	_, err = json.Marshal(ContentPart{Type: ContentPartTypeSource, FilePartFilename: stringPtr("request.txt")})
	require.Error(t, err)
}

func TestOptionalToolResultScalars_JSONCompatibility(t *testing.T) {
	for _, value := range []*string{nil, stringPtr(""), stringPtr("value")} {
		result := ToolResultOutput{Type: ToolOutputExecutionDenied, Reason: value}
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		var decoded ToolResultOutput
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.Equal(t, value, decoded.Reason)

		content := ToolResultContentValue{Type: ToolContentFile, Data: dataContentPtr(TextDataContent("value")), MediaType: "text/plain", Filename: value}
		encoded, err = json.Marshal(content)
		require.NoError(t, err)
		var decodedContent ToolResultContentValue
		require.NoError(t, json.Unmarshal(encoded, &decodedContent))
		assert.Equal(t, value, decodedContent.Filename)
	}
}

func dataContentPtr(value DataContent) *DataContent { return &value }
