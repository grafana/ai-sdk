package main

import (
	"encoding/json"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("finish-step-metadata", handleFinishStepMetadata)
}

func handleFinishStepMetadata(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &simpleTextModel{}, aisdk.WithModelMessages(provider.UserText("hello")))
	err := aisdk.WriteUIMessageStream(w, result, aisdk.WithUIMessageStreamMessageMetadata(func(part aisdk.TextStreamPart) json.RawMessage {
		switch part.(type) {
		case aisdk.StreamFinishStep:
			return json.RawMessage(`{"step":1}`)
		default:
			return nil
		}
	}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
