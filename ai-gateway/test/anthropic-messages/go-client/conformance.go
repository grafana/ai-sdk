package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func conformance() error {
	var input struct {
		Stream  bool                       `json:"stream"`
		Request anthropic.MessageNewParams `json:"request"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	client := anthropic.NewClient(option.WithBaseURL(os.Args[1]), option.WithAPIKey(os.Args[2]), option.WithMaxRetries(0))
	var output struct {
		Message   *anthropic.Message `json:"message,omitempty"`
		Events    []json.RawMessage  `json:"events"`
		Error     string             `json:"error,omitempty"`
		Status    int                `json:"status,omitempty"`
		ErrorBody json.RawMessage    `json:"errorBody,omitempty"`
	}
	output.Events = []json.RawMessage{}
	var callErr error
	if input.Stream {
		stream := client.Messages.NewStreaming(context.Background(), input.Request)
		defer func() { _ = stream.Close() }()
		message := &anthropic.Message{}
		for stream.Next() {
			event := stream.Current()
			output.Events = append(output.Events, json.RawMessage(event.RawJSON()))
			if callErr = message.Accumulate(event); callErr != nil {
				break
			}
		}
		if callErr == nil {
			callErr = stream.Err()
		}
		if callErr == nil {
			output.Message = message
		}
	} else {
		output.Message, callErr = client.Messages.New(context.Background(), input.Request)
	}
	if callErr != nil {
		output.Error = callErr.Error()
		var apiErr *anthropic.Error
		if errors.As(callErr, &apiErr) {
			output.Status = apiErr.StatusCode
			if json.Valid([]byte(apiErr.RawJSON())) {
				output.ErrorBody = json.RawMessage(apiErr.RawJSON())
			}
		}
	}
	return json.NewEncoder(os.Stdout).Encode(output)
}
