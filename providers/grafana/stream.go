package grafana

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

type eventReader struct {
	reader  *bufio.Reader
	limits  Limits
	total   int64
	count   int64
	skipLF  bool
	started bool
}

var errStreamLimit = errors.New("grafana: stream processing limit exceeded")

func (r *eventReader) next() ([]byte, error) {
	if !r.started {
		r.started = true
		if prefix, _ := r.reader.Peek(3); bytes.Equal(prefix, []byte{0xef, 0xbb, 0xbf}) {
			_, _ = r.reader.Discard(3)
			r.total += 3
			if r.total > r.limits.StreamBytes {
				return nil, errStreamLimit
			}
		}
	}
	var event, line []byte
	var eventBytes int64
	dataPresent := false
	for {
		b, err := r.reader.ReadByte()
		if err != nil {
			return nil, err
		}
		r.total++
		if r.total > r.limits.StreamBytes {
			return nil, errStreamLimit
		}
		if r.skipLF {
			r.skipLF = false
			if b == '\n' {
				continue
			}
		}
		eventBytes++
		if eventBytes > r.limits.StreamEventBytes {
			return nil, errStreamLimit
		}
		if b != '\n' && b != '\r' {
			line = append(line, b)
			continue
		}
		if b == '\r' {
			r.skipLF = true
		}
		if len(line) == 0 {
			if dataPresent {
				r.count++
				if r.count > r.limits.StreamEvents {
					return nil, errStreamLimit
				}
				return bytes.TrimSuffix(event, []byte{'\n'}), nil
			}
			eventBytes = 0
		} else if bytes.Equal(line, []byte("data")) || bytes.HasPrefix(line, []byte("data:")) {
			data := line[4:]
			if len(data) > 0 {
				data = data[1:]
				if len(data) > 0 && data[0] == ' ' {
					data = data[1:]
				}
			}
			event = append(event, data...)
			event = append(event, '\n')
			dataPresent = true
		}
		line = line[:0]
	}
}

func consumeStream(ctx context.Context, body io.ReadCloser, parts chan<- provider.StreamPart, limits Limits, includeRaw bool) {
	defer close(parts)
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = body.Close() }) }
	stop := context.AfterFunc(ctx, closeBody)
	defer stop()
	defer closeBody()
	reader := eventReader{reader: bufio.NewReader(io.LimitReader(body, limits.StreamBytes+1)), limits: limits}
	send := func(part provider.StreamPart) bool {
		if ctx.Err() != nil {
			return false
		}
		select {
		case parts <- part:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for {
		data, err := reader.next()
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			retryable := !errors.Is(err, errStreamLimit)
			api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "grafana: stream read failed", StatusCode: 200, Cause: err, IsRetryable: &retryable})
			send(provider.StreamPart{Type: provider.PartError, APICallError: api})
			return
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		part, err := decodeStreamPart(data)
		if err != nil {
			api := protocolError("grafana: invalid stream event", 200, err)
			send(provider.StreamPart{Type: provider.PartError, APICallError: api})
			return
		}
		if part.Type == provider.PartRaw && !includeRaw {
			continue
		}
		if !send(part) {
			return
		}
	}
}

type wireWarning struct {
	Type    provider.WarningType `json:"type"`
	Feature *string              `json:"feature"`
	Details *string              `json:"details"`
	Setting *string              `json:"setting"`
	Message *string              `json:"message"`
}

func (w *wireWarning) UnmarshalJSON(data []byte) error {
	type plain wireWarning
	return decodeFields(data, (*plain)(w), "type", "feature", "details", "setting", "message")
}

