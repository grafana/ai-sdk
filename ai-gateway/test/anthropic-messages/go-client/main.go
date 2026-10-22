package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type result struct {
	UnaryText         string          `json:"unary_text"`
	UnaryStopReason   string          `json:"unary_stop_reason"`
	UnaryInputTokens  int64           `json:"unary_input_tokens"`
	UnaryOutputTokens int64           `json:"unary_output_tokens"`
	ToolText          string          `json:"tool_text"`
	StreamID          string          `json:"stream_id"`
	StreamUsage       anthropic.Usage `json:"stream_usage"`
	StreamText        string          `json:"stream_text"`
	Stream            []string        `json:"stream_events"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: anthropic-messages-go <base-url> <gateway-token>")
	}
	client := anthropic.NewClient(option.WithBaseURL(os.Args[1]), option.WithAPIKey(os.Args[2]), option.WithMaxRetries(0))
	ctx := context.Background()
	response, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model: anthropic.Model("assistant"), MaxTokens: 64,
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("go unary"))},
	})
	if err != nil {
		return fmt.Errorf("anthropic Go SDK unary request: %w", err)
	}
	output := result{UnaryText: messageText(response)}
	output.UnaryStopReason = string(response.StopReason)
	output.UnaryInputTokens = response.Usage.InputTokens
	output.UnaryOutputTokens = response.Usage.OutputTokens
	toolParam := anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{Type: "object", Properties: map[string]any{"city": map[string]any{"type": "string"}}}, "weather")
	toolResponse, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model: anthropic.Model("assistant"), MaxTokens: 64, Tools: []anthropic.ToolUnionParam{toolParam},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("go tool"))},
	})
	if err != nil {
		return fmt.Errorf("anthropic Go SDK tool request: %w", err)
	}
	var toolUse anthropic.ToolUseBlock
	for _, content := range toolResponse.Content {
		if content.Type == "tool_use" {
			toolUse = content.AsToolUse()
			break
		}
	}
	if toolUse.ID == "" {
		return fmt.Errorf("anthropic Go SDK response did not contain tool_use")
	}
	continued, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model: anthropic.Model("assistant"), MaxTokens: 64, Tools: []anthropic.ToolUnionParam{toolParam},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("go tool")),
			anthropic.NewAssistantMessage(anthropic.NewToolUseBlock(toolUse.ID, toolUse.Input, toolUse.Name)),
			anthropic.NewUserMessage(anthropic.NewToolResultBlock(toolUse.ID, "sunny", false)),
		},
	})
	if err != nil {
		return fmt.Errorf("anthropic Go SDK tool continuation: %w", err)
	}
	output.ToolText = messageText(continued)
	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model: anthropic.Model("assistant"), MaxTokens: 64,
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("go incremental stream"))},
	})
	var streamed anthropic.Message
	for stream.Next() {
		if err := streamed.Accumulate(stream.Current()); err != nil {
			return fmt.Errorf("anthropic Go SDK accumulating stream: %w", err)
		}
		part := stream.Current().AsAny()
		output.Stream = append(output.Stream, fmt.Sprintf("%T", part))
		if textDelta, ok := part.(anthropic.ContentBlockDeltaEvent); ok && textDelta.Delta.Type == "text_delta" {
			output.StreamText += textDelta.Delta.AsTextDelta().Text
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("anthropic Go SDK streaming request: %w", err)
	}
	output.StreamID = streamed.ID
	output.StreamUsage = streamed.Usage
	if output.UnaryText == "" || output.ToolText == "" || output.UnaryStopReason != "end_turn" || output.UnaryInputTokens != 7 || output.UnaryOutputTokens != 3 || output.StreamText != "synthetic stream" || len(output.Stream) == 0 {
		return fmt.Errorf("anthropic Go SDK received an incomplete response")
	}
	return json.NewEncoder(os.Stdout).Encode(output)
}

func messageText(message *anthropic.Message) string {
	if message == nil {
		return ""
	}
	for _, content := range message.Content {
		if content.Type == "text" {
			return content.AsText().Text
		}
	}
	return ""
}
