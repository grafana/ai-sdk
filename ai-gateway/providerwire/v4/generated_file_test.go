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
func TestGeneratedFileProjectionAndValidity(t *testing.T) {
	original := "opaque:Original?token=application\"<&"
	for _, data := range []provider.DataContent{provider.BytesDataContent([]byte{1, 2, 3}), provider.Base64DataContent(""), provider.Base64DataContent("opaque-base64"), {URL: "opaque:URL", OriginalURL: &original}} {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: &data, Filename: strings.Repeat("x", 1<<20) + string([]byte{255}), ProviderMetadata: provider.ProviderMetadata{"app": json.RawMessage(`{"token":"ordinary", "nested":{"value":"<&"}}`)}}}
		mapped, err := mapUnarySuccess(result)
		require.NoError(t, err)
		body, ok := marshalUnarySuccess(mapped)
		require.True(t, ok)
		require.NotContains(t, string(body), "filename")
		require.NoError(t, compileWireSchema(t, unarySuccessSchemaJSON).Validate(body))
		event := streamEvent{typeName: provider.PartFile, mediaType: "image/png", fileData: unaryReasoningFile(&data), metadata: result.Content[0].ProviderMetadata}
		_, ok = encodeStreamFrame(event)
		require.True(t, ok)
		h := &handler{}
		state := newStreamState(nil)
		w := httptest.NewRecorder()
		require.Equal(t, streamPartContinue, h.processStreamPart(w, state, provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: event.fileData, ProviderMetadata: event.metadata}))
		require.Equal(t, streamPartContinue, h.processStreamPart(w, state, finishPart()))
		require.Equal(t, segmentFinished, state.phase)
		requireStreamBodyMatchesSchema(t, w.Body.String())
	}
}

func TestGeneratedFileInvalidAndAggregate(t *testing.T) {
	original := "bad"
	for _, data := range []*provider.DataContent{nil, {}, {Bytes: []byte{1}, URL: "opaque:x"}, {Text: "x"}, {Reference: json.RawMessage(`{}`)}, {Base64: "x", OriginalURL: &original}, {URL: string([]byte{255})}} {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: data}}
		_, err := mapUnarySuccess(result)
		require.Error(t, err)
	}
	data := provider.BytesDataContent(make([]byte, 7<<20))
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: &data}, {Type: provider.ContentFile, MediaType: "image/png", Data: &data}}
	mapped, err := mapUnarySuccess(result)
	require.NoError(t, err)
	body, ok := marshalUnarySuccess(mapped)
	require.True(t, ok)
	require.Greater(t, len(body), 8<<20)
	large := &provider.StreamFileData{Bytes: make([]byte, 2<<20)}
	w := httptest.NewRecorder()
	h := &handler{}
	require.Equal(t, streamPartContinue, h.processStreamPart(w, newStreamState(nil), provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: large}))
	require.Greater(t, w.Body.Len(), 2<<20)
}

func TestReasoningOriginalURLHasNoSizeLimit(t *testing.T) {
	original := strings.Repeat("x", 9<<20)
	data := provider.URLDataContent("opaque:x")
	data.OriginalURL = &original
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &data}}
	_, err := mapUnarySuccess(result)
	require.NoError(t, err)
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
