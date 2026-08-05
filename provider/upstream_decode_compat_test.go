package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests lock the decoder tolerance introduced by
// provider-wire-upstream-decode-compat: the decoders accept the upstream Vercel
// AI SDK LanguageModelV4 JSON shapes. The emitted form is now the upstream
// shape as well (see provider-wire-upstream-full-compat and
// TestUpstreamEmittedForm); these decode tests remain valid because decoding
// stays tolerant of both the upstream and legacy Go encodings.

func TestMessage_UnmarshalUpstreamSystemString(t *testing.T) {
	var m Message
	require.NoError(t, json.Unmarshal([]byte(`{"role":"system","content":"be helpful"}`), &m))
	assert.Equal(t, RoleSystem, m.Role)
	assert.Equal(t, []ContentPart{{Type: ContentPartTypeText, Text: "be helpful"}}, m.Content)
}

func TestMessage_UnmarshalCanonicalArrayStillWorks(t *testing.T) {
	var m Message
	require.NoError(t, json.Unmarshal([]byte(`{"role":"user","content":[{"type":"text","text":"hi"}]}`), &m))
	assert.Equal(t, RoleUser, m.Role)
	assert.Equal(t, []ContentPart{{Type: ContentPartTypeText, Text: "hi"}}, m.Content)
}

func TestToolResultOutput_UnmarshalUpstreamValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want ToolResultOutput
	}{
		{
			name: "text",
			in:   `{"type":"text","value":"ok"}`,
			want: ToolResultOutput{Type: ToolOutputText, Text: "ok"},
		},
		{
			name: "error-text",
			in:   `{"type":"error-text","value":"boom"}`,
			want: ToolResultOutput{Type: ToolOutputErrorText, Text: "boom"},
		},
		{
			name: "json",
			in:   `{"type":"json","value":{"a":1}}`,
			want: ToolResultOutput{Type: ToolOutputJSON, JSON: json.RawMessage(`{"a":1}`)},
		},
		{
			name: "content-text",
			in:   `{"type":"content","value":[{"type":"text","text":"hello"}]}`,
			want: ToolResultOutput{Type: ToolOutputContent, Content: []ToolResultContentValue{{Type: ToolContentText, Text: "hello"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out ToolResultOutput
			require.NoError(t, json.Unmarshal([]byte(tc.in), &out))
			assert.Equal(t, tc.want, out)
		})
	}
}

func TestToolResultOutput_UnmarshalCanonicalSplitStillWorks(t *testing.T) {
	var out ToolResultOutput
	require.NoError(t, json.Unmarshal([]byte(`{"type":"text","text":"ok"}`), &out))
	assert.Equal(t, ToolResultOutput{Type: ToolOutputText, Text: "ok"}, out)
}

func TestDataContent_UnmarshalUpstreamUnion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want DataContent
	}{
		{"url", `{"type":"url","url":"https://example.com/x.png"}`, DataContent{URL: "https://example.com/x.png"}},
		{"data-base64", `{"type":"data","data":"aGVsbG8="}`, DataContent{Base64: "aGVsbG8="}},
		{"empty data", `{"type":"data","data":""}`, DataContent{Bytes: []byte{}}},
		{"empty URL", `{"type":"url","url":""}`, DataContent{variant: dataContentVariantURL}},
		{"empty text", `{"type":"text","text":""}`, DataContent{variant: dataContentVariantText}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d DataContent
			require.NoError(t, json.Unmarshal([]byte(tc.in), &d))
			assert.Equal(t, tc.want, d)

			encoded, err := json.Marshal(d)
			require.NoError(t, err)
			assert.JSONEq(t, tc.in, string(encoded))
		})
	}
}

func TestDataContent_UnmarshalCanonicalStillWorks(t *testing.T) {
	var d DataContent
	require.NoError(t, json.Unmarshal([]byte(`{"url":"https://example.com/x.png"}`), &d))
	assert.Equal(t, DataContent{URL: "https://example.com/x.png"}, d)
}

