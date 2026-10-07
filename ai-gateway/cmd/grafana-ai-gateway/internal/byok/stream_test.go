package byok

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_OpenAIStreamPreflightAndCommitment(t *testing.T) {
	const created = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"model\":\"native-model\",\"output\":[]}}\n\n"
	const added = "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[]}}\n\n"
	const completed = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	const rejection = "event: error\ndata: {\"type\":\"error\",\"code\":\"invalid_api_key\",\"message\":\"dummy-rejection\"}\n\n"
	const rateLimit = "event: error\ndata: {\"type\":\"error\",\"code\":\"rate_limit_exceeded\",\"message\":\"dummy-rejection\"}\n\n"
	for _, tc := range []struct {
		name                  string
		first                 string
		attempts              int
		setupError, partError bool
	}{
		{name: "classified credential rejection stops", first: rejection, attempts: 1, setupError: true},
		{name: "classified rate limit advances", first: rateLimit, attempts: 2},
		{name: "first native part commits before error", first: created + added + rejection, attempts: 1, partError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var keys []string
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				keys = append(keys, r.Header.Get("Authorization"))
				body := created + completed
				if len(keys) == 1 {
					body = tc.first
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			model, err := New("openai/native-model", json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`), client)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result, err := model.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
			if tc.setupError {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				var kinds []provider.StreamPartType
				for part := range result.Stream {
					kinds = append(kinds, part.Type)
				}
				require.NotEmpty(t, kinds)
				assert.Equal(t, provider.PartStreamStart, kinds[0])
				if tc.partError {
					assert.Contains(t, kinds, provider.PartError)
				} else {
					assert.Contains(t, kinds, provider.PartFinish)
				}
			}
			require.Len(t, keys, tc.attempts)
			assert.Equal(t, "Bearer dummy-first", keys[0])
			if tc.attempts == 2 {
				assert.Equal(t, "Bearer dummy-second", keys[1])
			}
		})
	}
}
