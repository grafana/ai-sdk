package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestGeneratedFileDecode(t *testing.T) {
	for _, data := range []string{`{"type":"data","data":""}`, `{"type":"data","data":"AQID"}`, `{"type":"url","url":"opaque:url"}`, `{"type":"url","url":"opaque:url","originalUrl":""}`, `{"type":"url","url":"opaque:url","originalUrl":"opaque:Original?token=application"}`} {
		for _, metadata := range []string{"", `,"providerMetadata":{}`, `,"providerMetadata":{"app":{"token":"application","nested":[1,null]}}`} {
			part := `{"type":"file","mediaType":"image/png","data":` + data + metadata + `,"filename":false}`
			result, err := decodeGenerate([]byte(`{"content":[` + part + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`))
			require.NoError(t, err)
			require.Equal(t, provider.ContentFile, result.Content[0].Type)
			require.Empty(t, result.Content[0].Filename)
			got, err := json.Marshal(result.Content[0].Data)
			require.NoError(t, err)
			require.JSONEq(t, data, string(got))
			stream, err := decodeStreamPart([]byte(part))
			require.NoError(t, err)
			require.Equal(t, provider.PartFile, stream.Type)
			got, err = json.Marshal(stream.Data)
			require.NoError(t, err)
			require.JSONEq(t, data, string(got))
			require.Equal(t, result.Content[0].ProviderMetadata, stream.ProviderMetadata)
		}
	}
}

func TestGeneratedFileMalformed(t *testing.T) {
	for _, data := range []string{`null`, `{}`, `{"type":"data"}`, `{"type":"data","data":null}`, `{"type":"data","data":9}`, `{"type":"data","data":"","url":"x"}`, `{"type":"data","data":"","originalUrl":"x"}`, `{"type":"url","url":"x","originalUrl":null}`, `{"type":"url","url":"x","originalUrl":42}`, `{"type":"reference","reference":{}}`} {
		part := `{"type":"file","mediaType":"image/png","data":` + data + `}`
		_, err := decodeStreamPart([]byte(part))
		require.Error(t, err, data)
		_, err = decodeGenerate([]byte(`{"content":[` + part + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`))
		require.Error(t, err, data)
	}
}

func TestGeneratedFileHostileBoundsAndBackpressure(t *testing.T) {
	event := `{"type":"file","mediaType":"image/png","data":{"type":"data","data":"` + strings.Repeat("x", 512) + `"}}`
	frame := sseFrame(event)
	for _, total := range []bool{false, true} {
		limits := DefaultLimits()
		limits.StreamEventBytes = int64(len(frame) - 1)
		if total {
			limits.StreamBytes = limits.StreamEventBytes
		}
		body := &trackedBody{Reader: strings.NewReader(frame)}
		model := streamFromBody(t, body, &limits)
		result, err := model.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		parts := collectParts(t, result)
		require.Len(t, parts, 1)
		require.Equal(t, provider.PartError, parts[0].Type)
		require.True(t, body.closed)
	}
	body := &atomicBody{ReadCloser: io.NopCloser(strings.NewReader(strings.Repeat(frame, 1000)))}
	model := streamFromBody(t, body, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := model.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(result.Stream) == 64 }, time.Second, time.Millisecond)
	cancel()
	collectParts(t, result)
	require.Equal(t, int32(1), body.closed.Load())
	unary := `{"content":[` + event + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`
	limits := DefaultLimits()
	limits.UnaryBytes = int64(len(unary) - 1)
	p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, unary)
	}, &limits)
	m, err := p.LanguageModel("files")
	require.NoError(t, err)
	_, err = m.DoGenerate(context.Background(), provider.CallOptions{})
	require.Error(t, err)
}
