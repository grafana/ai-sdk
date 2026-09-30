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

const toolSearchLimit = 5
const toolSearchDescription = "Search for tools by keywords in their names and descriptions. " +
	"Returns up to five matching tools. " +
	"Matches become available on the next model step, after this execution finishes. " +
	"Wait for their tool definitions before calling the discovered tools. " +
	"If no tools match, try different keywords."

type toolSearchInput struct {
	Query string `json:"query" jsonschema:"minLength=1"`
}

type toolSearchOutput struct {
	Tools []toolSearchMatch `json:"tools"`
}

// ToolSearch returns a core function tool that discovers explicitly deferred tools.
// Matches become available on the next model step within the same generation.
// It must be registered in a ToolSet; its original executor is not bound to a registry.
func ToolSearch() Tool {
	input, inputErr := schema.SchemaFor[toolSearchInput]()
	output, outputErr := schema.SchemaFor[toolSearchOutput]()
	return Tool{
		Type:         UserToolFunction,
		Description:  toolSearchDescription,
		InputSchema:  input,
		OutputSchema: output,
		toolSearch:   true,
		Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			if err := errors.Join(inputErr, outputErr); err != nil {
				return nil, fmt.Errorf("aisdk: compiling tool search schemas: %w", err)
			}
			return nil, errors.New("aisdk: tool search must be bound by an AI SDK generation")
		},
	}
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

func resolveToolDescriptions(tools ToolSet, runtimeContext any, active []string, activeSet bool) ToolSet {
	var activeNames map[string]bool
	if activeSet {
		activeNames = make(map[string]bool, len(active))
		for _, name := range active {
			activeNames[name] = true
		}
	}
	var resolved ToolSet
	for name, tool := range tools {
		if tool.DescriptionFunc == nil || (activeSet && !activeNames[name]) {
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
		if !tool.DeferLoading && !tool.toolSearch {
			continue
		}
		enabled = true
		if tool.toolSearch && tool.DeferLoading {
			return nil, fmt.Errorf("aisdk: search tool %q must not defer loading", name)
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
	if s == nil {
		return tools
	}
	eligible := tools
	if activeSet {
		eligible = make(ToolSet, len(active))
		for _, name := range active {
			if tool, ok := tools[name]; ok {
				eligible[name] = tool
			}
		}
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
		if !tool.toolSearch {
			continue
		}
		candidates := make(ToolSet)
		for name, candidate := range eligible {
			if candidate.DeferLoading && !candidate.toolSearch && s.sharesCaller(searchName, name, eligible) {
				candidates[name] = candidate
			}
		}
		tool.Execute = func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
			var args toolSearchInput
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("aisdk: parsing tool search input: %w", err)
			}
			matches := searchDeferredTools(candidates, args.Query, runtimeContext)
			s.mu.Lock()
			for _, match := range matches {
				s.discovered[match.Name] = true
			}
			s.mu.Unlock()
			return json.Marshal(toolSearchOutput{Tools: matches})
		}
		prepared[searchName] = tool
	}
	return prepared
}

type toolSearchMatch struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	score       int
}

var toolSearchCamelBoundary = regexp.MustCompile(`([a-z\d])([A-Z])`)
var toolSearchWords = regexp.MustCompile(`[\p{L}\p{N}]+`)

func tokenizeToolSearch(text string) []string {
	text = toolSearchCamelBoundary.ReplaceAllString(text, "$1 $2")
	return toolSearchWords.FindAllString(cases.Lower(language.Und).String(text), -1)
}

func searchDeferredTools(candidates ToolSet, query string, runtimeContext any) []toolSearchMatch {
	terms := make(map[string]bool)
	for _, term := range tokenizeToolSearch(query) {
		terms[term] = true
	}
	matches := make([]toolSearchMatch, 0, len(candidates))
	for _, name := range slices.Sorted(maps.Keys(candidates)) {
		description := resolveToolDescription(candidates[name], runtimeContext)
		nameTerms := tokenizeToolSearch(name)
		var descriptionTerms []string
		if description != nil {
			descriptionTerms = tokenizeToolSearch(*description)
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
			matches = append(matches, toolSearchMatch{Name: name, Description: description, score: score})
		}
	}
	slices.SortStableFunc(matches, func(a, b toolSearchMatch) int { return b.score - a.score })
	if len(matches) > toolSearchLimit {
		matches = matches[:toolSearchLimit]
	}
	return matches
}
