package openai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreflightResponseStream(t *testing.T) {
	requestBody := responses.ResponseNewParams{Model: "gpt-4o"}

	t.Run("pre-cancelled context wins over closed stream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		items := make(chan responseStreamItem)
		close(items)

		buffered, err := preflightResponseStream(ctx, items, requestBody, nil)
		assert.Nil(t, buffered)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("cancellation while waiting is returned directly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		items := make(chan responseStreamItem)
		result := make(chan error, 1)
		go func() {
			_, err := preflightResponseStream(ctx, items, requestBody, nil)
			result <- err
		}()
		cancel()

		assert.ErrorIs(t, <-result, context.Canceled)
		close(items)
	})

	t.Run("error flushed with in progress is returned", func(t *testing.T) {
		items := make(chan responseStreamItem, 2)
		accepted := unmarshalEvent(t, `{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_1","status":"in_progress"}}`)
		failed := unmarshalEvent(t, `{"type":"error","sequence_number":2,"message":"quota exhausted","code":"insufficient_quota"}`)
		items <- responseStreamItem{event: &accepted}
		items <- responseStreamItem{event: &failed}
		close(items)

		buffered, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		assert.Nil(t, buffered)
		require.Error(t, err)
		var apiErr *provider.APICallError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 429, apiErr.StatusCode)
		assert.False(t, apiErr.IsRetryable)
		assert.Equal(t, "insufficient_quota", apiErr.Code)
		assert.JSONEq(t, `{"type":"error","sequence_number":2,"message":"quota exhausted","code":"insufficient_quota"}`, apiErr.ResponseBody)
		assert.NotEmpty(t, apiErr.RequestBodyValues)
	})

	t.Run("returns after accepted grace and surfaces later error in stream", func(t *testing.T) {
		items := make(chan responseStreamItem, 2)
		accepted := unmarshalEvent(t, `{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_1","status":"in_progress"}}`)
		failed := unmarshalEvent(t, `{"type":"error","sequence_number":2,"message":"late failure","code":"rate_limit_error"}`)
		items <- responseStreamItem{event: &accepted}
		go func() {
			time.Sleep(acceptedStreamErrorGrace + 25*time.Millisecond)
			items <- responseStreamItem{event: &failed}
			close(items)
		}()

		buffered, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		require.NoError(t, err)
		require.Len(t, buffered, 1)

		parts := make(chan provider.StreamPart, 8)
		go func() {
			defer close(parts)
			consumeStream(context.Background(), items, buffered, parts, nil, buildResult{}, requestBody, nil, seqIDGen(), "openai")
		}()
		var got []provider.StreamPart
		for part := range parts {
			got = append(got, part)
		}
		require.Len(t, got, 3)
		assert.Equal(t, provider.PartStreamStart, got[0].Type)
		assert.Equal(t, provider.PartError, got[1].Type)
		require.NotNil(t, got[1].APICallError)
		assert.Equal(t, 429, got[1].APICallError.StatusCode)
		assert.Equal(t, provider.PartFinish, got[2].Type)
		require.NotNil(t, got[2].FinishReason)
		assert.Equal(t, provider.FinishReasonError, got[2].FinishReason.Unified)
	})

	t.Run("response failed returns structured error", func(t *testing.T) {
		items := make(chan responseStreamItem, 1)
		failed := unmarshalEvent(t, `{"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","status":"failed","error":{"message":"bad input","code":"invalid_request_error"}}}`)
		items <- responseStreamItem{event: &failed}
		close(items)

		_, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		require.Error(t, err)
		var apiErr *provider.APICallError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 400, apiErr.StatusCode)
		assert.Equal(t, "bad input", apiErr.Message)
	})

	t.Run("first output returns immediately with buffered events", func(t *testing.T) {
		items := make(chan responseStreamItem, 2)
		created := unmarshalEvent(t, `{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","status":"in_progress"}}`)
		output := unmarshalEvent(t, `{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`)
		items <- responseStreamItem{event: &created}
		items <- responseStreamItem{event: &output}
		close(items)

		buffered, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		require.NoError(t, err)
		require.Len(t, buffered, 2)
	})
}