func TestStreamPart_FileData_RoundTrip(t *testing.T) {
	dataCases := []struct {
		name string
		json string
		want StreamFileData
	}{
		{"data", `{"type":"data","data":"AQID"}`, StreamFileData{Type: StreamFileDataTypeData, Base64: "AQID"}},
		{"empty data", `{"type":"data","data":""}`, StreamFileData{Type: StreamFileDataTypeData}},
		{"URL", `{"type":"url","url":"https://example.com/image.png"}`, StreamFileData{Type: StreamFileDataTypeURL, URL: "https://example.com/image.png"}},
	}
	for _, partType := range []StreamPartType{PartFile, PartReasoningFile} {
		for _, tc := range dataCases {
			t.Run(string(partType)+"/"+tc.name, func(t *testing.T) {
				input := `{"type":"` + string(partType) + `","mediaType":"image/png","data":` + tc.json + `}`

				var part StreamPart
				require.NoError(t, json.Unmarshal([]byte(input), &part))
				require.NotNil(t, part.Data)
				assert.Equal(t, tc.want, *part.Data)

				encoded, err := json.Marshal(part)
				require.NoError(t, err)
				assert.JSONEq(t, input, string(encoded))
			})
		}
	}
}

func TestStreamFileData_UnmarshalRejectsMissingData(t *testing.T) {
	var fileData StreamFileData
	err := json.Unmarshal([]byte(`{"type":"data"}`), &fileData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required data")

	var part StreamPart
	require.NoError(t, json.Unmarshal([]byte(`{"type":"file","mediaType":"image/png","data":{"type":"data"}}`), &part))
	assert.Nil(t, part.Data)
}

func TestStreamFileData_UnmarshalRejectsUnsupportedVariants(t *testing.T) {
	for _, input := range []string{
		`{"type":"reference","reference":{"openai":"file_123"}}`,
		`{"type":"text","text":"inline document"}`,
	} {
		var data StreamFileData
		err := json.Unmarshal([]byte(input), &data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported stream file-data variant")

		var part StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"file","mediaType":"image/png","data":`+input+`}`), &part))
		assert.Nil(t, part.Data)
	}
}

func TestToolResultContentValue_UnmarshalLegacyFileVariants(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    ToolResultContentValue
		encoded string
	}{
		{
			name:    "raw base64",
			in:      `{"type":"file-data","data":"aGk=","mediaType":"image/png"}`,
			want:    ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{Base64: "aGk="}, MediaType: "image/png"},
			encoded: `{"type":"file","data":{"type":"data","data":"aGk="},"mediaType":"image/png"}`,
		},
		{
			name:    "empty raw base64",
			in:      `{"type":"file-data","data":"","mediaType":"image/png"}`,
			want:    ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{Bytes: []byte{}}, MediaType: "image/png"},
			encoded: `{"type":"file","data":{"type":"data","data":""},"mediaType":"image/png"}`,
		},
		{
			name:    "URL",
			in:      `{"type":"file-url","url":"https://example.com/file.pdf","mediaType":"application/pdf"}`,
			want:    ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{URL: "https://example.com/file.pdf"}, MediaType: "application/pdf"},
			encoded: `{"type":"file","data":{"type":"url","url":"https://example.com/file.pdf"},"mediaType":"application/pdf"}`,
		},
		{
			name:    "provider reference",
			in:      `{"type":"file-reference","providerReference":{"openai":"file-123"},"mediaType":"application/pdf"}`,
			want:    ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{Reference: json.RawMessage(`{"openai":"file-123"}`)}, MediaType: "application/pdf"},
			encoded: `{"type":"file","data":{"type":"reference","reference":{"openai":"file-123"}},"mediaType":"application/pdf"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var value ToolResultContentValue
			require.NoError(t, json.Unmarshal([]byte(tc.in), &value))
			assert.Equal(t, tc.want, value)

			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, tc.encoded, string(encoded))
		})
	}
}

