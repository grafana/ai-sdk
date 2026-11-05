package chatcompletions

import (
	"encoding/json"
	"fmt"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOfficialGoResponseMetadataIsNative(t *testing.T) {
	for _, count := range []int{1, 2} {
		for _, metadata := range []string{`{}`, `{"tag":"native"}`} {
			native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				choices := make([]map[string]any, count)
				for i := range choices {
					choices[i] = map[string]any{"index": i, "message": map[string]any{"content": "ok"}, "finish_reason": "stop"}
				}
				raw, _ := json.Marshal(choices)
				_, _ = fmt.Fprintf(w, `{"choices":%s,"metadata":%s}`, raw, metadata)
			}))
			model := compatible.New("native", compatible.WithBaseURL(native.URL), compatible.WithProviderName("custom.chat"))
			server := httptest.NewServer(nativeChoiceHandler(t, execution.Translate(model, execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"})))
			client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
			result, err := client.Chat.Completions.New(t.Context(), openaisdk.ChatCompletionNewParams{Model: "alias", N: openaisdk.Int(int64(count)), Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("hi")}}, option.WithJSONSet("metadata", map[string]string{"tag": "request"}))
			require.NoError(t, err)
			var got map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(result.RawJSON()), &got))
			assert.JSONEq(t, metadata, string(got["metadata"]))
			server.Close()
			native.Close()
		}
	}
}