func decodeStreamPart(data []byte) (provider.StreamPart, error) {
	invalid := func() (provider.StreamPart, error) {
		return provider.StreamPart{}, errors.New("grafana: invalid stream part")
	}
	if !validJSON(data) {
		return invalid()
	}
	var value struct {
		Type             provider.StreamPartType `json:"type"`
		ID               *string                 `json:"id"`
		Delta            *string                 `json:"delta"`
		ModelID          *string                 `json:"modelId"`
		Timestamp        *string                 `json:"timestamp"`
		Warnings         *[]wireWarning          `json:"warnings"`
		FinishReason     *wireFinish             `json:"finishReason"`
		Usage            *wireUsage              `json:"usage"`
		RawValue         json.RawMessage         `json:"rawValue"`
		Error            json.RawMessage         `json:"error"`
		ToolCallID       *string                 `json:"toolCallId"`
		ToolName         *string                 `json:"toolName"`
		Input            *string                 `json:"input"`
		Result           json.RawMessage         `json:"result"`
		IsError          bool                    `json:"isError"`
		ProviderExecuted bool                    `json:"providerExecuted"`
		Dynamic          bool                    `json:"dynamic"`
		Preliminary      bool                    `json:"preliminary"`
	}
	if decodeFields(data, &value, "type", "id", "delta", "modelId", "timestamp", "warnings", "finishReason", "usage", "rawValue", "error", "toolCallId", "toolName", "input", "result", "isError", "providerExecuted", "dynamic", "preliminary") != nil {
		return invalid()
	}
	part := provider.StreamPart{Type: value.Type}
	switch value.Type {
	case provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd:
		if value.ID == nil || *value.ID == "" || value.ProviderExecuted || value.Dynamic || value.Preliminary {
			return invalid()
		}
		part.ID = *value.ID
		if value.Type == provider.PartToolInputStart {
			if value.ToolName == nil || *value.ToolName == "" {
				return invalid()
			}
			part.ToolName = *value.ToolName
		}
		if value.Type == provider.PartToolInputDelta {
			if value.Delta == nil {
				return invalid()
			}
			part.Delta = *value.Delta
		}
	case provider.PartToolCall, provider.PartToolResult:
		if value.ToolCallID == nil || *value.ToolCallID == "" || value.ToolName == nil || *value.ToolName == "" || value.ProviderExecuted || value.Dynamic || value.Preliminary {
			return invalid()
		}
		part.ToolCallID, part.ToolName = *value.ToolCallID, *value.ToolName
		if value.Type == provider.PartToolCall {
			if value.Input == nil {
				return invalid()
			}
			part.Input = *value.Input
		} else {
			if len(value.Result) == 0 || string(value.Result) == "null" {
				return invalid()
			}
			part.Result, part.IsError = value.Result, value.IsError
		}
	case provider.PartStreamStart:
		if value.Warnings == nil {
			return invalid()
		}
		part.Warnings = make([]provider.Warning, 0, len(*value.Warnings))
		for _, warning := range *value.Warnings {
			mapped := provider.Warning{Type: warning.Type}
			switch warning.Type {
			case provider.WarnUnsupported, provider.WarnCompatibility:
				if warning.Feature == nil {
					return invalid()
				}
				mapped.Feature = *warning.Feature
				if warning.Details != nil {
					mapped.Details = *warning.Details
				}
			case provider.WarnDeprecated:
				if warning.Setting == nil || warning.Message == nil {
					return invalid()
				}
				mapped.Setting = *warning.Setting
				mapped.Message = *warning.Message
			case provider.WarnOther:
				if warning.Message == nil {
					return invalid()
				}
				mapped.Message = *warning.Message
			default:
				return invalid()
			}
			part.Warnings = append(part.Warnings, mapped)
		}
	case provider.PartResponseMeta:
		if value.ModelID == nil || !publicModelID.MatchString(*value.ModelID) {
			return invalid()
		}
		if value.ID != nil {
			part.ResponseID = *value.ID
		}
		if value.ModelID != nil {
			part.ModelID = *value.ModelID
		}
		if value.Timestamp != nil {
			if strings.Contains(*value.Timestamp, ",") {
				return invalid()
			}
			parsed, err := time.Parse(time.RFC3339Nano, *value.Timestamp)
			if err != nil {
				return invalid()
			}
			part.Timestamp = parsed
		}
	case provider.PartTextStart, provider.PartTextEnd, provider.PartTextDelta:
		if value.ID == nil || strings.TrimSpace(*value.ID) == "" {
			return invalid()
		}
		part.ID = *value.ID
		if value.Type == provider.PartTextDelta {
			if value.Delta == nil {
				return invalid()
			}
			part.Delta = *value.Delta
		}
	case provider.PartFinish:
		finish, err := decodeFinish(value.FinishReason)
		if err != nil {
			return invalid()
		}
		usage, err := decodeUsage(value.Usage)
		if err != nil {
			return invalid()
		}
		part.FinishReason = &finish
		part.Usage = &usage
	case provider.PartRaw:
		if value.RawValue == nil {
			return invalid()
		}
		part.RawValue = value.RawValue
	case provider.PartError:
		var eventError struct {
			wireError
			StatusCode int   `json:"statusCode"`
			Retryable  *bool `json:"retryable"`
		}
		if decodeFields(value.Error, &eventError, "message", "type", "code", "param", "statusCode", "retryable") != nil || eventError.Retryable == nil {
			return invalid()
		}
		err := mapGatewayError(&eventError.wireError, eventError.StatusCode)
		gateway, ok := err.(*GatewayError)
		if !ok || gateway.IsRetryable != *eventError.Retryable {
			return invalid()
		}
		part.APICallError = gateway.cause
	default:
		return invalid()
	}
	return part, nil
}
