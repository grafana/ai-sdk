// Package openaicompatible implements provider.LanguageModel for
// OpenAI-compatible Chat Completions APIs.
//
// The provider targets /v1/chat/completions and is intentionally small: it
// works with OpenAI, vLLM, LM Studio, Kimi/Moonshot-style compatible endpoints,
// and local servers that implement the common Chat Completions shape.
package openaicompatible
