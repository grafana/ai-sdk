package openai

import (
	"encoding/json"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// itemReference builds an item_reference input item. The SDK leaves the type
// field empty by default, so set it explicitly to match the upstream wire shape
// ({"type":"item_reference","id":...}).
func assistantOutputMessage(text string, options OpenAIPartOptions) responses.ResponseInputItemUnionParam {
	message := map[string]any{
		"role": "assistant",
		"content": []map[string]any{{
			"type": "output_text",
			"text": text,
		}},
	}
	if options.ItemID != "" {
		message["id"] = options.ItemID
	}
	if options.Phase != "" {
		message["phase"] = options.Phase
	}
	raw, _ := json.Marshal(message)
	value := param.Override[responses.ResponseOutputMessageParam](json.RawMessage(raw))
	return responses.ResponseInputItemUnionParam{OfOutputMessage: &value}
}

func itemReference(id string) responses.ResponseInputItemUnionParam {
	item := responses.ResponseInputItemParamOfItemReference(id)
	if item.OfItemReference != nil {
		item.OfItemReference.Type = "item_reference"
	}
	return item
}

func compactionInputItem(id string, encryptedContent *string) responses.ResponseInputItemUnionParam {
	item := map[string]any{
		"type": "compaction",
		"id":   id,
	}
	if encryptedContent != nil {
		item["encrypted_content"] = *encryptedContent
	}
	raw, _ := json.Marshal(item)
	value := param.Override[responses.ResponseCompactionItemParam](json.RawMessage(raw))
	return responses.ResponseInputItemUnionParam{OfCompaction: &value}
}

// reasoningItem builds a reasoning input item carrying encrypted content and a
// single summary text part.
func reasoningItem(id, encrypted, summary string) responses.ResponseInputItemUnionParam {
	// Upstream only emits a summary_text entry when the reasoning text is
	// non-empty; an empty reasoning part produces an empty summary array.
	summaries := []responses.ResponseReasoningItemSummaryParam{}
	if summary != "" {
		summaries = append(summaries, responses.ResponseReasoningItemSummaryParam{Text: summary})
	}
	item := responses.ResponseInputItemParamOfReasoning(id, summaries)
	if item.OfReasoning != nil && encrypted != "" {
		item.OfReasoning.EncryptedContent = param.NewOpt(encrypted)
	}
	return item
}

// mcpApprovalResponse builds an mcp_approval_response input item.
func mcpApprovalResponse(approvalRequestID string, approve bool) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemParamOfMcpApprovalResponse(approvalRequestID, approve)
}
