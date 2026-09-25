package anthropic

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRawFrameBody_ForwardsBeforeLongLineCompletes(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	capture := &rawFrameCapture{}
	body := &rawFrameBody{ReadCloser: reader, reader: bufio.NewReaderSize(reader, 16), capture: capture}
	release := make(chan struct{})
	defer close(release)
	go func() {
		_, _ = io.WriteString(writer, "data: "+strings.Repeat("x", 11))
		<-release
		_, _ = io.WriteString(writer, "\n\n")
		_ = writer.Close()
	}()
	read := make(chan int, 1)
	go func() {
		buf := make([]byte, 16)
		n, _ := body.Read(buf)
		read <- n
	}()
	select {
	case n := <-read:
		assert.Positive(t, n)
	case <-time.After(time.Second):
		require.FailNow(t, "read waited for a line terminator instead of forwarding to the SDK")
	}
}

func TestRawFrameBody_DoesNotBufferUnterminatedComment(t *testing.T) {
	original := io.NopCloser(strings.NewReader(":" + strings.Repeat("x", 256<<10)))
	capture := &rawFrameCapture{}
	body := &rawFrameBody{ReadCloser: original, reader: bufio.NewReaderSize(original, 16), capture: capture}
	_, err := io.Copy(io.Discard, body)
	require.NoError(t, err)
	assert.Empty(t, body.line)
	assert.Empty(t, body.frameData)
	assert.Empty(t, capture.take())
}

func TestRawFrameBody_DoesNotBufferNonDataLines(t *testing.T) {
	original := io.NopCloser(strings.NewReader(strings.Repeat(": ignored\n", 10000)))
	capture := &rawFrameCapture{}
	body := &rawFrameBody{ReadCloser: original, reader: bufio.NewReaderSize(original, 16), capture: capture}
	_, err := io.Copy(io.Discard, body)
	require.NoError(t, err)
	assert.Empty(t, body.frameData)
	assert.Empty(t, body.line)
	assert.Empty(t, capture.take())
}

func TestRawFrameBody_Frames(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire string
		want []string
	}{
		{name: "LF multiline and comments", wire: ": comment\n\ndata: {\"type\":\"ping\",\ndata: \"value\":1}\n\ndata: [DONE]\n\n", want: []string{`{"type":"ping","value":1}`}},
		{name: "CRLF and malformed", wire: "data: {\"type\":\"ping\"}\r\n\r\ndata: not JSON\r\n\r\n", want: []string{`{"type":"ping"}`, ""}},
		{name: "unknown event", wire: "event: future_event\ndata: {\"type\":\"future_event\"}\n\n", want: []string{`{"type":"future_event"}`}},
		{name: "incomplete frame", wire: "data: {\"type\":\"ping\"}\n", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &rawFrameCapture{}
			original := io.NopCloser(strings.NewReader(tc.wire))
			body := &rawFrameBody{ReadCloser: original, reader: bufio.NewReaderSize(original, 1), capture: capture}
			var got bytes.Buffer
			buf := make([]byte, 3)
			for {
				n, err := body.Read(buf)
				got.Write(buf[:n])
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wire, got.String())
			var values []string
			for _, part := range capture.take() {
				assert.Equal(t, provider.PartRaw, part.Type)
				values = append(values, string(part.RawValue))
			}
			assert.Equal(t, tc.want, values)
			assert.Empty(t, capture.take())
			require.NoError(t, body.Close())
		})
	}
}
