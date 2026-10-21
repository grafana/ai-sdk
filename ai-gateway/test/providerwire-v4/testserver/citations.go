package main

import "github.com/grafana/ai-sdk/provider"

func scenarioCitations() []provider.Citation {
	return []provider.Citation{
		{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "u", URL: "https://example.com"}, StartIndex: new(0), EndIndex: new(6)},
		{Source: provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "", MediaType: "text/plain", Title: "", ProviderMetadata: provider.ProviderMetadata{}}, CitedText: new("")},
	}
}

func scenarioCitationStream() []provider.StreamPart {
	return []provider.StreamPart{
		{Type: provider.PartResponseMeta, ResponseID: "native"},
		{Type: provider.PartCustom, Kind: "anthropic.message_start", ProviderMetadata: provider.ProviderMetadata{"anthropic": []byte(`{"id":"native","model":"test","usage":{"input_tokens":1,"output_tokens":0}}`)}},
		{Type: provider.PartTextStart, ID: "t"},
		{Type: provider.PartTextDelta, ID: "t", Delta: "answer"},
		{Type: provider.PartTextEnd, ID: "t", Citations: scenarioCitations()},
		{Type: provider.PartToolInputStart, ID: "c", ToolName: "write", InputEncoding: provider.ToolInputEncodingText},
		{Type: provider.PartToolInputDelta, ID: "c", Delta: "SELECT \"value\"\n"},
		{Type: provider.PartToolInputEnd, ID: "c"},
		{Type: provider.PartToolCall, ToolCallID: "c", ToolName: "write", Input: `"SELECT \"value\"\n"`},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}},
	}
}
