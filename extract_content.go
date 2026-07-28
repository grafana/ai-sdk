package aisdk

import (
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// ExtractTextContent returns the concatenation of every Text field from
// [provider.ContentPartTypeText] parts in order. Non-text parts (reasoning,
// tool calls, files, etc.) are skipped. Returns "" when no text parts are
// present.
//
// Mirrors upstream's extractTextContent helper, which is a small but commonly
// needed primitive when post-processing a model's response content. Use this
// when you drive your own loop on top of [provider.LanguageModel] or
// post-process structures that carry [[]provider.ContentPart].
func ExtractTextContent(content []provider.ContentPart) string {
	var sb strings.Builder
	for i := range content {
		if content[i].Type == provider.ContentPartTypeText {
			sb.WriteString(content[i].Text)
		}
	}
	return sb.String()
}

// ExtractReasoningContent returns the reasoning text from
// [provider.ContentPartTypeReasoning] parts joined by "\n", matching
// upstream's extractReasoningContent semantics. Non-reasoning parts are
// skipped. Returns "" when no reasoning parts are present.
func ExtractReasoningContent(content []provider.ContentPart) string {
	var sb strings.Builder
	first := true
	for i := range content {
		if content[i].Type != provider.ContentPartTypeReasoning {
			continue
		}
		if !first {
			sb.WriteByte('\n')
		}
		sb.WriteString(content[i].Text)
		first = false
	}
	return sb.String()
}

// ExtractTextContentGenerate is the [provider.GenerateContentPart] counterpart
// of [ExtractTextContent], for use with the content slice returned by
// [provider.LanguageModel.DoGenerate]. Returns the concatenated text from all
// [provider.ContentText] parts, or "" if none are present.
func ExtractTextContentGenerate(content []provider.GenerateContentPart) string {
	var sb strings.Builder
	for i := range content {
		if content[i].Type == provider.ContentText {
			sb.WriteString(content[i].Text)
		}
	}
	return sb.String()
}

// ExtractReasoningContentGenerate is the [provider.GenerateContentPart]
// counterpart of [ExtractReasoningContent], for use with the content slice
// returned by [provider.LanguageModel.DoGenerate]. Returns the reasoning text
// from all [provider.ContentReasoning] parts joined by "\n", or "" if none
// are present.
func ExtractReasoningContentGenerate(content []provider.GenerateContentPart) string {
	var sb strings.Builder
	first := true
	for i := range content {
		if content[i].Type != provider.ContentReasoning {
			continue
		}
		if !first {
			sb.WriteByte('\n')
		}
		sb.WriteString(content[i].Text)
		first = false
	}
	return sb.String()
}
