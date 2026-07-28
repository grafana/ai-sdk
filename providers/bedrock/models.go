package bedrock

import (
	"slices"
	"sort"
)

// knownModelIDs lists the Amazon Bedrock chat model ids from upstream
// @ai-sdk/amazon-bedrock's AmazonBedrockChatModelId union. The list is
// advisory; New accepts any string.
var knownModelIDs = []string{
	"amazon.titan-text-express-v1",
	"amazon.titan-text-lite-v1",
	"amazon.titan-tg1-large",
	"anthropic.claude-3-5-haiku-20241022-v1:0",
	"anthropic.claude-3-5-sonnet-20240620-v1:0",
	"anthropic.claude-3-5-sonnet-20241022-v2:0",
	"anthropic.claude-3-7-sonnet-20250219-v1:0",
	"anthropic.claude-3-haiku-20240307-v1:0",
	"anthropic.claude-3-opus-20240229-v1:0",
	"anthropic.claude-3-sonnet-20240229-v1:0",
	"anthropic.claude-fable-5",
	"anthropic.claude-haiku-4-5-20251001-v1:0",
	"anthropic.claude-instant-v1",
	"anthropic.claude-opus-4-1-20250805-v1:0",
	"anthropic.claude-opus-4-20250514-v1:0",
	"anthropic.claude-opus-4-5-20251101-v1:0",
	"anthropic.claude-opus-4-6-v1",
	"anthropic.claude-opus-4-7",
	"anthropic.claude-opus-4-8",
	"anthropic.claude-sonnet-4-20250514-v1:0",
	"anthropic.claude-sonnet-4-5-20250929-v1:0",
	"anthropic.claude-sonnet-4-6-v1",
	"anthropic.claude-sonnet-5",
	"anthropic.claude-v2",
	"anthropic.claude-v2:1",
	"cohere.command-light-text-v14",
	"cohere.command-r-plus-v1:0",
	"cohere.command-r-v1:0",
	"cohere.command-text-v14",
	"meta.llama3-1-405b-instruct-v1:0",
	"meta.llama3-1-70b-instruct-v1:0",
	"meta.llama3-1-8b-instruct-v1:0",
	"meta.llama3-2-11b-instruct-v1:0",
	"meta.llama3-2-1b-instruct-v1:0",
	"meta.llama3-2-3b-instruct-v1:0",
	"meta.llama3-2-90b-instruct-v1:0",
	"meta.llama3-70b-instruct-v1:0",
	"meta.llama3-8b-instruct-v1:0",
	"mistral.mistral-7b-instruct-v0:2",
	"mistral.mistral-large-2402-v1:0",
	"mistral.mistral-small-2402-v1:0",
	"mistral.mixtral-8x7b-instruct-v0:1",
	"openai.gpt-oss-120b-1:0",
	"openai.gpt-oss-20b-1:0",
	"us.amazon.nova-lite-v1:0",
	"us.amazon.nova-micro-v1:0",
	"us.amazon.nova-premier-v1:0",
	"us.amazon.nova-pro-v1:0",
	"us.anthropic.claude-3-5-haiku-20241022-v1:0",
	"us.anthropic.claude-3-5-sonnet-20240620-v1:0",
	"us.anthropic.claude-3-5-sonnet-20241022-v2:0",
	"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
	"us.anthropic.claude-3-haiku-20240307-v1:0",
	"us.anthropic.claude-3-opus-20240229-v1:0",
	"us.anthropic.claude-3-sonnet-20240229-v1:0",
	"us.anthropic.claude-fable-5",
	"us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"us.anthropic.claude-opus-4-1-20250805-v1:0",
	"us.anthropic.claude-opus-4-20250514-v1:0",
	"us.anthropic.claude-opus-4-5-20251101-v1:0",
	"us.anthropic.claude-opus-4-6-v1",
	"us.anthropic.claude-opus-4-7",
	"us.anthropic.claude-opus-4-8",
	"us.anthropic.claude-sonnet-4-20250514-v1:0",
	"us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"us.anthropic.claude-sonnet-4-6-v1",
	"us.anthropic.claude-sonnet-5",
	"us.deepseek.r1-v1:0",
	"us.meta.llama3-1-70b-instruct-v1:0",
	"us.meta.llama3-1-8b-instruct-v1:0",
	"us.meta.llama3-2-11b-instruct-v1:0",
	"us.meta.llama3-2-1b-instruct-v1:0",
	"us.meta.llama3-2-3b-instruct-v1:0",
	"us.meta.llama3-2-90b-instruct-v1:0",
	"us.meta.llama3-3-70b-instruct-v1:0",
	"us.meta.llama4-maverick-17b-instruct-v1:0",
	"us.meta.llama4-scout-17b-instruct-v1:0",
	"us.mistral.pixtral-large-2502-v1:0",
}

// ModelIDs returns a sorted copy of the advisory known model id list.
func ModelIDs() []string {
	ids := slices.Clone(knownModelIDs)
	sort.Strings(ids)
	return ids
}
