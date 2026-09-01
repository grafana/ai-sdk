package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

const acceptedStreamErrorGrace = 50 * time.Millisecond

type responseStreamItem struct {
	event *responses.ResponseStreamEventUnion
	err   error
}

func pumpResponseStream(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion]) <-chan responseStreamItem {
	items := make(chan responseStreamItem, 64)
	go func() {
		defer close(items)
		defer func() { _ = stream.Close() }()
		send := func(item responseStreamItem) bool {
			select {
			case items <- item:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for stream.Next() {
			event := stream.Current()
			if !send(responseStreamItem{event: &event}) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			send(responseStreamItem{err: err})
		}
	}()
	return items
}

func preflightResponseStream(ctx context.Context, items <-chan responseStreamItem, requestBody responses.ResponseNewParams, response *http.Response) ([]responses.ResponseStreamEventUnion, error) {
	var buffered []responses.ResponseStreamEventUnion
	accepted := false
	if err := ctx.Err(); err != nil {
		drainResponseStream(items)
		return nil, err
	}

	for {
		if err := ctx.Err(); err != nil {
			drainResponseStream(items)
			return nil, err
		}
		var (
			item responseStreamItem
			ok   bool
		)
		if accepted {
			timer := time.NewTimer(acceptedStreamErrorGrace)
			select {
			case item, ok = <-items:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			case <-timer.C:
				if err := ctx.Err(); err != nil {
					drainResponseStream(items)
					return nil, err
				}
				return buffered, nil
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				drainResponseStream(items)
				return nil, ctx.Err()
			}
		} else {
			select {
			case item, ok = <-items:
			case <-ctx.Done():
				drainResponseStream(items)
				return nil, ctx.Err()
			}
		}

		if err := ctx.Err(); err != nil {
			drainResponseStream(items)
			return nil, err
		}
		if !ok {
			return buffered, nil
		}
		if item.err != nil {
			if errors.Is(item.err, context.Canceled) || errors.Is(item.err, context.DeadlineExceeded) {
				return nil, item.err
			}
			return nil, wrapStreamTransportError(item.err, requestBody, response)
		}
		if item.event == nil {
			continue
		}
		event := *item.event
		if apiErr := openAIStreamEventError(event, requestBody, response); apiErr != nil {
			drainResponseStream(items)
			return nil, apiErr
		}
		buffered = append(buffered, event)

		switch event.Type {
		case "response.created":
		case "response.in_progress":
			accepted = true
		case "response.failed", "error":
		case "unknown_chunk":
		default:
			return buffered, nil
		}
	}
}

func drainResponseStream(items <-chan responseStreamItem) {
	go func() {
		for range items {
		}
	}()
}

func wrapStreamTransportError(err error, requestBody responses.ResponseNewParams, response *http.Response) error {
	var streamErr *ssestream.StreamError
	if errors.As(err, &streamErr) {
		if apiErr := openAIStreamRawError(streamErr.Event.Data, "error", requestBody, response); apiErr != nil {
			return apiErr
		}
	}
	wrapped := wrapAPIError(err, requestBody)
	if apiErr, ok := wrapped.(*provider.APICallError); ok {
		return apiErr
	}
	requestJSON, _ := json.Marshal(requestBody)
	responseURL, headers := openAIResponseContext(response)
	if responseURL == "" {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			responseURL = urlErr.URL
		}
	}
	retryable := true
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           err.Error(),
		URL:               responseURL,
		RequestBodyValues: requestJSON,
		ResponseHeaders:   headers,
		IsRetryable:       &retryable,
		Cause:             err,
	})
}

func openAIStreamEventError(event responses.ResponseStreamEventUnion, requestBody responses.ResponseNewParams, response *http.Response) *provider.APICallError {
	if event.Type != "error" && event.Type != "response.failed" {
		return nil
	}
	return openAIStreamRawError(json.RawMessage(event.RawJSON()), event.Type, requestBody, response)
}

func openAIStreamRawError(raw json.RawMessage, eventType string, requestBody responses.ResponseNewParams, response *http.Response) *provider.APICallError {
	var frame map[string]any
	if err := json.Unmarshal(raw, &frame); err != nil {
		return nil
	}
	if value, ok := frame["type"].(string); ok && value != "" {
		eventType = value
	}
	var details map[string]any
	if eventType == "response.failed" {
		responseValue, _ := frame["response"].(map[string]any)
		details, _ = responseValue["error"].(map[string]any)
	} else if nested, ok := frame["error"].(map[string]any); ok {
		details = nested
	} else {
		details = frame
	}
	if details == nil {
		return nil
	}
	message, _ := details["message"].(string)
	if message == "" {
		return nil
	}

	requestJSON, _ := json.Marshal(requestBody)
	url, headers := openAIResponseContext(response)
	statusCode := openAIStreamErrorStatus(details["code"], details["type"])
	retryable := statusCode == 408 || statusCode == 409 || statusCode == 429 || statusCode >= 500
	if details["code"] == "insufficient_quota" || details["type"] == "insufficient_quota" {
		retryable = false
	}
	errorType, _ := details["type"].(string)
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           message,
		Type:              errorType,
		Code:              details["code"],
		URL:               url,
		RequestBodyValues: requestJSON,
		StatusCode:        statusCode,
		ResponseHeaders:   headers,
		ResponseBody:      string(raw),
		IsRetryable:       &retryable,
		Data:              append(json.RawMessage(nil), raw...),
	})
}

func openAIResponseContext(response *http.Response) (string, map[string][]string) {
	if response == nil {
		return "", nil
	}
	var url string
	if response.Request != nil && response.Request.URL != nil {
		url = response.Request.URL.String()
	}
	return url, response.Header.Clone()
}

func openAIStreamErrorStatus(code, errorType any) int {
	if numeric, ok := numericHTTPStatus(code); ok {
		return numeric
	}
	discriminator := strings.ToLower(fmt.Sprintf("%v %v", code, errorType))
	switch {
	case strings.Contains(discriminator, "insufficient_quota"), strings.Contains(discriminator, "rate_limit"):
		return 429
	case strings.Contains(discriminator, "authentication"):
		return 401
	case strings.Contains(discriminator, "permission"):
		return 403
	case strings.Contains(discriminator, "not_found"):
		return 404
	case strings.Contains(discriminator, "invalid"), strings.Contains(discriminator, "bad_request"), strings.Contains(discriminator, "context_length"):
		return 400
	case strings.Contains(discriminator, "overload"):
		return 503
	case strings.Contains(discriminator, "timeout"):
		return 504
	default:
		return 500
	}
}

func numericHTTPStatus(value any) (int, bool) {
	var status int
	switch value := value.(type) {
	case float64:
		status = int(value)
		if float64(status) != value {
			return 0, false
		}
	case string:
		parsed, err := strconv.Atoi(value)
		if err != nil || len(value) != 3 {
			return 0, false
		}
		status = parsed
	default:
		return 0, false
	}
	return status, status >= 400 && status <= 599
}
