package main

import (
	"context"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("input-start-dynamic", handleInputStartDynamic)
}

type inputStartDynamicModel struct {
	name    string
	dynamic *bool
}

func (*inputStartDynamicModel) SpecificationVersion() string               { return "v4" }
func (*inputStartDynamicModel) Provider() string                           { return "test" }
func (*inputStartDynamicModel) ModelID() string                            { return "input-start-dynamic" }
func (*inputStartDynamicModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*inputStartDynamicModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *inputStartDynamicModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 3)
	stream <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: m.name, Dynamic: m.dynamic}
	stream <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call-1"}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleInputStartDynamic(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("tool")
	var dynamic *bool
	switch r.URL.Query().Get("dynamic") {
	case "true":
		value := true
		dynamic = &value
	case "false":
		value := false
		dynamic = &value
	}
	result := aisdk.StreamText(r.Context(), &inputStartDynamicModel{name: name, dynamic: dynamic},
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithTools(aisdk.ToolSet{
			"dynamic":  {Type: aisdk.UserToolDynamic},
			"ordinary": {Type: aisdk.UserToolFunction},
		}),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
