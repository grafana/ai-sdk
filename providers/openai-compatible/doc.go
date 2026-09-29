// Package openaicompatible implements provider.LanguageModel for
// OpenAI-compatible Chat Completions APIs.
//
// The provider targets /v1/chat/completions and is intentionally small: it
// works with OpenAI, vLLM, LM Studio, Kimi/Moonshot-style compatible endpoints,
// and local servers that implement the common Chat Completions shape.
//
// Fields in providerOptions that this package does not recognize are merged
// into the request body, which is how an endpoint-specific field reaches a
// backend, and they may replace fields such as model or temperature. Seven
// fields cannot be set that way: messages, tools, tool_choice,
// reasoning_effort, verbosity, stream and stream_options. Sending one is
// reported as an unsupported feature and dropped. Use WithRequestTransform to
// rewrite the finished body.
package openaicompatible
