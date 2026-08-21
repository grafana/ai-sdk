package providerwire

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const parentCompatibilityCommit = "32e5ab7f1ab9e524477cc0ece04c690a89854a24"

type parentCompatibilityCorpus struct {
	SchemaVersion int                      `json:"schemaVersion"`
	ParentCommit  string                   `json:"parentCommit"`
	Rows          []parentCompatibilityRow `json:"rows"`
}

type parentCompatibilityRow struct {
	ID             string `json:"id"`
	Partition      string `json:"partition"`
	CanonicalBytes string `json:"canonicalBytesBase64"`
	ParentDecode   struct {
		Status             string `json:"status"`
		SemanticProjection string `json:"semanticProjectionBase64"`
		ExactError         string `json:"exactError"`
	} `json:"parentDecode"`
}

func TestParentRequestCompatibilityCorpus(t *testing.T) {
	expectedPartitions := map[string]string{
		"empty":                            "top-level/zero",
		"exact-large-numbers":              "top-level/numeric-boundaries",
		"settings-and-opaque-options":      "top-level/scalars-and-collections",
		"parent-empty-collection-collapse": "top-level/non-nil-empty-collections",
		"system-content-projection":        "messages/system-concatenation",
		"mixed-content-inactive-fields":    "content/mixed-inactive-fields",
		"mixed-function-provider-tool":     "tools/mixed-inactive-fields",
		"data-precedence":                  "data/mixed-arm-parent-precedence",
		"reference-string-object":          "data/reference/parent-decodable",
		"reference-non-string-json":        "data/reference/encoder-only",
		"tool-result-arms":                 "tool-results/active-and-inactive-fields",
		"empty-raw-provider-options":       "provider-options/empty-raw-to-null",
		"source-and-generated-filename":    "content/response-source-filename",
	}
	data, err := os.ReadFile("testdata/parent_request_compat_v1.json")
	require.NoError(t, err)
	var corpus parentCompatibilityCorpus
	require.NoError(t, json.Unmarshal(data, &corpus))
	assert.Equal(t, 1, corpus.SchemaVersion)
	assert.Equal(t, parentCompatibilityCommit, corpus.ParentCommit)
	require.Len(t, corpus.Rows, len(expectedPartitions))

	seen := map[string]bool{}
	for _, row := range corpus.Rows {
		t.Run(row.ID, func(t *testing.T) {
			require.False(t, seen[row.ID])
			seen[row.ID] = true
			partition, ok := expectedPartitions[row.ID]
			require.True(t, ok, "unexpected corpus row")
			assert.Equal(t, partition, row.Partition)
			want, err := base64.StdEncoding.DecodeString(row.CanonicalBytes)
			require.NoError(t, err)
			options := migratedParentCorpusOptions(t, row.ID)
			got, err := EncodeCallOptions(options)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))

			decoded, err := DecodeCallOptions(want)
			require.NoError(t, err)
			if row.ParentDecode.Status == "success" {
				projection, err := base64.StdEncoding.DecodeString(row.ParentDecode.SemanticProjection)
				require.NoError(t, err)
				currentProjection, err := EncodeCallOptions(decoded)
				require.NoError(t, err)
				assert.Equal(t, string(projection), string(currentProjection))
			} else {
				assert.Equal(t, "rejected", row.ParentDecode.Status)
				assert.NotEmpty(t, row.ParentDecode.ExactError)
			}
		})
	}
}

