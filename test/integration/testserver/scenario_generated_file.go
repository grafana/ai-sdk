package main

import (
	"context"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("generated-file", handleGeneratedFile)
}

type generatedFileModel struct{}

func (*generatedFileModel) SpecificationVersion() string               { return "v4" }
func (*generatedFileModel) Provider() string                           { return "test" }
func (*generatedFileModel) ModelID() string                            { return "generated-file" }
func (*generatedFileModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*generatedFileModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*generatedFileModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 2)
	stream <- provider.StreamPart{
		Type: provider.PartFile, MediaType: "text/plain",
		Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "data:text/plain;base64,SGVsbG8="},
	}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleGeneratedFile(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &generatedFileModel{}, aisdk.WithModelMessages(provider.UserText("generate a file")))
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
