package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentPart_AllTypes_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		part ContentPart
		// want, when set, is the expected decoded value after a marshal/
		// unmarshal round trip. When nil, part is expected to round-trip
		// unchanged. It is set for cases where the upstream tagged file-data
		// union normalizes raw bytes to their base64 encoding.
		want *ContentPart
	}{
		{
			name: "text",
			part: ContentPart{Type: ContentPartTypeText, Text: "hello"},
		},
		{
			name: "file with URL",
			part: ContentPart{
				Type:      ContentPartTypeFile,
				Data:      &DataContent{URL: "https://example.com/image.png"},
				MediaType: "image/png",
				Filename:  new("image.png"),
			},
		},
		{
			name: "file with bytes",
			part: ContentPart{
				Type:      ContentPartTypeFile,
				Data:      &DataContent{Bytes: []byte{0x01, 0x02, 0x03}},
				MediaType: "application/octet-stream",
			},
			// Raw bytes encode as the upstream {type:"data",data:<base64>}
			// union and decode back into Base64.
			want: &ContentPart{
				Type:      ContentPartTypeFile,
				Data:      &DataContent{Base64: "AQID"},
				MediaType: "application/octet-stream",
			},
		},
		{
			name: "file with provider reference",
			part: ContentPart{
				Type:      ContentPartTypeFile,
				Data:      &DataContent{Reference: json.RawMessage(`{"openai":"file-abc123"}`)},
				MediaType: "application/pdf",
			},
		},
		{
			name: "file with empty provider reference",
			part: ContentPart{
				Type:      ContentPartTypeFile,
				Data:      &DataContent{Reference: json.RawMessage(`{}`)},
				MediaType: "application/pdf",
			},
		},
		{
			name: "reasoning",
			part: ContentPart{Type: ContentPartTypeReasoning, Text: "thinking..."},
		},
		{
			name: "reasoning-file",
			part: ContentPart{
				Type:      ContentPartTypeReasoningFile,
				Data:      &DataContent{Base64: "AAEC"},
				MediaType: "image/png",
			},
		},
		{
			name: "source url",
			part: ContentPart{
				Type:       ContentPartTypeSource,
				SourceType: SourceTypeURL,
				ID:         "src_1",
				URL:        "https://example.com",
				Title:      "Example",
			},
		},
		{
			name: "source document",
			part: ContentPart{
				Type:       ContentPartTypeSource,
				SourceType: SourceTypeDocument,
				ID:         "src_2",
				MediaType:  "application/pdf",
				Title:      "Report",
				Filename:   new("report.pdf"),
			},
		},
		{
			name: "tool-call",
			part: ContentPart{
				Type:       ContentPartTypeToolCall,
				ToolCallID: "tc_1",
				ToolName:   "search",
				Input:      json.RawMessage(`{"q":"go"}`),
			},
		},
		{
			name: "tool-call provider-executed",
			part: ContentPart{
				Type:             ContentPartTypeToolCall,
				ToolCallID:       "tc_2",
				ToolName:         "web_search",
				Input:            json.RawMessage(`{"q":"x"}`),
				ProviderExecuted: true,
			},
		},
		{
			name: "tool-result",
			part: ContentPart{
				Type:       ContentPartTypeToolResult,
				ToolCallID: "tc_1",
				ToolName:   "search",
				Output:     &ToolResultOutput{Type: ToolOutputText, Text: "ok"},
			},
		},
		{
			name: "custom",
			part: ContentPart{
				Type: ContentPartTypeCustom,
				Kind: "anthropic.cache-control",
			},
		},
		{
			name: "tool-approval-request",
			part: ContentPart{
				Type:       ContentPartTypeToolApprovalRequest,
				ApprovalID: "apr_1",
				ToolCallID: "tc_1",
			},
		},
		{
			name: "tool-approval-response approved",
			part: ContentPart{
				Type:       ContentPartTypeToolApprovalResponse,
				ApprovalID: "apr_1",
				Approved:   boolPtr(true),
			},
		},
		{
			name: "tool-approval-response denied",
			part: ContentPart{
				Type:       ContentPartTypeToolApprovalResponse,
				ApprovalID: "apr_2",
				Approved:   boolPtr(false),
				Reason:     "unsafe",
			},
		},
		{
			name: "with provider options",
			part: ContentPart{
				Type: ContentPartTypeText,
				Text: "x",
				ProviderOptions: ProviderOptions{
					"anthropic": RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cache":"ephemeral"}`)},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.part)
			require.NoError(t, err)

			var decoded ContentPart
			require.NoError(t, json.Unmarshal(data, &decoded))
			want := tc.part
			if tc.want != nil {
				want = *tc.want
			}
			assert.Equal(t, want, decoded)
		})
	}
}

func TestSourcePartFilenameNormalization(t *testing.T) {
	for _, tc := range []struct {
		name     string
		filename string
		want     *string
	}{
		{name: "absent"},
		{name: "named", filename: "report.pdf", want: new("report.pdf")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := SourcePart(SourceInfo{SourceType: SourceTypeDocument, Filename: tc.filename})
			assert.Equal(t, tc.want, part.Filename)
		})
	}
}

