package bedrock

// normalizeToolCallID rewrites a tool call ID into a form acceptable by the
// model. For non-Mistral models the ID passes through unchanged. For Mistral
// models on Bedrock, the ID must match `^[a-zA-Z0-9]{9}$` -- exactly 9
// alphanumeric characters. Bedrock generates IDs like
// `tooluse_bpe71yCfRu2b5i-nKGDr5g` which Mistral rejects, so we keep the
// first 9 alphanumeric characters.
//
// Mirrors upstream `normalize-tool-call-id.ts`.
func normalizeToolCallID(toolCallID string, isMistral bool) string {
	if !isMistral {
		return toolCallID
	}
	var buf [9]byte
	n := 0
	for i := 0; i < len(toolCallID) && n < 9; i++ {
		c := toolCallID[i]
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			buf[n] = c
			n++
		}
	}
	return string(buf[:n])
}
