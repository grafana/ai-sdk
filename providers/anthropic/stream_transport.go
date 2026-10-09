package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/grafana/ai-sdk/provider"
)

const (
	maxPreflightFrames = 64
	maxPreflightBytes  = 1 << 20
)

type messageStreamItem struct {
	event *sdk.BetaRawMessageStreamEventUnion
	err   error
	// errorFrame marks an Anthropic error event: it is reported and reading
	// continues, unlike transport and decoding failures which end the stream.
	errorFrame bool
	rawValue   json.RawMessage
	hasRaw     bool
	frameBytes int
}

type messageFrameDecoder struct {
	frame   ssestream.Event
	pending bool
}

func (d *messageFrameDecoder) Next() bool             { pending := d.pending; d.pending = false; return pending }
func (d *messageFrameDecoder) Event() ssestream.Event { return d.frame }
func (d *messageFrameDecoder) Err() error             { return nil }
func (d *messageFrameDecoder) Close() error           { return nil }

func pumpMessageStream(ctx context.Context, response *http.Response, includeRaw bool) <-chan messageStreamItem {
	items := make(chan messageStreamItem, 64)
	go func() {
		defer close(items)
		send := func(item messageStreamItem) bool {
			select {
			case items <- item:
				return true
			case <-ctx.Done():
				return false
			}
		}
		decoder := ssestream.NewDecoder(response)
		if decoder == nil {
			send(messageStreamItem{err: errors.New("anthropic: missing stream response")})
			return
		}
		var closeOnce sync.Once
		closeDecoder := func() { closeOnce.Do(func() { _ = decoder.Close() }) }
		stopClose := context.AfterFunc(ctx, closeDecoder)
		defer func() { stopClose(); closeDecoder() }()
		for decoder.Next() {
			frame := decoder.Event()
			data := bytes.TrimSpace(frame.Data)
			if bytes.Equal(data, []byte("[DONE]")) {
				return
			}
			if len(frame.Data) == 0 && frame.Type == "" {
				continue
			}
			item := messageStreamItem{hasRaw: includeRaw && len(frame.Data) > 0, frameBytes: len(frame.Data)}
			if includeRaw && json.Valid(frame.Data) {
				item.rawValue = append(json.RawMessage(nil), frame.Data...)
			}
			if frame.Type == "error" {
				if response.Request == nil {
					item.err = fmt.Errorf("received error while streaming: %s", frame.Data)
				} else {
					apiErr := &sdk.Error{StatusCode: response.StatusCode, Request: response.Request, Response: response, RequestID: response.Header.Get("request-id"), WorkspaceID: response.Header.Get("anthropic-workspace-id")}
					if err := apiErr.UnmarshalJSON(frame.Data); err != nil {
						item.err = fmt.Errorf("anthropic: decoding stream error: %w", err)
					} else {
						item.err = apiErr
					}
				}
				item.errorFrame = true
				if !send(item) {
					return
				}
				continue
			}
			stream := ssestream.NewStream[sdk.BetaRawMessageStreamEventUnion](&messageFrameDecoder{frame: frame, pending: true}, nil)
			if stream.Next() {
				event := stream.Current()
				item.event = &event
			}
			if err := stream.Err(); err != nil {
				item.err = fmt.Errorf("anthropic: decoding stream event: %w", err)
			}
			if !send(item) || item.err != nil {
				return
			}
		}
		if err := decoder.Err(); err != nil {
			send(messageStreamItem{err: err})
		}
	}()
	return items
}

func preflightMessageStream(ctx context.Context, items <-chan messageStreamItem, body any) ([]messageStreamItem, error) {
	var buffered []messageStreamItem
	var frames, frameBytes int
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case item, ok := <-items:
			if !ok {
				return buffered, nil
			}
			if item.frameBytes > 0 {
				frames++
				frameBytes += item.frameBytes
				if frames > maxPreflightFrames || frameBytes > maxPreflightBytes {
					retryable := false
					return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "anthropic: stream startup buffer limit exceeded", IsRetryable: &retryable})
				}
			}
			if item.err != nil {
				return nil, wrapInitialStreamError(item.err, body)
			}
			if item.hasRaw || item.event != nil {
				buffered = append(buffered, item)
			}
			if item.event != nil {
				return buffered, nil
			}
		}
	}
}