// TestDataContent_UnmarshalUnknownVariantFailsClosed locks the fail-closed
// policy: an unknown tagged file-data variant is rejected with a decode error,
// not silently turned into an empty DataContent. Supported variants
// (data/url/reference/text) are covered by the decode tests above and in
// upstream_encode_compat_test.go.
func TestDataContent_UnmarshalUnknownVariantFailsClosed(t *testing.T) {
	var d DataContent
	err := json.Unmarshal([]byte(`{"type":"totally-unknown","data":"x"}`), &d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file-data variant")
}

func TestDataContent_UnmarshalMalformedTaggedVariantFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"data has wrong type", `{"type":"data","data":{"oops":true}}`},
		{"data is missing", `{"type":"data"}`},
		{"URL has wrong type", `{"type":"url","url":42}`},
		{"URL is null", `{"type":"url","url":null}`},
		{"reference has wrong type", `{"type":"reference","reference":"file-1"}`},
		{"reference value has wrong type", `{"type":"reference","reference":{"openai":42}}`},
		{"text has wrong type", `{"type":"text","text":[]}`},
		{"text is missing", `{"type":"text"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d DataContent
			require.Error(t, json.Unmarshal([]byte(tc.in), &d))
		})
	}
}

// TestToolResultOutput_UnmarshalMalformedValueFailsClosed locks the fail-closed
// policy: an upstream `value` that cannot be mapped onto the canonical field
// for its type returns a decode error instead of silently dropping the payload.
func TestToolResultOutput_UnmarshalMalformedValueFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"text value is missing", `{"type":"text"}`},
		{"text value is not a string", `{"type":"text","value":123}`},
		{"text value is null", `{"type":"text","value":null}`},
		{"legacy text is missing", `{"type":"text","providerOptions":{}}`},
		{"json value is missing", `{"type":"json"}`},
		{"content value is missing", `{"type":"content"}`},
		{"unknown output type", `{"type":"future","value":"x"}`},
		{"execution denied has value", `{"type":"execution-denied","value":"no"}`},
		{"content value is not an array", `{"type":"content","value":"nope"}`},
		{"content value is null", `{"type":"content","value":null}`},
		{"content item is null", `{"type":"content","value":[null]}`},
		{"content item type is missing", `{"type":"content","value":[{}]}`},
		{"content item type is unknown", `{"type":"content","value":[{"type":"future"}]}`},
		{
			name: "content file has unknown data variant",
			in:   `{"type":"content","value":[{"type":"file","data":{"type":"future","data":"aGk="},"mediaType":"image/png"}]}`,
		},
		{
			name: "content file has malformed data variant",
			in:   `{"type":"content","value":[{"type":"file","data":{"type":"data","data":{"oops":true}},"mediaType":"image/png"}]}`,
		},
		{
			name: "content file data is missing",
			in:   `{"type":"content","value":[{"type":"file","mediaType":"image/png"}]}`,
		},
		{
			name: "content file data type is missing",
			in:   `{"type":"content","value":[{"type":"file","data":{},"mediaType":"image/png"}]}`,
		},
		{
			name: "content file mediaType is missing",
			in:   `{"type":"content","value":[{"type":"file","data":{"type":"data","data":"aGk="}}]}`,
		},
		{
			name: "content text value is missing text",
			in:   `{"type":"content","value":[{"type":"text"}]}`,
		},
		{
			name: "legacy content file data is not a string",
			in:   `{"type":"content","value":[{"type":"file-data","data":{"type":"data","data":"aGk="},"mediaType":"image/png"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out ToolResultOutput
			err := json.Unmarshal([]byte(tc.in), &out)
			require.Error(t, err)
		})
	}
}

// TestMessage_UnmarshalNonSystemStringContentFailsClosed locks that string
// content is accepted only for the system role.
func TestMessage_UnmarshalNonSystemStringContentFailsClosed(t *testing.T) {
	for _, role := range []string{"user", "assistant", "tool"} {
		t.Run(role, func(t *testing.T) {
			var m Message
			err := json.Unmarshal([]byte(`{"role":"`+role+`","content":"stringy"}`), &m)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "only valid for the system role")
		})
	}
}

// TestUpstreamEmittedForm asserts the encoders now emit the upstream Vercel AI
// SDK LanguageModelV4 JSON (system content as a string, tool-result single
// `value` union, DataContent tagged union). This supersedes decisions D6
// (system as array) and D4 (see stream-part error tests) from
// 2026-04-30-lossless-provider-wire.
func TestUpstreamEmittedForm(t *testing.T) {
	system := NewSystemMessage("be helpful")
	sysJSON, err := json.Marshal(system)
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"system","content":"be helpful"}`, string(sysJSON))

	out := ToolResultOutput{Type: ToolOutputText, Text: "ok"}
	outJSON, err := json.Marshal(out)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"text","value":"ok"}`, string(outJSON))

	data := DataContent{URL: "https://example.com/x.png"}
	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"url","url":"https://example.com/x.png"}`, string(dataJSON))
}
