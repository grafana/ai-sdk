package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerwirev4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"go.yaml.in/yaml/v4"
)

type recordedUsageEvent struct {
	line []byte
	kind string
}
type recordedNativeRequest struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

func registerRecordedUsage(mux *http.ServeMux, root string) (func(), error) {
	var entries []catalog.StaticEntry
	var servers []*httptest.Server
	closeServers := func() {
		for _, server := range servers {
			server.Close()
		}
	}
	fixtures := []struct{ name, dir, family string }{
		{"usage-compaction-advisor", filepath.Join(root, "usage-compaction-advisor"), "anthropic"},
		{"usage-fallback", filepath.Join(root, "usage-fallback"), "anthropic"},
		{"simple-text", filepath.Join(root, "simple-text"), "anthropic"},
		{"tool-call", filepath.Join(root, "tool-call"), "anthropic"},
		{"thinking-tool-signature-roundtrip", filepath.Join(root, "thinking-tool-signature-roundtrip"), "anthropic"},
		{"openai-reasoning-text", filepath.Join(root, "..", "..", "openai", "recorded", "reasoning-text"), "openai"},
	}
	for _, fixture := range fixtures {
		configBytes, err := os.ReadFile(filepath.Join(fixture.dir, "config.yaml"))
		if err != nil {
			closeServers()
			return nil, fmt.Errorf("reading recorded config: %w", err)
		}
		var config struct {
			Model  string `yaml:"model"`
			Prompt string `yaml:"prompt"`
		}
		if err := yaml.Unmarshal(configBytes, &config); err != nil || config.Model == "" || config.Prompt == "" {
			closeServers()
			return nil, fmt.Errorf("invalid recorded config for %s", fixture.name)
		}
		paths, err := filepath.Glob(filepath.Join(fixture.dir, "input*.chunks.txt"))
		if err != nil || len(paths) == 0 {
			closeServers()
			return nil, fmt.Errorf("finding recorded input for %s", fixture.name)
		}
		streams := make([][]recordedUsageEvent, 0, len(paths))
		for _, path := range paths {
			input, err := os.ReadFile(path)
			if err != nil {
				closeServers()
				return nil, fmt.Errorf("reading recorded input: %w", err)
			}
			var events []recordedUsageEvent
			scanner := bufio.NewScanner(bytes.NewReader(input))
			scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
			for scanner.Scan() {
				line := scanner.Bytes()
				if len(line) == 0 {
					continue
				}
				var event struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(line, &event) != nil || event.Type == "" {
					closeServers()
					return nil, fmt.Errorf("invalid recorded event for %s", fixture.name)
				}
				events = append(events, recordedUsageEvent{line: bytes.Clone(line), kind: event.Type})
			}
			if err := scanner.Err(); err != nil || len(events) == 0 {
				closeServers()
				return nil, fmt.Errorf("scanning recorded input for %s", fixture.name)
			}
			streams = append(streams, events)
		}
		var mu sync.Mutex
		var requests []recordedNativeRequest
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(io.LimitReader(r.Body, (4<<20)+1))
			if err != nil || len(body) > 4<<20 {
				http.Error(w, "invalid native request", http.StatusBadRequest)
				return
			}
			mu.Lock()
			events := streams[len(requests)%len(streams)]
			requests = append(requests, recordedNativeRequest{Method: r.Method, URL: r.URL.RequestURI(), Headers: r.Header.Clone(), Body: string(body)})
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			for _, event := range events {
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.kind, event.line); err != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}))
		servers = append(servers, backend)
		var model provider.LanguageModel
		if fixture.family == "anthropic" {
			model = anthropicprovider.New("test-api-key", config.Model, anthropicprovider.WithRequestOptions(option.WithBaseURL(backend.URL), option.WithMaxRetries(0)))
		} else {
			model = openaiprovider.NewResponsesWithClient(openaisdk.NewClient(openaioption.WithAPIKey("test-api-key"), openaioption.WithBaseURL(backend.URL+"/v1"), openaioption.WithMaxRetries(0)), config.Model)
		}
		entries = append(entries, catalog.StaticEntry{Info: catalog.ModelInfo{ID: fixture.name}, Model: model})
		mux.HandleFunc("GET /recorded-usage/"+fixture.name+"/requests", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"count": len(requests), "requests": requests})
		})
	}
	resolver, err := catalog.NewStatic(entries)
	if err != nil {
		closeServers()
		return nil, err
	}
	created, err := providerwirev4.New(providerwirev4.Config{Resolver: resolver, Limits: providerwirev4.Limits{RequestBytes: 1 << 20, UnaryResponseBytes: 1 << 20, StreamParts: 1000, StreamFrameBytes: 1 << 20, ModelDuration: 10 * time.Second, StreamIdleDuration: 2 * time.Second, StreamDrainDuration: 100 * time.Millisecond}})
	if err != nil {
		closeServers()
		return nil, err
	}
	mux.Handle("/recorded-usage/language-model", http.StripPrefix("/recorded-usage", created))
	return closeServers, nil
}
