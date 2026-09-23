package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverseParity_ProfileFamilyRequestCapture(t *testing.T) {
	const profile = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/opaque"
	for _, streaming := range []bool{false, true} {
		name := "generate"
		if streaming {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var req map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &req))
				assert.JSONEq(t, `["/delta/stop_sequence"]`, string(req["additionalModelResponseFieldPaths"]))
				assert.Contains(t, string(req["additionalModelRequestFields"]), `"budget_tokens":2458`)
				if streaming {
					w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
					_, err = w.Write(encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, err = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}} ,"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
				}
				require.NoError(t, err)
			}))
			defer server.Close()
			model := New(profile, WithModelFamily(ModelFamilyAnthropic), WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningHigh}
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
			}
		})
	}
}
