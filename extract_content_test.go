package aisdk

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/ai-sdk/provider"
)

func TestExtractTextContent(t *testing.T) {
	cases := []struct {
		name    string
		content []provider.ContentPart
		want    string
	}{
		{
			name:    "empty content returns empty string",
			content: nil,
			want:    "",
		},
		{
			name: "only non-text parts returns empty string",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeReasoning, Text: "thinking"},
				{Type: provider.ContentPartTypeFile, MediaType: "image/png"},
			},
			want: "",
		},
		{
			name: "single text part",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: "hello"},
			},
			want: "hello",
		},
		{
			name: "multiple text parts concatenate with empty join",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: "foo"},
				{Type: provider.ContentPartTypeText, Text: "bar"},
				{Type: provider.ContentPartTypeText, Text: "baz"},
			},
			want: "foobarbaz",
		},
		{
			name: "mixed parts preserves text order and skips non-text",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: "a "},
				{Type: provider.ContentPartTypeReasoning, Text: "ignored"},
				{Type: provider.ContentPartTypeText, Text: "b"},
			},
			want: "a b",
		},
		{
			name: "out-of-order parts preserve original order",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeReasoning, Text: "step 1"},
				{Type: provider.ContentPartTypeText, Text: "second"},
				{Type: provider.ContentPartTypeReasoning, Text: "step 2"},
				{Type: provider.ContentPartTypeText, Text: "first"},
			},
			want: "secondfirst",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractTextContent(tc.content)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExtractReasoningContent(t *testing.T) {
	cases := []struct {
		name    string
		content []provider.ContentPart
		want    string
	}{
		{
			name:    "empty content returns empty string",
			content: nil,
			want:    "",
		},
		{
			name: "only non-reasoning parts returns empty string",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: "hello"},
				{Type: provider.ContentPartTypeFile, MediaType: "image/png"},
			},
			want: "",
		},
		{
			name: "single reasoning part has no trailing newline",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeReasoning, Text: "thinking"},
			},
			want: "thinking",
		},
		{
			name: "multiple reasoning parts join with newline",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeReasoning, Text: "step 1"},
				{Type: provider.ContentPartTypeReasoning, Text: "step 2"},
				{Type: provider.ContentPartTypeReasoning, Text: "step 3"},
			},
			want: "step 1\nstep 2\nstep 3",
		},
		{
			name: "mixed parts skip non-reasoning",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeReasoning, Text: "a"},
				{Type: provider.ContentPartTypeText, Text: "ignored"},
				{Type: provider.ContentPartTypeReasoning, Text: "b"},
			},
			want: "a\nb",
		},
		{
			name: "out-of-order parts preserve original order",
			content: []provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: "ignored"},
				{Type: provider.ContentPartTypeReasoning, Text: "second"},
				{Type: provider.ContentPartTypeText, Text: "ignored"},
				{Type: provider.ContentPartTypeReasoning, Text: "first"},
			},
			want: "second\nfirst",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractReasoningContent(tc.content)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExtractTextContentGenerate(t *testing.T) {
	cases := []struct {
		name    string
		content []provider.GenerateContentPart
		want    string
	}{
		{
			name:    "empty content returns empty string",
			content: nil,
			want:    "",
		},
		{
			name: "only non-text parts returns empty string",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentReasoning, Text: "thinking"},
				{Type: provider.ContentToolCall, ToolName: "search"},
			},
			want: "",
		},
		{
			name: "multiple text parts concatenate with empty join",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: "foo"},
				{Type: provider.ContentText, Text: "bar"},
			},
			want: "foobar",
		},
		{
			name: "mixed parts preserve text order",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: "a "},
				{Type: provider.ContentReasoning, Text: "ignored"},
				{Type: provider.ContentText, Text: "b"},
			},
			want: "a b",
		},
		{
			name: "out-of-order parts preserve original order",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentReasoning, Text: "ignored"},
				{Type: provider.ContentText, Text: "second"},
				{Type: provider.ContentReasoning, Text: "ignored"},
				{Type: provider.ContentText, Text: "first"},
			},
			want: "secondfirst",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractTextContentGenerate(tc.content)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExtractReasoningContentGenerate(t *testing.T) {
	cases := []struct {
		name    string
		content []provider.GenerateContentPart
		want    string
	}{
		{
			name:    "empty content returns empty string",
			content: nil,
			want:    "",
		},
		{
			name: "only non-reasoning parts returns empty string",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: "hello"},
				{Type: provider.ContentToolCall, ToolName: "search"},
			},
			want: "",
		},
		{
			name: "multiple reasoning parts join with newline",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentReasoning, Text: "step 1"},
				{Type: provider.ContentReasoning, Text: "step 2"},
			},
			want: "step 1\nstep 2",
		},
		{
			name: "mixed parts skip non-reasoning",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentReasoning, Text: "a"},
				{Type: provider.ContentText, Text: "ignored"},
				{Type: provider.ContentReasoning, Text: "b"},
			},
			want: "a\nb",
		},
		{
			name: "out-of-order parts preserve original order",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: "ignored"},
				{Type: provider.ContentReasoning, Text: "second"},
				{Type: provider.ContentText, Text: "ignored"},
				{Type: provider.ContentReasoning, Text: "first"},
			},
			want: "second\nfirst",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractReasoningContentGenerate(tc.content)
			assert.Equal(t, tc.want, got)
		})
	}
}
