package chatcompletions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deadlineResponseWriter struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
	err       error
}

func (w *deadlineResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return w.err
}

func TestStreamWriter_RequestDeadline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deadline bool
		err      error
		status   int
	}{
		{name: "preserve host deadline when request has none", status: 200},
		{name: "apply request deadline", deadline: true, status: 200},
		{name: "plugin writer need not support deadlines", deadline: true, err: http.ErrNotSupported, status: 200},
		{name: "real writer failure", deadline: true, err: errors.New("write deadline failed"), status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(t, &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(textParts()...), nil
			}})
			req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
			req.Header.Set("Content-Type", "application/json")
			deadline := time.Now().Add(time.Minute)
			if tc.deadline {
				ctx, cancel := context.WithDeadline(req.Context(), deadline)
				defer cancel()
				req = req.WithContext(ctx)
			}
			writer := &deadlineResponseWriter{ResponseRecorder: httptest.NewRecorder(), err: tc.err}
			h.ServeHTTP(writer, req)
			assert.Equal(t, tc.status, writer.Code)
			if tc.deadline {
				require.Equal(t, []time.Time{deadline}, writer.deadlines)
			} else {
				assert.Empty(t, writer.deadlines)
			}
			if tc.status == 200 {
				assert.Contains(t, writer.Body.String(), "[DONE]")
			} else {
				assert.NotContains(t, writer.Body.String(), "[DONE]")
			}
		})
	}
}
