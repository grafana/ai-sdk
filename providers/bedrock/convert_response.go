package bedrock

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

// parseResponse decodes a non-streaming Converse response body into a
// `provider.GenerateResult`. The `meta` carries flags collected during
// request preparation (notably whether the synthetic json tool was injected,
// which changes how a tool-call response is surfaced).
func parseResponse(body []byte, headers map[string][]string, modelID string, meta requestMeta, generateID func() string) (*provider.GenerateResult, error) {
	var resp converseResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bedrock: decoding response: %w", err)
	}

	if resp.Output == nil {
		return nil, fmt.Errorf("bedrock: response missing output.message")
	}

	content := make([]provider.GenerateContentPart, 0, len(resp.Output.Message.Content))
	var isJSONResponseFromTool bool
	var jsonExtractor *jsonObjectTextExtractor
	if meta.usesJSONInstruction {
		jsonExtractor = &jsonObjectTextExtractor{}
	}
	for _, part := range resp.Output.Message.Content {
		switch {
		case part.Text != "":
			text := part.Text
			if jsonExtractor != nil {
				text = jsonExtractor.process(text)
			}
			content = append(content, provider.GenerateContentPart{
				Type: provider.ContentText,
				Text: text,
			})
		case part.ToolUse != nil:
			// JSON-response-tool collapse: when the synthetic json tool is
			// invoked, emit a text part carrying the stringified input so
			// downstream output parsers receive the JSON.
			if meta.usesJSONResponseTool && part.ToolUse.Name == jsonResponseToolName {
				isJSONResponseFromTool = true
				content = append(content, provider.GenerateContentPart{
					Type: provider.ContentText,
					Text: string(part.ToolUse.Input),
				})
				continue
			}
			// Fall back to a generated id/name when the server omits them
			// (matches upstream's `?? generateId()` behavior).
			rawToolCallID := part.ToolUse.ToolUseID
			if rawToolCallID == "" && generateID != nil {
				rawToolCallID = generateID()
			}
			toolName := part.ToolUse.Name
			if toolName == "" && generateID != nil {
				toolName = "tool-" + generateID()
			}
			content = append(content, provider.GenerateContentPart{
				Type:       provider.ContentToolCall,
				ToolCallID: normalizeToolCallID(rawToolCallID, meta.isMistral),
				ToolName:   toolName,
				Input:      part.ToolUse.Input,
			})
		case part.ReasoningContent != nil:
			rc := part.ReasoningContent
			cp := provider.GenerateContentPart{Type: provider.ContentReasoning}
			switch {
			case rc.ReasoningText != nil:
				cp.Text = rc.ReasoningText.Text
				if rc.ReasoningText.Signature != "" {
					cp.ProviderMetadata = provider.ProviderMetadata{
						"amazonBedrock": jsonRawOrZero(ReasoningMetadata{Signature: rc.ReasoningText.Signature}),
						"bedrock":       jsonRawOrZero(ReasoningMetadata{Signature: rc.ReasoningText.Signature}),
					}
				}
			case rc.RedactedReasoning != nil:
				cp.Text = ""
				cp.ProviderMetadata = provider.ProviderMetadata{
					"amazonBedrock": jsonRawOrZero(ReasoningMetadata{RedactedData: rc.RedactedReasoning.Data}),
					"bedrock":       jsonRawOrZero(ReasoningMetadata{RedactedData: rc.RedactedReasoning.Data}),
				}
			}
			content = append(content, cp)
		}
	}

	finish := mapFinishReason(resp.StopReason, isJSONResponseFromTool)

	result := &provider.GenerateResult{
		Content:      content,
		FinishReason: finish,
		Usage:        convertUsage(resp.Usage),
		Response:     buildResponseMetadata(headers, modelID),
	}

	if pm := buildProviderMetadata(resp, isJSONResponseFromTool); pm != nil {
		result.ProviderMetadata = pm
	}

	return result, nil
}

// mapFinishReason translates Bedrock stop reason strings to the unified
// finish reason. The isJSONResponseFromTool flag flips `tool_use` to `stop`
// because the synthetic JSON tool acts as the model's final answer rather
// than a downstream tool call. Mirrors upstream behavior.
func mapFinishReason(stopReason string, isJSONResponseFromTool bool) provider.FinishReason {
	out := provider.FinishReason{Raw: stopReason}
	switch stopReason {
	case "end_turn", "stop_sequence":
		out.Unified = provider.FinishReasonStop
	case "max_tokens":
		out.Unified = provider.FinishReasonLength
	case "content_filtered", "guardrail_intervened":
		out.Unified = provider.FinishReasonContentFilter
	case "tool_use":
		if isJSONResponseFromTool {
			out.Unified = provider.FinishReasonStop
		} else {
			out.Unified = provider.FinishReasonToolCalls
		}
	default:
		out.Unified = provider.FinishReasonOther
	}
	return out
}