func TestFileContentPartFilenamePresence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		filename *string
		present  bool
	}{
		{name: "absent"},
		{name: "empty", filename: new(""), present: true},
		{name: "named", filename: new("report.pdf"), present: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := FilePart("application/pdf", Base64DataContent("AQID"))
			part.Filename = tc.filename
			encoded, err := json.Marshal(part)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &fields))
			_, present := fields["filename"]
			assert.Equal(t, tc.present, present)
			var decoded ContentPart
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.Equal(t, tc.filename, decoded.Filename)
		})
	}
}

func TestDataContentValidate_ProviderReference(t *testing.T) {
	assert.NoError(t, (DataContent{Reference: json.RawMessage(`{"openai":"file-abc123"}`)}).Validate())
	assert.NoError(t, (DataContent{Reference: json.RawMessage(`{}`)}).Validate())
	assert.Error(t, (DataContent{URL: "https://example.com/doc.pdf", Reference: json.RawMessage(`{}`)}).Validate())
}

func TestDataContentValidate_EmptyVariantConflict(t *testing.T) {
	data := Base64DataContent("")
	assert.NoError(t, data.Validate())

	data.URL = "https://example.com/file"
	assert.Error(t, data.Validate())
}

func TestDataContentConstructedArms(t *testing.T) {
	for _, tc := range []struct {
		name string
		data DataContent
		wire string
	}{
		{name: "empty bytes", data: BytesDataContent([]byte{}), wire: `{"type":"data","data":""}`},
		{name: "empty base64", data: Base64DataContent(""), wire: `{"type":"data","data":""}`},
		{name: "bytes", data: BytesDataContent([]byte{1, 2, 3}), wire: `{"type":"data","data":"AQID"}`},
		{name: "empty URL", data: URLDataContent(""), wire: `{"type":"url","url":""}`},
		{name: "empty reference", data: ReferenceDataContent(json.RawMessage(`{}`)), wire: `{"type":"reference","reference":{}}`},
		{name: "empty text", data: TextDataContent(""), wire: `{"type":"text","text":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.data.Validate())
			encoded, err := json.Marshal(tc.data)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wire, string(encoded))
			var decoded DataContent
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.Equal(t, tc.data.IsData(), decoded.IsData())
			assert.Equal(t, tc.data.IsURL(), decoded.IsURL())
			assert.Equal(t, tc.data.IsReference(), decoded.IsReference())
			assert.Equal(t, tc.data.IsText(), decoded.IsText())
		})
	}
}

func TestDataContentTaggedDecodingRejectsInactiveArms(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire string
	}{
		{name: "empty text with empty URL", wire: `{"type":"text","text":"","url":""}`},
		{name: "empty text with null URL", wire: `{"type":"text","text":"","url":null}`},
		{name: "empty data with legacy bytes", wire: `{"type":"data","data":"","bytes":"AQI="}`},
		{name: "URL with legacy base64", wire: `{"type":"url","url":"https://example.test/file","base64":"AQI="}`},
		{name: "reference with text", wire: `{"type":"reference","reference":{},"text":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value DataContent
			require.Error(t, json.Unmarshal([]byte(tc.wire), &value))
		})
	}
}

func TestDataContentLegacyDecodingRejectsMultipleArms(t *testing.T) {
	for _, wire := range []string{
		`{"url":"","text":""}`,
		`{"bytes":"","base64":""}`,
	} {
		t.Run(wire, func(t *testing.T) {
			var value DataContent
			require.Error(t, json.Unmarshal([]byte(wire), &value))
		})
	}
}

func TestDataContentInvalidReferences(t *testing.T) {
	for _, wire := range []string{`null`, `[]`, `{"openai":1}`, `{"type":"file"}`, `{"openai":null}`} {
		t.Run(wire, func(t *testing.T) {
			value := DataContent{Reference: json.RawMessage(wire)}
			require.Error(t, value.Validate())
			_, err := json.Marshal(value)
			require.Error(t, err)
		})
	}
}

func TestDataContentSelectedEmptyText(t *testing.T) {
	var value DataContent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"text","text":""}`), &value))
	require.NoError(t, value.Validate())
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"text","text":""}`, string(encoded))
	value.URL = "https://example.test/file"
	require.Error(t, value.Validate())
	_, err = json.Marshal(value)
	require.Error(t, err)
}

func TestContentPartType_AllConstantsCovered(t *testing.T) {
	defined := []ContentPartType{
		ContentPartTypeText,
		ContentPartTypeFile,
		ContentPartTypeReasoning,
		ContentPartTypeReasoningFile,
		ContentPartTypeSource,
		ContentPartTypeToolCall,
		ContentPartTypeToolResult,
		ContentPartTypeCustom,
		ContentPartTypeToolApprovalRequest,
		ContentPartTypeToolApprovalResponse,
	}
	assert.Len(t, defined, 10)

	unique := make(map[ContentPartType]bool)
	for _, ct := range defined {
		assert.False(t, unique[ct], "duplicate ContentPartType: %q", ct)
		unique[ct] = true
	}
}
