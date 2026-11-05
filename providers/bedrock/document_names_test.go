package bedrock

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_UniqueDocumentNames(t *testing.T) {
	for _, tc := range []struct {
		name      string
		filenames []string
		want      []string
	}{
		{
			name:      "upstream duplicate document names",
			filenames: []string{"invoice.pdf", "invoice.pdf", "rates.v1.pdf", "rates.v2.pdf", "/workspace/.eve/attachments/ab12/a.pdf", "invoice (2).pdf", "document-1.pdf", "", strings.Repeat("a", 201) + ".pdf", strings.Repeat("a", 201) + ".pdf"},
			want:      []string{"invoice", "invoice (2)", "rates", "rates (2)", "workspace", "invoice (2) (2)", "document-1", "document-1 (2)", strings.Repeat("a", 200), strings.Repeat("a", 196) + " (2)"},
		},
		{
			name:      "skip literal suffix and sanitized collisions",
			filenames: []string{"a&b.pdf", "ab (2).pdf", "ab.pdf", "ab.pdf"},
			want:      []string{"ab", "ab (2)", "ab (3)", "ab (4)"},
		},
		{
			name:      "generated and explicit names in both orders",
			filenames: []string{"", "document-1.pdf", "document-2.pdf", ".pdf", ""},
			want:      []string{"document-1", "document-1 (2)", "document-2", "document-2 (2)", "document-3"},
		},
		{
			name:      "trim before adding suffix",
			filenames: []string{strings.Repeat("a", 195) + " bbbb.pdf", strings.Repeat("a", 195) + " bbbb.pdf"},
			want:      []string{strings.Repeat("a", 195) + " bbbb", strings.Repeat("a", 195) + " (2)"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var parts []provider.ContentPart
			for _, filename := range tc.filenames {
				part := provider.FilePart("application/pdf", provider.DataContent{Base64: "AAECAw=="})
				if filename != "" {
					part.Filename = new(filename)
				}
				parts = append(parts, part)
			}
			for range 2 {
				req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(parts...)}})
				require.Empty(t, warnings)
				require.Len(t, req.Messages, 1)
				var names []string
				for _, block := range req.Messages[0].Content {
					require.NotNil(t, block.Document)
					names = append(names, block.Document.Name)
					assert.LessOrEqual(t, len(block.Document.Name), 200)
				}
				assert.Equal(t, tc.want, names)
			}
		})
	}
}

func TestBuildRequest_UniqueDocumentNamesAcrossMessages(t *testing.T) {
	for _, role := range []provider.Role{provider.RoleTool, provider.RoleAssistant, provider.RoleUser} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			file := provider.FilePart("text/plain", provider.DataContent{Text: "hello"})
			file.Filename = new("invoice.txt")
			result := provider.ToolResultPart("call-123", "read-file", &provider.ToolResultOutput{
				Type: provider.ToolOutputContent,
				Content: []provider.ToolResultContentValue{{
					Type: provider.ToolContentFile, MediaType: "application/pdf", Filename: new("invoice.pdf"),
					Data: &provider.DataContent{Base64: "AAECAw=="},
				}},
			})
			prompt := []provider.Message{
				provider.NewUserMessage(file),
				provider.NewAssistantMessage(provider.ToolCallPart("call-123", "read-file", json.RawMessage(`{}`))),
				{Role: role, Content: []provider.ContentPart{result}},
				provider.NewUserMessage(file),
			}
			for range 2 {
				req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
					Prompt: prompt,
					Tools:  []provider.Tool{{Type: provider.ToolTypeFunction, Name: "read-file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
				})
				require.Empty(t, warnings)
				var names []string
				for _, message := range req.Messages {
					for _, block := range message.Content {
						if block.Document != nil {
							names = append(names, block.Document.Name)
						}
						if block.ToolResult != nil {
							for _, content := range block.ToolResult.Content {
								if content.Document != nil {
									names = append(names, content.Document.Name)
								}
							}
						}
					}
				}
				assert.Equal(t, []string{"invoice", "invoice (2)", "invoice (3)"}, names)
			}
		})
	}
}

func TestBuildRequest_DocumentSuffixWidth(t *testing.T) {
	var parts []provider.ContentPart
	for range 11 {
		part := provider.FilePart("application/pdf", provider.DataContent{Base64: "AAECAw=="})
		part.Filename = new(strings.Repeat("a", 200) + ".pdf")
		parts = append(parts, part)
	}
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(parts...)}})
	for i, block := range req.Messages[0].Content {
		want := strings.Repeat("a", 200)
		if i > 0 {
			suffix := fmt.Sprintf(" (%d)", i+1)
			want = strings.Repeat("a", 200-len(suffix)) + suffix
		}
		assert.Equal(t, want, block.Document.Name)
	}
}
