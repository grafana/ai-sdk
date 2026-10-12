package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	v4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	"github.com/openai/openai-go/v3/option"
)

func registerNativeStructured(mux *http.ServeMux) (func(), error) {
	var mu sync.Mutex
	var captured []json.RawMessage
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		mu.Lock()
		captured = append(captured, body)
		mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/failure") {
			w.WriteHeader(503)
			_, _ = fmt.Fprint(w, `{"error":{"message":"native failure","type":"server_error"}}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/invalid") {
			w.WriteHeader(400)
			_, _ = fmt.Fprint(w, `{"error":{"message":"invalid native schema","type":"invalid_request_error"}}`)
			return
		}
		var request struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &request)
		w.Header().Set("Content-Type", "application/json")
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, `data: {"type":"response.completed","response":{"id":"native-structured","model":"backend","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`+"\n\n")
		} else {
			_, _ = fmt.Fprint(w, `{"id":"native-structured","model":"backend","status":"completed","output":[{"type":"message","id":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"value\":\"ok\"}","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		}
	}))
	create := func(path string) provider.LanguageModel {
		return openaiprovider.NewResponses("synthetic-key", "backend", openaiprovider.WithRequestOptions(option.WithBaseURL(native.URL+path), option.WithHTTPClient(native.Client()), option.WithMaxRetries(0)))
	}
	success := create("/success")
	ordered, err := fallback.New(create("/failure"), success)
	if err != nil {
		native.Close()
		return nil, err
	}
	resolver, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "native"}, Model: success}, {Info: catalog.ModelInfo{ID: "native-fallback"}, Model: ordered}, {Info: catalog.ModelInfo{ID: "native-invalid"}, Model: create("/invalid")}})
	if err != nil {
		native.Close()
		return nil, err
	}
	runtime, err := v4.New(v4.Config{Resolver: resolver, Limits: v4.Limits{RequestBytes: 1 << 20, UnaryResponseBytes: 1 << 20, StreamFrameBytes: 1 << 20, StreamParts: 100, ModelDuration: time.Second * 5, StreamIdleDuration: time.Second, StreamDrainDuration: time.Second}})
	if err != nil {
		native.Close()
		return nil, err
	}
	mux.Handle("POST /native-structured"+v4.LanguageModelPath, http.StripPrefix("/native-structured", runtime))
	mux.HandleFunc("GET /native-structured/requests", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(captured)
		captured = nil
	})
	return native.Close, nil
}
