package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sync"

	"github.com/grafana/ai-sdk/schema"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const defaultToolSearchMaxResults = 5
const toolSearchInstructions = "Matches become available on the next model step, after this execution finishes. " +
	"Wait for their tool definitions before calling the discovered tools. " +
	"If no tools match, try different keywords."

type toolSearchInput struct {
	Query string `json:"query" jsonschema:"minLength=1"`
}

type toolSearchOutput struct {
	Tools []ToolSearchCandidate `json:"tools"`
}

// ToolSearchCandidate contains only the name and resolved description of an eligible tool.
// A nil description is absent; a pointer to an empty description is explicitly empty.
type ToolSearchCandidate struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// ToolSearchFunc ranks eligible deferred tools. Candidates are sorted by name and
// detached from SDK state. Unknown and duplicate names are ignored before limiting
// results. An empty ranking returns no results. The function should honor ctx and
// be safe for concurrent calls.
type ToolSearchFunc func(ctx context.Context, query string, candidates []ToolSearchCandidate) ([]string, error)

type toolSearchConfig struct {
	maxResults int
	search     ToolSearchFunc
}

// ToolSearchOption configures a ToolSearch tool.
type ToolSearchOption func(*toolSearchConfig)

// WithToolSearchMaxResults sets the maximum matches per search (default five).
// A non-positive limit is rejected before the generation invokes the model.
func WithToolSearchMaxResults(maxResults int) ToolSearchOption {
	return func(cfg *toolSearchConfig) { cfg.maxResults = maxResults }
}

// WithToolSearchFunc replaces keyword ranking. A nil function restores keyword ranking.
func WithToolSearchFunc(search ToolSearchFunc) ToolSearchOption {
	return func(cfg *toolSearchConfig) { cfg.search = search }
}

// ToolSearch returns a core function tool that discovers explicitly deferred tools.
// Matches become available on the next model step within the same generation.
// It must be registered in a ToolSet; its original executor is not bound to a registry.
func ToolSearch(opts ...ToolSearchOption) Tool {
	cfg := toolSearchConfig{maxResults: defaultToolSearchMaxResults}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	input, inputErr := schema.SchemaFor[toolSearchInput]()
	output, outputErr := schema.SchemaFor[toolSearchOutput]()
	return Tool{
		Type:         UserToolFunction,
		Description:  cfg.description(),
		InputSchema:  input,
		OutputSchema: output,
		toolSearch:   &cfg,
		Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			if err := errors.Join(inputErr, outputErr); err != nil {
				return nil, fmt.Errorf("aisdk: compiling tool search schemas: %w", err)
			}
			return nil, errors.New("aisdk: tool search must be bound by an AI SDK generation")
		},
	}
}

func (cfg toolSearchConfig) description() string {
	intro := "Search for tools by keywords in their names and descriptions."
	if cfg.search != nil {
		intro = "Search for tools matching a query."
	}
	limit := "five"
	if cfg.maxResults != defaultToolSearchMaxResults {
		limit = fmt.Sprint(cfg.maxResults)
	}
	return intro + " Returns up to " + limit + " matching tools. " + toolSearchInstructions
}

func resolveToolDescription(tool Tool, runtimeContext any) *string {
	if tool.DescriptionFunc != nil {
		description := tool.DescriptionFunc(ToolDescriptionOptions{Context: runtimeContext})
		return &description
	}
	if tool.Description != "" {
		return &tool.Description
	}
	return nil
}

func resolveToolDescriptions(tools ToolSet, runtimeContext any) ToolSet {
	var resolved ToolSet
	for name, tool := range tools {
		if tool.DescriptionFunc == nil {
			continue
		}
		if resolved == nil {
			resolved = maps.Clone(tools)
		}
		tool.Description = *resolveToolDescription(tool, runtimeContext)
		tool.DescriptionFunc = nil
		resolved[name] = tool
	}
	if resolved == nil {
		return tools
	}
	return resolved
}

type toolSearchState struct {
	mu         sync.Mutex
	discovered map[string]bool
	routes     ToolRoutes
}

func newToolSearchState(tools ToolSet, routes ToolRoutes) (*toolSearchState, error) {
	enabled := false
	for _, name := range slices.Sorted(maps.Keys(tools)) {
		tool := tools[name]
		if !tool.DeferLoading && tool.toolSearch == nil {
			continue
		}
		enabled = true
		if cfg := tool.toolSearch; cfg != nil {
			if tool.DeferLoading {
				return nil, fmt.Errorf("aisdk: search tool %q must not defer loading", name)
			}
			if cfg.maxResults < 1 {
				return nil, fmt.Errorf("aisdk: search tool %q max results must be positive, got %d", name, cfg.maxResults)
			}
		}
		for _, callerName := range routes[name].Callers {
			caller := tools[callerName].Caller
			if caller == nil || caller.Type != ToolCallerLocal || caller.PrepareModelMessage == nil {
				return nil, fmt.Errorf("aisdk: tool %q must be callable directly or through a local caller with PrepareModelMessage", name)
			}
		}
	}
	if !enabled {
		return nil, nil
	}
	return &toolSearchState{discovered: make(map[string]bool), routes: routes}, nil
}

