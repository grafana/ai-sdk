package v4

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

// Synthetic provider values exercise the public transport, not a native media producer.
func TestGeneratedFileProjectionAndExactBounds(t *testing.T) {
	original := "opaque:Original?token=application\"<&"
	for _, data := range []provider.DataContent{provider.BytesDataContent([]byte{1, 2, 3}), provider.Base64DataContent(""), provider.Base64DataContent("opaque-base64"), {URL: "opaque:URL", OriginalURL: &original}} {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: &data, Filename: strings.Repeat("x", 1<<20) + string([]byte{255}), ProviderMetadata: provider.ProviderMetadata{"app": json.RawMessage(`{"token":"ordinary", "nested":{"value":"<&"}}`)}}}
		mapped, err := mapUnarySuccess(result, 1<<20)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		require.NotContains(t, string(body), "filename")
		require.NoError(t, compileWireSchema(t, unarySuccessSchemaJSON).Validate(body))
		for _, delta := range []int64{-1, 0, 1} {
			limit := int64(len(body)) + delta
			bounded, err := mapUnarySuccess(result, limit)
			require.NoError(t, err)
			_, ok = encodeUnarySuccess(bounded, limit)
			require.Equal(t, delta >= 0, ok)
		}
		event := streamEvent{typeName: provider.PartFile, mediaType: "image/png", fileData: unaryReasoningFile(&data), metadata: result.Content[0].ProviderMetadata}
		frame, ok := encodeStreamFrame(event, 1<<20)
		require.True(t, ok)
		for _, delta := range []int64{-1, 0, 1} {
			_, ok = encodeStreamFrame(event, int64(len(frame))+delta)
			require.Equal(t, delta >= 0, ok)
		}
		h := &handler{limits: Limits{StreamFrameBytes: 1 << 20}}
		state := newStreamState(100, nil)
		w := httptest.NewRecorder()
		require.Equal(t, streamPartContinue, h.processStreamPart(w, state, provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: event.fileData, ProviderMetadata: event.metadata}))
		require.Equal(t, streamPartFinished, h.processStreamPart(w, state, finishPart()))
		requireStreamBodyMatchesSchema(t, w.Body.String())
	}
}

func TestGeneratedFileInvalidAndAggregate(t *testing.T) {
	original := "bad"
	for _, data := range []*provider.DataContent{nil, {}, {Bytes: []byte{1}, URL: "opaque:x"}, {Text: "x"}, {Reference: json.RawMessage(`{}`)}, {Base64: "x", OriginalURL: &original}, {URL: string([]byte{255})}} {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: data}}
		_, err := mapUnarySuccess(result, 1<<20)
		require.Error(t, err)
	}
	data := provider.BytesDataContent(make([]byte, 128))
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: &data}}
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	result.Content = append(result.Content, result.Content[0])
	mapped, err = mapUnarySuccess(result, int64(len(body)))
	if err == nil {
		_, ok = encodeUnarySuccess(mapped, int64(len(body)))
		require.False(t, ok)
	}
	huge := &provider.StreamFileData{Bytes: make([]byte, 1024)}
	w := httptest.NewRecorder()
	h := &handler{limits: Limits{StreamFrameBytes: 256}}
	require.Equal(t, streamPartAdapterFailure, h.processStreamPart(w, newStreamState(100, nil), provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: huge}))
	require.Empty(t, w.Body.String())
}

func TestReasoningOriginalURLPreflight(t *testing.T) {
	original := strings.Repeat("x", 1024)
	data := provider.URLDataContent("opaque:x")
	data.OriginalURL = &original
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &data}}
	require.False(t, unarySuccessPreflight(result, 512))
}

func TestGeneratedFileSchemaRejectsMalformedArms(t *testing.T) {
	schema := compileWireSchema(t, streamEventSchemaJSON)
	for _, part := range []string{
		`{"type":"file","data":{"type":"data","data":""}}`,
		`{"type":"file","mediaType":null,"data":{"type":"data","data":""}}`,
		`{"type":"file","mediaType":"image/png","data":{"type":"url","url":"x","originalUrl":null}}`,
		`{"type":"file","mediaType":"image/png","data":{"type":"data","data":"","url":"x"}}`,
		`{"type":"file","mediaType":"image/png","data":{"type":"data","data":"","originalUrl":"x"}}`,
		`{"type":"file","mediaType":"image/png","data":{"type":"data","data":""},"filename":"x"}`,
	} {
		require.Error(t, schema.Validate(json.RawMessage(part)), part)
	}
}

func TestGeneratedFileCancellationOwnsProducer(t *testing.T) {
	stopped := make(chan struct{})
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		parts := make(chan provider.StreamPart)
		go func() {
			defer close(parts)
			defer close(stopped)
			select {
			case parts <- provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: &provider.StreamFileData{Bytes: []byte{1}}}:
			case <-ctx.Done():
				return
			}
			<-ctx.Done()
		}()
		return &provider.StreamResult{Stream: parts}, nil
	}
	server := httptest.NewServer(harness.handler)
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := validRequest(`{"prompt":[]}`).WithContext(ctx)
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(server.URL, "http://")
	req.RequestURI = ""
	req.Header.Set(HeaderStreaming, "true")
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		if strings.Contains(line, `"type":"file"`) {
			break
		}
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("generated-file producer did not stop")
	}
}
