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
}

func handleHTTPError(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprint(w, httpErrorText)
}

type uiStreamErrorModel struct{}

func (*uiStreamErrorModel) SpecificationVersion() string               { return "v4" }
func (*uiStreamErrorModel) Provider() string                           { return "test" }
func (*uiStreamErrorModel) ModelID() string                            { return "test-ui-stream-error" }
func (*uiStreamErrorModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*uiStreamErrorModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*uiStreamErrorModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 1)
	stream <- provider.StreamPart{
		Type: provider.PartError,
		APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
			Message: streamErrorText,
		}),
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleUIStreamError(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &uiStreamErrorModel{},
		aisdk.WithModelMessages(provider.UserText("fail")),
	)
	if err := aisdk.WriteUIMessageStream(w, result,
		aisdk.OnUIMessageStreamError(func(error) string { return streamErrorText }),
	); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
