package provider

import (
	"errors"
	"fmt"
)

// ValidateFileInputs checks file-data selection in direct provider call prompts.
func ValidateFileInputs(prompt []Message) error {
	for messageIndex, message := range prompt {
		for partIndex, part := range message.Content {
			if part.Type == ContentPartTypeReasoningFile {
				if part.Filename != nil || part.Kind != "" {
					return fmt.Errorf("provider: message %d reasoning file part %d has inactive filename or kind", messageIndex, partIndex)
				}
				if err := validateInputFileData(part.Data); err != nil {
					return fmt.Errorf("provider: message %d reasoning file part %d: %w", messageIndex, partIndex, err)
				}
				if message.Role != RoleAssistant || (!part.Data.IsData() && !part.Data.IsURL()) {
					return fmt.Errorf("provider: message %d reasoning file part %d requires assistant data or URL", messageIndex, partIndex)
				}
			}
			if part.Type == ContentPartTypeFile {
				if err := validateInputFileData(part.Data); err != nil {
					return fmt.Errorf("provider: message %d file part %d: %w", messageIndex, partIndex, err)
				}
			}
			if part.Type != ContentPartTypeToolResult || part.Output == nil || part.Output.Type != ToolOutputContent {
				continue
			}
			for contentIndex, content := range part.Output.Content {
				if content.Type != ToolContentFile {
					continue
				}
				if err := validateInputFileData(content.Data); err != nil {
					return fmt.Errorf("provider: message %d tool result %d file %d: %w", messageIndex, partIndex, contentIndex, err)
				}
			}
		}
	}
	return nil
}

func validateInputFileData(data *DataContent) error {
	if data == nil {
		return errors.New("file data is required")
	}
	return data.Validate()
}
