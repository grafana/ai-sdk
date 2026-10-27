package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

const (
	httpErrorText   = "intentional server error"
	streamErrorText = "intentional stream error"
)

func init() {
	registerScenario("http-error", handleHTTPError)
	registerScenario("ui-stream-error", handleUIStreamError)
	for _, kind := range []string{"text", "tool"} {
		registerScenario("ui-partial-stream-error-"+kind, func(w http.ResponseWriter, r *http.Request) {
			writeUIStreamError(w, r, &uiStreamErrorModel{partial: kind})
		})
	}
}

func handleHTTPError(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprint(w, httpErrorText)
}

type uiStreamErrorModel struct{ partial string }

func (*uiStreamErrorModel) SpecificationVersion() string               { return "v4" }
func (*uiStreamErrorModel) Provider() string                           { return "test" }
func (*uiStreamErrorModel) ModelID() string                            { return "test-ui-stream-error" }
func (*uiStreamErrorModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*uiStreamErrorModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *uiStreamErrorModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 3)
	switch m.partial {
	case "text":
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "partial"}
	case "tool":
		stream <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "weather"}
		stream <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call-1", Delta: `{"city":"Lon`}
	}
	retryable := m.partial != ""
	stream <- provider.StreamPart{
		Type: provider.PartError,
		APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
			Message:     streamErrorText,
			IsRetryable: &retryable,
		}),
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleUIStreamError(w http.ResponseWriter, r *http.Request) {
	writeUIStreamError(w, r, &uiStreamErrorModel{})
}

func writeUIStreamError(w http.ResponseWriter, r *http.Request, model provider.LanguageModel) {
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("fail")),
	)
	if err := aisdk.WriteUIMessageStream(w, result,
		aisdk.OnUIMessageStreamError(func(error) string { return streamErrorText }),
	); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
