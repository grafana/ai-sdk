package main

import (
	"time"

	"github.com/grafana/ai-sdk/provider"
)

func scenarioNativeValues(kind string) *provider.GenerateResult {
	sources := scenarioSources()
	sources = append(sources, sources[0], provider.GenerateContentPart{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: "", Title: "", MediaType: "text/plain"})
	result := &provider.GenerateResult{
		Content:      sources,
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Warnings: []provider.Warning{
			{Type: provider.WarnUnsupported, Feature: "native model ☃", Details: "limit=42"},
			{Type: provider.WarnCompatibility, Feature: ""},
			{Type: provider.WarnDeprecated, Setting: "", Message: "use native setting"},
			{Type: provider.WarnOther, Message: ""},
			{Type: provider.WarnOther, Message: "ordinary token-looking text sk-application"},
		},
	}
	switch kind {
	case "native-values":
		result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-response", ModelID: "native model ☃ /", Timestamp: time.Date(2026, 9, 30, 12, 0, 0, 123000000, time.UTC)}}
	case "native-empty":
		result.Response = &provider.GenerateResponse{}
	case "native-partial":
		result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-response"}}
	}
	return result
}

func scenarioNativeStream(kind string) *provider.StreamResult {
	result := scenarioNativeValues(kind)
	parts := []provider.StreamPart{{Type: provider.PartStreamStart, Warnings: result.Warnings}}
	if result.Response != nil {
		parts = append(parts, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: result.Response.ID, ModelID: result.Response.ModelID, Timestamp: result.Response.Timestamp})
	}
	for _, source := range result.Content {
		parts = append(parts, provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: source.SourceType, ID: source.ID, URL: source.URL, Title: source.Title, MediaType: source.MediaType, Filename: source.Filename, ProviderMetadata: source.ProviderMetadata}})
	}
	parts = append(parts, provider.StreamPart{Type: provider.PartFinish, FinishReason: &result.FinishReason, Usage: &result.Usage})
	return &provider.StreamResult{Stream: scenarioStream(parts...)}
}
