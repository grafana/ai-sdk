package chatcompletionsadapter_test

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/nativeproxy"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const catalogKey = "fake-stack-chat-key"

func TestOfficialSDK_StackCatalog(t *testing.T) {
	for _, mode := range []platformhost.Topology{platformhost.TopologySelfHosted, platformhost.TopologyCloud} {
		t.Run(string(mode), func(t *testing.T) {
			stack := &upstream{key: catalogKey}
			proxy, err := nativeproxy.Start("api.openai.com:443", http.HandlerFunc(stack.handle))
			require.NoError(t, err)
			t.Cleanup(proxy.Close)
			ca := filepath.Join(t.TempDir(), "native-ca.pem")
			require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: proxy.Certificate.Leaf.Raw}), 0600))

			operator := &upstream{}
			operatorServer := httptest.NewServer(http.HandlerFunc(operator.handle))
			t.Cleanup(operatorServer.Close)
			base := startCommandWithOptions(t, operatorServer.URL, string(mode), commandOptions{
				pluginBuildTag: "gatewaytest",
				environment: []string{
					"GATEWAY_TEST_CA_FILE=" + ca,
					"GODEBUG=x509usefallbackroots=1",
					"HTTPS_PROXY=" + proxy.URL,
					"NO_PROXY=localhost,127.0.0.1",
				},
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			saveSDKCatalog(t, ctx, base)

			client := openai.NewClient(option.WithBaseURL(base+"/modelcatalogs/team/v1/"), option.WithAPIKey(testToken), option.WithMaxRetries(0))
			params := sdkParams(t, `,"max_completion_tokens":32`)
			result, err := client.Chat.Completions.New(ctx, params)
			require.NoError(t, err)
			require.Len(t, result.Choices, 1)
			assert.Equal(t, "hello", result.Choices[0].Message.Content)
			assert.Equal(t, "gpt-4o", result.Model)
			assert.NotContains(t, result.RawJSON(), catalogKey)
			require.Equal(t, 1, stack.count())
			assert.Equal(t, "gpt-4o", stack.last()["model"])
			assert.Equal(t, false, stack.last()["store"])

			params.StreamOptions.IncludeUsage = openai.Bool(true)
			stream := client.Chat.Completions.NewStreaming(ctx, params)
			defer func() { _ = stream.Close() }()
			var text strings.Builder
			var finishes, usage int
			for stream.Next() {
				chunk := stream.Current()
				assert.Equal(t, "gpt-4o", chunk.Model)
				assert.NotContains(t, chunk.RawJSON(), catalogKey)
				if len(chunk.Choices) == 0 {
					usage++
					assert.EqualValues(t, 5, chunk.Usage.TotalTokens)
					continue
				}
				require.Len(t, chunk.Choices, 1)
				text.WriteString(chunk.Choices[0].Delta.Content)
				if chunk.Choices[0].FinishReason != "" {
					finishes++
					assert.Equal(t, "stop", chunk.Choices[0].FinishReason)
				}
			}
			require.NoError(t, stream.Err())
			assert.Equal(t, "hello", text.String())
			assert.Equal(t, 1, finishes)
			assert.Equal(t, 1, usage)
			require.Equal(t, 2, stack.count())
			assert.Equal(t, "gpt-4o", stack.last()["model"])
			assert.Equal(t, true, stack.last()["stream"])
			assert.Zero(t, operator.count())

			namespace := openai.NewClient(option.WithBaseURL(base+"/v1/"), option.WithAPIKey(testToken), option.WithMaxRetries(0))
			result, err = namespace.Chat.Completions.New(ctx, sdkParams(t, `,"max_completion_tokens":32`))
			if mode == platformhost.TopologyCloud {
				var apiError *openai.Error
				require.ErrorAs(t, err, &apiError)
				assert.Equal(t, http.StatusBadRequest, apiError.StatusCode)
				assert.Zero(t, operator.count())
			} else {
				require.NoError(t, err)
				assert.Equal(t, "gpt-4.1", result.Model)
				assert.Equal(t, 1, operator.count())
			}
			assert.Equal(t, 2, stack.count())
		})
	}
}

func saveSDKCatalog(t *testing.T, ctx context.Context, base string) {
	t.Helper()
	body := `{"metadata":{"name":"team"},"spec":{"providers":{"team":{"type":"openai"}},"models":{"team-chat":{"name":"Team Chat","aliases":["chat"],"primary":{"provider":"team","model":"gpt-4o"}}}},"secure":{"team":{"create":"` + catalogKey + `"}}}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/modelcatalogs", strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+testToken)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusCreated, response.StatusCode)
}
