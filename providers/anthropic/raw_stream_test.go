package anthropic

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
