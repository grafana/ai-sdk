package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerwirev4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"go.yaml.in/yaml/v4"
)

type recordedUsageEvent struct {
	line []byte
	kind string
}

func registerRecordedUsage(mux *http.ServeMux, root string) (func(), error) {
	var entries []catalog.StaticEntry
	var servers []*httptest.Server
	closeServers := func() {
		for _, server := range servers {
			server.Close()
		}
	}
	for _, name := range []string{"usage-compaction-advisor", "usage-fallback"} {
		dir := filepath.Join(root, name)
		configBytes, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
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
			return nil, fmt.Errorf("invalid recorded config for %s: %v", name, err)
		}
		fixture, err := os.ReadFile(filepath.Join(dir, "input.chunks.txt"))
		if err != nil {
			closeServers()
			return nil, fmt.Errorf("reading recorded provider input: %w", err)
		}
		var events []recordedUsageEvent
		scanner := bufio.NewScanner(bytes.NewReader(fixture))
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
				return nil, fmt.Errorf("invalid recorded provider event for %s", name)
			}
			events = append(events, recordedUsageEvent{line: bytes.Clone(line), kind: event.Type})
		}
		if err := scanner.Err(); err != nil || len(events) == 0 {
			closeServers()
			return nil, fmt.Errorf("scanning recorded provider events for %s: %v", name, err)
		}
		var requests atomic.Int64
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
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
		entries = append(entries, catalog.StaticEntry{Info: catalog.ModelInfo{ID: name}, Model: anthropicprovider.New("test-api-key", config.Model, anthropicprovider.WithRequestOptions(option.WithBaseURL(backend.URL), option.WithMaxRetries(0)))})
		mux.HandleFunc("GET /recorded-usage/"+name+"/requests", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]int64{"count": requests.Load()})
		})
	}
	resolver, err := catalog.NewStatic(entries)
	if err != nil {
		closeServers()
		return nil, err
	}
	created, err := providerwirev4.New(providerwirev4.Config{Resolver: resolver, Limits: providerwirev4.Limits{
		RequestBytes: 1 << 20, UnaryResponseBytes: 1 << 20, StreamParts: 1_000, StreamFrameBytes: 1 << 20,
		ModelDuration: 10 * time.Second, StreamIdleDuration: 2 * time.Second, StreamDrainDuration: 100 * time.Millisecond,
	}})
	if err != nil {
		closeServers()
		return nil, err
	}
	mux.Handle("/recorded-usage/language-model", http.StripPrefix("/recorded-usage", created))
	return closeServers, nil
}