// convertUsage maps Bedrock usage block to provider.Usage with cache-token
// detail. Bedrock reports `inputTokens` exclusive of cache reads and writes;
// total is the sum.
func convertUsage(u *converseUsage) provider.Usage {
	if u == nil {
		return provider.Usage{}
	}
	noCache := u.InputTokens
	cacheRead := 0
	cacheWrite := 0
	if u.CacheReadInputTokens != nil {
		cacheRead = *u.CacheReadInputTokens
	}
	if u.CacheWriteInputTokens != nil {
		cacheWrite = *u.CacheWriteInputTokens
	}
	total := noCache + cacheRead + cacheWrite
	out := provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total:      intPtr(total),
			NoCache:    intPtr(noCache),
			CacheRead:  intPtr(cacheRead),
			CacheWrite: intPtr(cacheWrite),
		},
		OutputTokens: provider.OutputTokenUsage{
			Total: intPtr(u.OutputTokens),
			Text:  intPtr(u.OutputTokens),
		},
	}
	if raw, err := json.Marshal(u); err == nil {
		out.Raw = raw
	}
	return out
}

func intPtr(v int) *int { return &v }

// buildResponseMetadata builds GenerateResult.Response from Bedrock response
// headers. Bedrock sets x-amzn-requestid; the date header carries the
// response time.
func buildResponseMetadata(headers map[string][]string, modelID string) *provider.GenerateResponse {
	if headers == nil {
		return &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ModelID: modelID, Provider: providerName}}
	}
	get := func(key string) string {
		for k, v := range headers {
			if len(v) == 0 {
				continue
			}
			if equalFoldASCII(k, key) {
				return v[0]
			}
		}
		return ""
	}
	flat := make(map[string]string, len(headers))
	for k, v := range headers {
		if len(v) > 0 {
			flat[k] = v[0]
		}
	}
	resp := &provider.GenerateResponse{
		ResponseMetadata: provider.ResponseMetadata{
			ID:       get("x-amzn-requestid"),
			ModelID:  modelID,
			Provider: providerName,
		},
		Headers: flat,
	}
	if dateStr := get("date"); dateStr != "" {
		if t, err := time.Parse(time.RFC1123, dateStr); err == nil {
			resp.Timestamp = t
		} else if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			resp.Timestamp = t
		}
	}
	return resp
}

// equalFoldASCII compares two ASCII strings case-insensitively. Header
// canonicalization in net/http already uppercases the first letter but we
// stay lenient.
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// buildProviderMetadata assembles the per-response provider metadata payload
// under the `amazonBedrock` (and legacy `bedrock`) namespaces. Returns nil
// when no metadata fields are populated.
func buildProviderMetadata(resp converseResponse, isJSONResponseFromTool bool) provider.ProviderMetadata {
	payload := map[string]any{}

	if len(resp.Trace) > 0 {
		// Decode trace into a generic value so the encoder writes nested
		// objects rather than a base64 string.
		var trace any
		if err := json.Unmarshal(resp.Trace, &trace); err == nil {
			payload["trace"] = trace
		}
	}
	if len(resp.PerformanceConfig) > 0 {
		var pc any
		if err := json.Unmarshal(resp.PerformanceConfig, &pc); err == nil {
			payload["performanceConfig"] = pc
		}
	}
	if len(resp.ServiceTier) > 0 {
		var st any
		if err := json.Unmarshal(resp.ServiceTier, &st); err == nil {
			payload["serviceTier"] = st
		}
	}
	if resp.Usage != nil {
		usagePayload := map[string]any{}
		if resp.Usage.CacheWriteInputTokens != nil {
			usagePayload["cacheWriteInputTokens"] = *resp.Usage.CacheWriteInputTokens
		}
		if len(resp.Usage.CacheDetails) > 0 {
			var cd any
			if err := json.Unmarshal(resp.Usage.CacheDetails, &cd); err == nil {
				usagePayload["cacheDetails"] = cd
			}
		}
		if len(usagePayload) > 0 {
			payload["usage"] = usagePayload
		}
	}
	if isJSONResponseFromTool {
		payload["isJsonResponseFromTool"] = true
	}

	// stopSequence: upstream defaults to null and treats a non-null value as a
	// trigger for building the metadata payload. When the payload is built for
	// any reason, stopSequence is always present (null or string).
	var stopSequence *string
	if resp.AdditionalModelResponseFields != nil && resp.AdditionalModelResponseFields.Delta != nil {
		stopSequence = resp.AdditionalModelResponseFields.Delta.StopSequence
	}

	if len(payload) == 0 && stopSequence == nil {
		return nil
	}
	// Always include stopSequence (as null or value) once the payload exists,
	// matching upstream's `stopSequence: stopSequence` spread.
	payload["stopSequence"] = stopSequence

	encoded := jsonRawOrZero(payload)
	return provider.ProviderMetadata{
		"amazonBedrock": encoded,
		"bedrock":       encoded,
	}
}