func migratedParentCorpusOptions(t *testing.T, id string) provider.CallOptions {
	t.Helper()
	approved := false
	switch id {
	case "empty":
		return provider.CallOptions{}
	case "exact-large-numbers":
		return provider.CallOptions{
			MaxOutputTokens: ptrLanguageInt64(9007199254740993), TopK: ptrLanguageInt(-17), Seed: ptrLanguageInt(0),
		}
	case "settings-and-opaque-options":
		return provider.CallOptions{
			MaxOutputTokens: ptrLanguageInt(128), Temperature: ptrFloat(0.25), TopP: ptrFloat(0.9), TopK: ptrLanguageInt(4),
			PresencePenalty: ptrFloat(-0.5), FrequencyPenalty: ptrFloat(0.75), StopSequences: []string{"stop"},
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object"}`), Name: ptrString("result"), Description: ptrString("schema")},
			Seed:           ptrLanguageInt(7), Reasoning: ptrEffort(provider.ReasoningHigh), Headers: map[string]string{"x-test": "value"},
			ProviderOptions: corpusProviderOptions("test", `{"nested":[1,true,null]}`),
		}
	case "parent-empty-collection-collapse":
		return provider.CallOptions{Prompt: []provider.Message{}, Tools: []provider.Tool{}, StopSequences: []string{}, Headers: map[string]string{}, ProviderOptions: provider.ProviderOptions{}}
	case "system-content-projection":
		data := provider.TextDataContent("ignored")
		return provider.CallOptions{Prompt: []provider.Message{{
			Role: provider.RoleSystem,
			Content: []provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: "first"},
				{Type: provider.ContentPartTypeFile, FilePartFilename: ptrString("ignored.pdf"), MediaType: "application/pdf", Data: &data},
				{Type: provider.ContentPartTypeText, Text: "second", ToolName: "inactive"},
			},
		}}}
	case "mixed-content-inactive-fields":
		data := provider.TextDataContent("active")
		return provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{{
			Type: provider.ContentPartTypeFile, Text: "inactive", Data: &data, FilePartFilename: ptrString("mixed.txt"), MediaType: "text/plain",
			ToolCallID: "inactive-call", ToolName: "inactive-tool", Input: json.RawMessage(`{"x":1}`), ApprovalID: "inactive-approval", Approved: &approved, Reason: ptrString("inactive-reason"),
		}}}}}
	case "mixed-function-provider-tool":
		return provider.CallOptions{
			Tools: []provider.Tool{{
				Type: provider.ToolTypeFunction, Name: "lookup", Description: ptrString("find"), InputSchema: json.RawMessage(`{"type":"object"}`),
				InputExamples: []provider.InputExample{{Input: json.RawMessage(`{"q":"x"}`)}}, Strict: ptrBool(false),
				ID: "provider.inactive", Args: map[string]json.RawMessage{"mode": json.RawMessage(`"fast"`)}, ProviderOptions: corpusProviderOptions("test", `{"flag":true}`),
			}},
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"},
		}
	case "data-precedence":
		data := provider.DataContent{Bytes: []byte("bytes"), Base64: "aWdub3JlZA==", URL: "https://example.com/ignored", Reference: json.RawMessage(`{"ignored":"ref"}`), Text: "ignored"}
		return provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{{
			Type: provider.ContentPartTypeFile, Data: &data, FilePartFilename: ptrString("data.bin"), MediaType: "application/octet-stream",
		}}}}}
	case "reference-string-object":
		data := provider.ReferenceDataContent(json.RawMessage(`{"openai":"file-123"}`))
		return provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/pdf", data))}}
	case "reference-non-string-json":
		data := provider.ReferenceDataContent(json.RawMessage(`{"count":1,"enabled":true,"nested":{"x":null}}`))
		return provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/json", data))}}
	case "tool-result-arms":
		urlData := provider.URLDataContent("https://example.com/report.pdf")
		return provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(
			provider.ToolResultPart("call-1", "lookup", &provider.ToolResultOutput{
				Type: provider.ToolOutputContent,
				Content: []provider.ToolResultContentValue{
					{Type: provider.ToolContentText, Text: "ok"},
					{Type: provider.ToolContentFile, Data: &urlData, MediaType: "application/pdf", Filename: ptrString("report.pdf")},
				},
				Reason: ptrString("inactive"), ProviderOptions: corpusProviderOptions("test", `{"output":true}`),
			}),
			provider.ToolResultPart("call-2", "denied", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: ptrString("not allowed")}),
		)}}
	case "empty-raw-provider-options":
		return provider.CallOptions{
			ProviderOptions: provider.ProviderOptions{"top": provider.RawProviderOption{Key: "top"}},
			Prompt: []provider.Message{
				{Role: provider.RoleUser, ProviderOptions: provider.ProviderOptions{"message": provider.RawProviderOption{Key: "message"}}, Content: []provider.ContentPart{{
					Type: provider.ContentPartTypeText, Text: "value", ProviderOptions: provider.ProviderOptions{"content": provider.RawProviderOption{Key: "content"}},
				}}},
				{Role: provider.RoleTool, Content: []provider.ContentPart{{
					Type: provider.ContentPartTypeToolResult, ToolCallID: "call", ToolName: "tool", Output: &provider.ToolResultOutput{
						Type: provider.ToolOutputText, Text: "ok", ProviderOptions: provider.ProviderOptions{"output": provider.RawProviderOption{Key: "output"}},
					},
				}}},
			},
			Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", ProviderOptions: provider.ProviderOptions{"tool": provider.RawProviderOption{Key: "tool"}}}},
		}
	case "source-and-generated-filename":
		data := provider.URLDataContent("https://example.com/generated.pdf")
		return provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(
			provider.ContentPart{Type: provider.ContentPartTypeSource, SourceType: provider.SourceTypeDocument, ID: "source-1", Title: "Report", MediaType: "application/pdf", Filename: "source.pdf"},
			provider.ContentPart{Type: provider.ContentPartTypeFile, Data: &data, MediaType: "application/pdf", FilePartFilename: ptrString("generated.pdf")},
		)}}
	default:
		t.Fatalf("missing migrated corpus builder for %q", id)
		return provider.CallOptions{}
	}
}

func ptrLanguageInt64(value int64) *provider.LanguageModelNumber {
	number := provider.LanguageModelNumberFromInt64(value)
	return &number
}

func corpusProviderOptions(key, raw string) provider.ProviderOptions {
	return provider.ProviderOptions{key: provider.RawProviderOption{Key: key, Raw: json.RawMessage(raw)}}
}
