package openaicompatible

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

// ChatCompletionFileOptions preserves the native Chat file union for this
// common file part. Identity, bytes, media type and filename come from the part.
// This codec flag is consumed locally and does not alter default conversion.
type ChatCompletionFileOptions struct {
	PreserveNativeFile bool `json:"preserveNativeFile,omitempty"`
}

func (ChatCompletionFileOptions) ProviderKey() string { return "openaiCompatible" }

func nativeFileRequested(options provider.ProviderOptions) (bool, error) {
	switch value := options["openaiCompatible"].(type) {
	case ChatCompletionFileOptions:
		return value.PreserveNativeFile, nil
	case provider.RawProviderOption:
		var parsed ChatCompletionFileOptions
		if err := json.Unmarshal(value.Raw, &parsed); err != nil {
			return false, fmt.Errorf("openai: invalid native file option: %w", err)
		}
		return parsed.PreserveNativeFile, nil
	default:
		return false, nil
	}
}

func nativeChatFile(part provider.ContentPart, providerName string) (chatContentPart, error) {
	file := &filePart{Filename: part.Filename}
	if part.Data.IsReference() {
		var ids map[string]string
		if json.Unmarshal(part.Data.Reference, &ids) != nil || ids[providerName] == "" {
			return chatContentPart{}, fmt.Errorf("openai: invalid native file reference")
		}
		id := ids[providerName]
		file.FileID = &id
	} else {
		normalized, err := normalizeDataURLFilePart(part)
		if err != nil {
			return chatContentPart{}, err
		}
		if normalized.Data.IsURL() {
			return chatContentPart{}, fmt.Errorf("openai: native Chat files require data or reference")
		}
		data, err := dataURL(mediaType(normalized.MediaType), normalized.Data)
		if err != nil {
			return chatContentPart{}, err
		}
		file.FileData = data
	}
	return chatContentPart{Type: "file", File: file}, nil
}
