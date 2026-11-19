// Package responsescodec replays recorded OpenAI Responses inputs to official
// clients, once directly and once through the Go OpenAI provider and the
// Responses codec, so the two results can be compared.
package responsescodec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/openai/openai-go/v3/option"
	"go.yaml.in/yaml/v4"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/openai/responses"
	"github.com/grafana/ai-sdk/providers/openai"
)

// Fixtures are the recorded OpenAI inputs in scope. web-search,
// code-interpreter and computer-call use hosted tools the codec refuses.
var Fixtures = []string{
	"simple-text",
	"tool-call",
	"parallel-tool-calls",
	"multi-tool-chain",
	"reasoning-text",
	"reasoning-tool-use",
	"structured-array-output",
	"structured-choice-output",
	"structured-json-output",
	"structured-json-output-length",
	"structured-json-raw-output",
}

// Model is the catalog model ID that client requests name.
const Model = "fixture"

type fixtureConfig struct {
	Model           string `yaml:"model"`
	Prompt          string `yaml:"prompt"`
	ProviderOptions struct {
		OpenAI struct {
			ReasoningEffort  string `yaml:"reasoningEffort"`
			ReasoningSummary string `yaml:"reasoningSummary"`
		} `yaml:"openai"`
	} `yaml:"providerOptions"`
	Tools map[string]struct {
		Description string         `yaml:"description"`
		InputSchema map[string]any `yaml:"inputSchema"`
	} `yaml:"tools"`
	ResponseFormat *struct {
		Name string `yaml:"name"`
	} `yaml:"responseFormat"`
}

// Fixture is one recorded scenario and its provider call inputs.
type Fixture struct {
	Name   string   `json:"name"`
	Inputs []string `json:"inputs"`
}

// Handler serves:
//
//	GET  /fixtures                                   the fixtures and their inputs
//	GET  /request/{fixture}                          the client request body for a fixture
//	POST /direct/{fixture}/{input}/v1/responses      the recorded input as SSE
//	POST /codec/{fixture}/{input}/v1/responses       the recorded input through the provider and codec
func Handler(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fixtures", func(w http.ResponseWriter, _ *http.Request) {
		var fixtures []Fixture
		for _, name := range Fixtures {
			inputs, err := Inputs(root, name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			fixtures = append(fixtures, Fixture{name, inputs})
		}
		_ = json.NewEncoder(w).Encode(fixtures)
	})
	mux.HandleFunc("GET /request/{fixture}", func(w http.ResponseWriter, req *http.Request) {
		body, err := Request(root, req.PathValue("fixture"))
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		_, _ = w.Write(body)
	})
	mux.HandleFunc("POST /direct/{fixture}/{input}/v1/responses", func(w http.ResponseWriter, req *http.Request) {
		replay(w, filepath.Join(root, req.PathValue("fixture"), req.PathValue("input")+".chunks.txt"))
	})
	mux.HandleFunc("POST /codec/{fixture}/{input}/v1/responses", func(w http.ResponseWriter, req *http.Request) {
		config, err := loadConfig(root, req.PathValue("fixture"))
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		base := fmt.Sprintf("http://%s/direct/%s/%s/v1", req.Host, req.PathValue("fixture"), req.PathValue("input"))
		model := openai.NewResponses("replay-key", config.Model, openai.WithRequestOptions(option.WithBaseURL(base)))
		c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: Model}, Model: execution.Translate(model, execution.Target{Type: execution.TargetOpenAI})}})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		h, err := responses.New(runtime)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		req.URL.Path = responses.Path
		h.ServeHTTP(w, req)
	})
	return mux
}

// Inputs lists a fixture's recorded inputs, without the .chunks.txt suffix.
func Inputs(root, fixture string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(root, fixture, "input*.chunks.txt"))
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("no recorded inputs for %s", fixture)
	}
	sort.Strings(matches)
	inputs := make([]string, 0, len(matches))
	for _, m := range matches {
		inputs = append(inputs, filepath.Base(m[:len(m)-len(".chunks.txt")]))
	}
	return inputs, nil
}

func loadConfig(root, fixture string) (fixtureConfig, error) {
	var config fixtureConfig
	raw, err := os.ReadFile(filepath.Join(root, fixture, "config.yaml"))
	if err != nil {
		return config, err
	}
	return config, yaml.Unmarshal(raw, &config)
}

// Request builds the streaming Responses request a client sends for fixture.
// The replayed input ignores it, but the codec validates output against its
// tools and text format.
func Request(root, fixture string) ([]byte, error) {
	config, err := loadConfig(root, fixture)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"model":  Model,
		"input":  []any{map[string]any{"role": "user", "content": config.Prompt}},
		"stream": true,
		"store":  false,
	}
	if o := config.ProviderOptions.OpenAI; o.ReasoningEffort != "" || o.ReasoningSummary != "" {
		reasoning := map[string]any{}
		if o.ReasoningEffort != "" {
			reasoning["effort"] = o.ReasoningEffort
		}
		if o.ReasoningSummary != "" {
			reasoning["summary"] = o.ReasoningSummary
		}
		body["reasoning"] = reasoning
	}
	if len(config.Tools) > 0 {
		names := make([]string, 0, len(config.Tools))
		for name := range config.Tools {
			names = append(names, name)
		}
		sort.Strings(names)
		var tools []any
		for _, name := range names {
			t := config.Tools[name]
			tools = append(tools, map[string]any{"type": "function", "name": name, "description": t.Description, "parameters": t.InputSchema, "strict": false})
		}
		body["tools"] = tools
	}
	if config.ResponseFormat != nil {
		body["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": config.ResponseFormat.Name, "schema": map[string]any{"type": "object"}}}
	}
	return json.Marshal(body)
}

// replay writes each recorded event line as an SSE event.
func replay(w http.ResponseWriter, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &head) != nil {
			http.Error(w, "invalid recorded event", 500)
			return
		}
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", head.Type, line)
	}
}
