package anthropic

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPumpMessageStream_EmptyEligibleFrame(t *testing.T) {
	response := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: message_delta\n\n"))}
	items := pumpMessageStream(t.Context(), response, true)
	item, ok := <-items
	require.True(t, ok)
	require.Error(t, item.err)
	assert.False(t, item.hasRaw)
	_, ok = <-items
	assert.False(t, ok)
}

func TestDoStream_WhitespaceData(t *testing.T) {
	for _, eventType := range []string{"message_delta", "ping"} {
		for _, data := range []string{"   ", ""} {
			for _, before := range []bool{false, true} {
				for _, raw := range []bool{false, true} {
					t.Run(fmt.Sprintf("event=%s/data=%q/before=%t/raw=%t", eventType, data, before, raw), func(t *testing.T) {
						frame := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
						payload := transportSSE(transportEvents[:1]) + frame + transportSSE(transportEvents[1:])
						if before {
							payload = frame + transportSSE(transportEvents)
						}
						m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
							response := transportResponse(r, true)
							response.Body = io.NopCloser(strings.NewReader(payload))
							return response, nil
						})})))
						result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: raw})
						if eventType == "message_delta" && before {
							require.Error(t, err)
							assert.Nil(t, result)
							return
						}
						require.NoError(t, err)
						var last provider.StreamPart
						var errors, nilRaws int
						for part := range result.Stream {
							if part.Type == provider.PartRaw && part.RawValue == nil {
								nilRaws++
							}
							if part.Type == provider.PartError {
								errors++
								if raw {
									assert.Equal(t, provider.PartRaw, last.Type)
									assert.Nil(t, last.RawValue)
								}
							}
							last = part
						}
						if eventType == "message_delta" {
							assert.Equal(t, 1, errors)
						} else {
							assert.Zero(t, errors)
						}
						if raw {
							assert.Equal(t, 1, nilRaws)
						} else {
							assert.Zero(t, nilRaws)
						}
					})
				}
			}
		}
	}
}