func (s *toolSearchState) route(name string) ToolRoute {
	if route, ok := s.routes[name]; ok {
		return route
	}
	return ToolRoute{Direct: true}
}

func (s *toolSearchState) sharesCaller(searchName, candidateName string, active ToolSet) bool {
	searchRoute, candidateRoute := s.route(searchName), s.route(candidateName)
	if searchRoute.Direct && candidateRoute.Direct {
		return true
	}
	for _, name := range searchRoute.Callers {
		if _, ok := active[name]; ok && slices.Contains(candidateRoute.Callers, name) {
			return true
		}
	}
	return false
}

func (s *toolSearchState) prepare(tools ToolSet, active []string, activeSet bool, runtimeContext any) ToolSet {
	eligible := tools
	if activeSet && tools != nil {
		eligible = make(ToolSet, len(active))
		for _, name := range active {
			if tool, ok := tools[name]; ok {
				eligible[name] = tool
			}
		}
	}
	if s == nil {
		return eligible
	}
	prepared := make(ToolSet, len(eligible))
	s.mu.Lock()
	for name, tool := range eligible {
		if !tool.DeferLoading || s.discovered[name] {
			prepared[name] = tool
		}
	}
	s.mu.Unlock()
	for searchName, tool := range prepared {
		cfg := tool.toolSearch
		if cfg == nil {
			continue
		}
		candidates := make(ToolSet)
		for name, candidate := range eligible {
			if candidate.DeferLoading && candidate.toolSearch == nil && s.sharesCaller(searchName, name, eligible) {
				candidates[name] = candidate
			}
		}
		tool.Execute = func(ctx context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			var args toolSearchInput
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("aisdk: parsing tool search input: %w", err)
			}
			catalog := make([]ToolSearchCandidate, 0, len(candidates))
			for _, name := range slices.Sorted(maps.Keys(candidates)) {
				catalog = append(catalog, ToolSearchCandidate{Name: name, Description: resolveToolDescription(candidates[name], runtimeContext)})
			}
			matches, err := cfg.findMatches(ctx, args.Query, catalog)
			if err != nil {
				return nil, err
			}
			output, err := json.Marshal(toolSearchOutput{Tools: matches})
			if err != nil {
				return nil, err
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for _, match := range matches {
				s.discovered[match.Name] = true
			}
			return output, nil
		}
		prepared[searchName] = tool
	}
	return prepared
}

func (cfg toolSearchConfig) findMatches(ctx context.Context, query string, catalog []ToolSearchCandidate) ([]ToolSearchCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.search == nil {
		return searchDeferredTools(catalog, query, cfg.maxResults), nil
	}

	available := make(map[string]ToolSearchCandidate, len(catalog))
	candidates := slices.Clone(catalog)
	for i, candidate := range catalog {
		available[candidate.Name] = candidate
		if candidate.Description != nil {
			candidates[i].Description = new(*candidate.Description)
		}
	}
	names, err := cfg.search(ctx, query, candidates)
	if err != nil {
		return nil, fmt.Errorf("aisdk: ranking deferred tools: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	matches := make([]ToolSearchCandidate, 0, min(cfg.maxResults, len(catalog), len(names)))
	for _, name := range names {
		match, ok := available[name]
		if !ok {
			continue
		}
		matches = append(matches, match)
		delete(available, name)
		if len(matches) == cfg.maxResults {
			break
		}
	}
	return matches, nil
}

var toolSearchCamelBoundary = regexp.MustCompile(`([a-z\d])([A-Z])`)
var toolSearchWords = regexp.MustCompile(`[\p{L}\p{N}]+`)

func tokenizeToolSearch(text string) []string {
	text = toolSearchCamelBoundary.ReplaceAllString(text, "$1 $2")
	return toolSearchWords.FindAllString(cases.Lower(language.Und).String(text), -1)
}

func searchDeferredTools(candidates []ToolSearchCandidate, query string, maxResults int) []ToolSearchCandidate {
	type scoredMatch struct {
		candidate ToolSearchCandidate
		score     int
	}
	terms := make(map[string]bool)
	for _, term := range tokenizeToolSearch(query) {
		terms[term] = true
	}
	ranked := make([]scoredMatch, 0, len(candidates))
	for _, candidate := range candidates {
		nameTerms := tokenizeToolSearch(candidate.Name)
		var descriptionTerms []string
		if candidate.Description != nil {
			descriptionTerms = tokenizeToolSearch(*candidate.Description)
		}
		score := 0
		for term := range terms {
			if slices.Contains(nameTerms, term) {
				score += 2
			}
			if slices.Contains(descriptionTerms, term) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scoredMatch{candidate: candidate, score: score})
		}
	}
	slices.SortStableFunc(ranked, func(a, b scoredMatch) int { return b.score - a.score })
	matches := make([]ToolSearchCandidate, min(maxResults, len(ranked)))
	for i := range matches {
		matches[i] = ranked[i].candidate
	}
	return matches
}
